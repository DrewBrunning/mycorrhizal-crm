package services

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"mycorrhizal/contactmodel"
	"mycorrhizal/logger"
	"mycorrhizal/models"
)

// Hobby preference -> Card.PersonalInfo startup backfill (issue #1738).
//
// Hobbies used to have two homes: Card.PersonalInfo (what CardDAV
// round-trips) and Preference rows with category "hobby". The Card is now the
// single canonical home, and "dislikes" are their own unsynced preference
// category. Card is stored in an encrypted-json column, so this cannot be a
// SQL migration: it is a one-shot Go startup job wired beside the other
// backfills, using migration 000052's data_backfills ledger to run exactly
// once per database. Rows created later through the deprecated hobby API path
// are therefore not swept mid-window.
//
// For each live hobby preference at backfill time:
//
//   - key favorite/like/empty: merge into (or create) a Card.PersonalInfo
//     entry {kind: hobby, value, level}, and record sensitivity/favorite/
//     notes/provenance in CRMEnvelope.PersonalInfoMeta keyed by the entry's
//     stable ID. A same-value (case-insensitive) entry is merged rather than
//     duplicated: fill level only if empty, keep the stricter sensitivity,
//     OR the favorite flag, concatenate non-empty notes, and keep existing
//     provenance unless it is empty.
//   - key "dislike": recategorize in place to dislike_activity, clear the key,
//     and keep every other column. It is never projected to a Card.
//
// Moved rows are then soft-deleted (recoverable, and excluded from
// GET /preferences by the normal soft-delete scope). A later boot is a no-op
// because the ledger row short-circuits it; if the process dies before the
// ledger is written, re-running is safe: moved rows are already soft-deleted
// and recategorized rows are no longer in the hobby category.
const (
	// HobbyPreferenceBackfillName is the data_backfills.name ledger key.
	HobbyPreferenceBackfillName = "preferences.hobby_to_personal_info"
	// HobbyPreferenceBackfillVersion bumps if the merge rules change and
	// already-processed data must be revisited.
	HobbyPreferenceBackfillVersion = 1
)

// HobbyPreferenceBackfillStats reports what a run did.
type HobbyPreferenceBackfillStats struct {
	PreferencesScanned        int64 `json:"preferences_scanned"`
	ContactsUpdated           int64 `json:"contacts_updated"`
	PersonalInfoEntriesMerged int64 `json:"personal_info_entries_merged"`
	DislikesRecategorized     int64 `json:"dislikes_recategorized"`
	PreferencesSoftDeleted    int64 `json:"preferences_soft_deleted"`
	AlreadyDone               bool  `json:"already_done"`
}

// MigrateHobbyPreferencesToPersonalInfo performs the one-shot lossless move
// described in the file comment. A nil db is a no-op. On a pre-000052 schema
// (no data_backfills table) it fails closed rather than silently doing
// nothing.
func MigrateHobbyPreferencesToPersonalInfo(db *gorm.DB) (*HobbyPreferenceBackfillStats, error) {
	stats := &HobbyPreferenceBackfillStats{}
	if db == nil {
		return stats, nil
	}

	done, err := hobbyPreferenceBackfillAlreadyDone(db)
	if err != nil {
		return stats, err
	}
	if done {
		stats.AlreadyDone = true
		logger.Info().Msg("hobby preference backfill: already complete, skipping")
		return stats, nil
	}

	var prefs []models.Preference
	if err := db.Where("category = ?", models.PreferenceCategoryHobby).
		Order("user_id ASC, entity_id ASC, created_at ASC, id ASC").
		Find(&prefs).Error; err != nil {
		return stats, fmt.Errorf("hobby preference backfill (load preferences): %w", err)
	}
	stats.PreferencesScanned = int64(len(prefs))

	for start := 0; start < len(prefs); {
		end := start + 1
		for end < len(prefs) &&
			prefs[end].UserID == prefs[start].UserID &&
			prefs[end].EntityID == prefs[start].EntityID {
			end++
		}
		group := prefs[start:end]
		start = end

		err := db.Transaction(func(tx *gorm.DB) error {
			var contact models.Contact
			if err := tx.Unscoped().
				Where("user_id = ? AND vcard_uid = ?", group[0].UserID, group[0].EntityID).
				First(&contact).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					// A preference can outlive its contact (e.g. a dangling
					// row). Do not destroy it or invent a contact; it stays
					// in the deprecated category and keeps working.
					logger.Warn().
						Str("vcard_uid", group[0].EntityID).
						Uint("user_id", group[0].UserID).
						Msg("hobby preference backfill: contact not found, leaving preferences untouched")
					return nil
				}
				return fmt.Errorf("hobby preference backfill (load contact): %w", err)
			}

			record := models.RecordForContact(&contact, "", nil)
			changed := false
			for i := range group {
				pref := &group[i]
				switch pref.Key {
				case models.PreferenceKeyDislike:
					if err := tx.Model(pref).Updates(map[string]any{
						"category": models.PreferenceCategoryDislikeActivity,
						"key":      "",
					}).Error; err != nil {
						return fmt.Errorf("hobby preference backfill (recategorize dislike): %w", err)
					}
					stats.DislikesRecategorized++
				case "", "favorite", "like":
					if pref.Value == "" {
						continue
					}
					mergeHobbyPreferenceIntoRecord(record, pref)
					if err := tx.Delete(pref).Error; err != nil {
						return fmt.Errorf("hobby preference backfill (soft-delete moved preference): %w", err)
					}
					stats.PersonalInfoEntriesMerged++
					stats.PreferencesSoftDeleted++
					changed = true
				default:
					// An unrecognized disposition cannot be mapped without
					// losing the key, so it is deliberately left in place.
					// It keeps working through the deprecated projection for
					// the rest of the 1.x window.
					logger.Warn().
						Str("preference_id", pref.ID).
						Str("key", pref.Key).
						Msg("hobby preference backfill: unrecognized key, leaving preference untouched")
				}
			}

			if !changed {
				return nil
			}
			models.ApplyRecordToContact(&contact, record, "")
			if err := tx.Unscoped().Save(&contact).Error; err != nil {
				return fmt.Errorf("hobby preference backfill (save contact id=%d): %w", contact.ID, err)
			}
			stats.ContactsUpdated++
			return nil
		})
		if err != nil {
			return stats, err
		}
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		return tx.Exec(
			`INSERT INTO data_backfills (name, version, completed_at) VALUES (?, ?, ?)`,
			HobbyPreferenceBackfillName, HobbyPreferenceBackfillVersion, Now().UTC(),
		).Error
	}); err != nil {
		return stats, fmt.Errorf("hobby preference backfill (record completion): %w", err)
	}

	logger.Info().
		Int64("preferences_scanned", stats.PreferencesScanned).
		Int64("contacts_updated", stats.ContactsUpdated).
		Int64("entries_merged", stats.PersonalInfoEntriesMerged).
		Int64("dislikes_recategorized", stats.DislikesRecategorized).
		Int64("preferences_soft_deleted", stats.PreferencesSoftDeleted).
		Msg("hobby preference backfill complete")
	return stats, nil
}

// hobbyPreferenceBackfillAlreadyDone reports whether the ledger row exists. A
// missing data_backfills table is a hard error (the caller is running on a
// schema older than migration 000052).
func hobbyPreferenceBackfillAlreadyDone(db *gorm.DB) (bool, error) {
	var n int64
	if err := db.Table("data_backfills").
		Where("name = ? AND version = ?", HobbyPreferenceBackfillName, HobbyPreferenceBackfillVersion).
		Count(&n).Error; err != nil {
		return false, fmt.Errorf("hobby preference backfill (read ledger): %w", err)
	}
	return n > 0, nil
}

// mergeHobbyPreferenceIntoRecord merges one favorite/like/empty hobby
// preference into record's Card.PersonalInfo + PersonalInfoMeta, creating an
// entry when no same-value (case-insensitive) entry exists.
func mergeHobbyPreferenceIntoRecord(record *contactmodel.Record, pref *models.Preference) {
	if record.Envelope.PersonalInfoMeta == nil {
		record.Envelope.PersonalInfoMeta = map[string]contactmodel.PersonalInfoMeta{}
	}

	records := record.Card.PersonalInfo
	idx := -1
	value := strings.TrimSpace(pref.Value)
	for i := range records {
		if strings.EqualFold(strings.TrimSpace(records[i].Value), value) {
			idx = i
			break
		}
	}

	if idx < 0 {
		entry := contactmodel.PersonalInfo{
			ID:    uuid.NewString(),
			Kind:  "hobby",
			Value: pref.Value,
		}
		if pref.Level != nil {
			entry.Level = *pref.Level
		}
		record.Card.PersonalInfo = append(records, entry)
		record.Envelope.PersonalInfoMeta[entry.ID] = metaFromHobbyPreference(pref, contactmodel.PersonalInfoMeta{})
		return
	}

	// Copy-on-write: never mutate a slice the caller still holds.
	entries := append([]contactmodel.PersonalInfo(nil), records...)
	entry := &entries[idx]
	if entry.ID == "" {
		entry.ID = uuid.NewString()
	}
	if entry.Kind == "" {
		entry.Kind = "hobby"
	}
	if entry.Level == "" && pref.Level != nil {
		entry.Level = *pref.Level
	}
	record.Card.PersonalInfo = entries

	existing := record.Envelope.PersonalInfoMeta[entry.ID]
	record.Envelope.PersonalInfoMeta[entry.ID] = metaFromHobbyPreference(pref, existing)
}

// metaFromHobbyPreference folds a preference into existing CRM metadata using
// the issue #1738 merge rules: stricter sensitivity, OR-ed favorite, existing
// notes plus the preference's notes separated by a blank line, and existing
// provenance unless it is empty.
func metaFromHobbyPreference(pref *models.Preference, existing contactmodel.PersonalInfoMeta) contactmodel.PersonalInfoMeta {
	meta := existing
	meta.Sensitivity = stricterSensitivity(existing.NormalizedSensitivity(), pref.Sensitivity)
	meta.Favorite = existing.Favorite || pref.Key == "favorite"
	switch {
	case existing.Notes == "":
		meta.Notes = pref.Notes
	case pref.Notes == "" || pref.Notes == existing.Notes:
		meta.Notes = existing.Notes
	default:
		meta.Notes = existing.Notes + "\n\n" + pref.Notes
	}
	if meta.Source == "" {
		meta.Source = pref.Source
	}
	if meta.Confidence == nil {
		meta.Confidence = pref.Confidence
	}
	if meta.LastConfirmed == nil {
		meta.LastConfirmed = pref.LastConfirmed
	}
	return meta
}

// stricterSensitivity returns the more protective of two sensitivity tokens
// (normal < private < secret). An unrecognized token is treated as normal,
// matching the rest of the model.
func stricterSensitivity(a, b string) string {
	rank := func(s string) int {
		switch s {
		case models.RelationshipSensitivitySecret:
			return 2
		case models.RelationshipSensitivityPrivate:
			return 1
		default:
			return 0
		}
	}
	if rank(b) > rank(a) {
		return b
	}
	if a == "" {
		return models.RelationshipSensitivityNormal
	}
	return a
}
