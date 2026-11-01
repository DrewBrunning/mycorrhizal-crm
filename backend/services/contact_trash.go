package services

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"mycorrhizal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Contact trash (issue #2002, ADR 0037): restore and permanent purge of a
// soft-deleted contact, driven by the ContactDeletionSnapshot that
// DeleteContact writes in the delete transaction.

var (
	// ErrContactNotInTrash is returned for a contact that is live, already
	// purged, owned by someone else, or soft-deleted without a snapshot. The
	// controller maps all of them to 404 (no ownership oracle).
	ErrContactNotInTrash = errors.New("contact is not in the trash")
	// ErrSnapshotUnsupported means the snapshot payload version is newer than
	// this binary understands; restore refuses rather than half-restoring.
	ErrSnapshotUnsupported = errors.New("contact deletion snapshot version is not supported")
)

// ContactUIDInUseError is returned when a live contact already holds the
// trashed contact's vcard_uid (the unique index is partial, so this is
// possible). The controller maps it to 409.
type ContactUIDInUseError struct{ LiveContactID uint }

func (e *ContactUIDInUseError) Error() string {
	return fmt.Sprintf("a live contact (id %d) already uses this contact's UID", e.LiveContactID)
}

// ContactDeletionPayload is the JSON stored in ContactDeletionSnapshot.Payload.
type ContactDeletionPayload struct {
	// Soft maps a soft-cascade table to the ids (as strings) of the rows THIS
	// deletion soft-deleted -- the batch marker (ADR 0037 decision 2). Rows the
	// user deleted earlier are not in it, so restore never resurrects them.
	Soft map[string][]string `json:"soft"`
	// Hard maps a restorable hard-cascade table to the rows it removed.
	Hard map[string][]map[string]any `json:"hard"`
	// SelfContact records that the user's "Me" pointer named this contact.
	SelfContact bool `json:"self_contact"`
	// SelfAmbientSet records that marking "Me" had turned this contact's
	// ambient flag on (users.self_contact_ambient_set, issue #2192), so a
	// restore returns the pointer with its provenance. Absent in older payloads.
	SelfAmbientSet bool `json:"self_ambient_set,omitempty"`
}

// RestoreSkip reports rows a restore could not bring back.
type RestoreSkip struct {
	Table  string `json:"table"`
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

// RestoreResult is what a successful restore reports.
type RestoreResult struct {
	Contact models.Contact `json:"-"`
	Skipped []RestoreSkip  `json:"skipped"`
}

// snapshotRestorer describes how one hard-cascade table is restored.
type snapshotRestorer struct {
	// valid reports whether the row may be re-created now (its other referent
	// still exists). nil means the schema's FKs are the only guard.
	valid func(tx *gorm.DB, userID uint, row map[string]any) (bool, error)
}

func contactLiveByUID(tx *gorm.DB, userID uint, uid any) (bool, error) {
	var n int64
	err := tx.Model(&models.Contact{}).Where("user_id = ? AND vcard_uid = ?", userID, uid).Count(&n).Error
	return n > 0, err
}

func liveRowExists(tx *gorm.DB, table string, userID uint, idCol string, id any) (bool, error) {
	var n int64
	err := tx.Table(table).Where(idCol+" = ? AND user_id = ? AND deleted_at IS NULL", id, userID).Count(&n).Error
	return n > 0, err
}

// snapshotRestorers lists every hard-cascade table whose rows are carried in
// the snapshot and re-created on restore (ADR 0037 decision 3). Every other
// hard step declares CascadeStep.SnapshotSkip.
var snapshotRestorers = map[string]snapshotRestorer{
	"activity_contacts": {valid: func(tx *gorm.DB, userID uint, row map[string]any) (bool, error) {
		return liveRowExists(tx, "activities", userID, "id", row["activity_id"])
	}},
	"relationship_edges": {valid: func(tx *gorm.DB, userID uint, row map[string]any) (bool, error) {
		// Both endpoints must be live contacts again; the deleted contact is
		// live by the time rows are re-created, so this is true for it.
		for _, col := range []string{"source_id", "target_id"} {
			ok, err := contactLiveByUID(tx, userID, row[col])
			if err != nil || !ok {
				return false, err
			}
		}
		return true, nil
	}},
	"household_members":   {},
	"circle_members":      {},
	"contact_tags":        {},
	"field_values":        {},
	"external_identities": {},
	"external_activities": {},
	"occasion_event_attendees": {valid: func(tx *gorm.DB, userID uint, row map[string]any) (bool, error) {
		return liveRowExists(tx, "occasion_events", userID, "id", row["event_id"])
	}},
	"import_source_links": {},
	// Issue #1997: the user's per-contact field visibility comes back with the
	// contact. The natural key (user, contact, field) makes a re-created row a
	// no-op rather than a duplicate.
	"contact_field_overrides": {},
}

func asIDStrings(raw []string) []string { return raw }

// normalizeRow makes a scanned row JSON-round-trippable: SQLite BLOB/TEXT
// values can arrive as []byte.
func normalizeRow(row map[string]any) map[string]any {
	for k, v := range row {
		if b, ok := v.([]byte); ok {
			row[k] = string(b)
		}
	}
	return row
}

// captureContactDeletion reads, BEFORE the cascade runs, everything restore
// will need that the cascade is about to destroy or tombstone.
func captureContactDeletion(tx *gorm.DB, contact models.Contact, userID uint) (ContactDeletionPayload, error) {
	target := ContactCascadeTarget{ID: contact.ID, VCardUID: contact.VCardUID, UserID: userID}
	payload := ContactDeletionPayload{Soft: map[string][]string{}, Hard: map[string][]map[string]any{}}
	for _, step := range ContactCascadeRegistry() {
		switch step.Mode {
		case CascadeSoft:
			query, args := step.Match(target)
			var ids []string
			if err := tx.Table(step.Table).Where(query, args...).Where("deleted_at IS NULL").Pluck("id", &ids).Error; err != nil {
				return payload, err
			}
			if len(ids) > 0 {
				payload.Soft[step.Table] = asIDStrings(ids)
			}
		case CascadeHard:
			if _, ok := snapshotRestorers[step.Table]; !ok {
				continue
			}
			query, args := step.Match(target)
			var rows []map[string]any
			if err := tx.Table(step.Table).Where(query, args...).Find(&rows).Error; err != nil {
				return payload, err
			}
			if len(rows) == 0 {
				continue
			}
			for i := range rows {
				rows[i] = normalizeRow(rows[i])
			}
			payload.Hard[step.Table] = rows
		case CascadeMutate:
			// Not a delete: the "Me" pointer is captured below; feeds stay
			// revoked and the sync link is retained (ADR 0037).
		}
	}
	var meRows []models.User
	if err := tx.Select("id", "self_contact_ambient_set").Where("id = ? AND self_contact_vcard_uid = ?", userID, contact.VCardUID).Find(&meRows).Error; err != nil {
		return payload, err
	}
	payload.SelfContact = len(meRows) > 0
	payload.SelfAmbientSet = len(meRows) > 0 && meRows[0].SelfContactAmbientSet
	return payload, nil
}

func writeContactDeletionSnapshot(tx *gorm.DB, contact models.Contact, userID uint, now time.Time, payload ContactDeletionPayload) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return tx.Create(&models.ContactDeletionSnapshot{
		UserID:          userID,
		ContactID:       contact.ID,
		ContactVCardUID: contact.VCardUID,
		TrashedAt:       now,
		PayloadVersion:  models.ContactDeletionSnapshotVersion,
		Payload:         string(raw),
	}).Error
}

func decodePayload(raw string) (ContactDeletionPayload, error) {
	var p ContactDeletionPayload
	dec := json.NewDecoder(bytes.NewReader([]byte(raw)))
	dec.UseNumber() // keep integer ids exact
	if err := dec.Decode(&p); err != nil {
		return p, err
	}
	return p, nil
}

func trashedContact(tx *gorm.DB, userID, contactID uint) (models.Contact, models.ContactDeletionSnapshot, error) {
	var contact models.Contact
	var snap models.ContactDeletionSnapshot
	err := tx.Unscoped().Where("id = ? AND user_id = ? AND deleted_at IS NOT NULL", contactID, userID).First(&contact).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return contact, snap, ErrContactNotInTrash
	}
	if err != nil {
		return contact, snap, err
	}
	err = tx.Where("user_id = ? AND contact_id = ?", userID, contactID).First(&snap).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return contact, snap, ErrContactNotInTrash
	}
	return contact, snap, err
}

// RestoreContact brings a trashed contact back from its snapshot in one
// transaction. It is owner-scoped: another user's id is ErrContactNotInTrash.
func RestoreContact(db *gorm.DB, userID, contactID uint, now time.Time) (*RestoreResult, error) {
	res := &RestoreResult{Skipped: []RestoreSkip{}}
	err := db.Transaction(func(tx *gorm.DB) error {
		contact, snap, err := trashedContact(tx, userID, contactID)
		if err != nil {
			return err
		}
		if snap.PayloadVersion != models.ContactDeletionSnapshotVersion {
			return ErrSnapshotUnsupported
		}
		payload, err := decodePayload(snap.Payload)
		if err != nil {
			return fmt.Errorf("decode deletion snapshot: %w", err)
		}

		var live models.Contact
		lerr := tx.Where("user_id = ? AND vcard_uid = ?", userID, contact.VCardUID).First(&live).Error
		if lerr == nil {
			return &ContactUIDInUseError{LiveContactID: live.ID}
		}
		if !errors.Is(lerr, gorm.ErrRecordNotFound) {
			return lerr
		}

		// Un-delete without hooks first (the audit hook's before-snapshot
		// re-query is scoped and would not find a soft-deleted row), then Save
		// the live row so BeforeSave rebuilds the flat columns and AfterSave
		// bumps the revision/ETag (ADR 0006): the CardDAV server and the
		// ?since= feed then see the contact as changed.
		if err := tx.Unscoped().Model(&models.Contact{}).Where("id = ? AND user_id = ?", contact.ID, userID).
			UpdateColumn("deleted_at", nil).Error; err != nil {
			return err
		}
		contact.DeletedAt = gorm.DeletedAt{}
		if err := tx.Save(&contact).Error; err != nil {
			return err
		}

		tables := make([]string, 0, len(payload.Soft))
		for t := range payload.Soft {
			tables = append(tables, t)
		}
		sort.Strings(tables)
		for _, table := range tables {
			if err := tx.Table(table).Where("id IN ? AND user_id = ? AND deleted_at IS NOT NULL", payload.Soft[table], userID).
				UpdateColumns(map[string]any{"deleted_at": nil, "updated_at": now}).Error; err != nil {
				return err
			}
		}

		hard := make([]string, 0, len(payload.Hard))
		for t := range payload.Hard {
			hard = append(hard, t)
		}
		sort.Strings(hard)
		for _, table := range hard {
			skipped, err := restoreRows(tx, userID, table, payload.Hard[table])
			if err != nil {
				return err
			}
			if skipped > 0 {
				res.Skipped = append(res.Skipped, RestoreSkip{Table: table, Reason: "referent_gone_or_exists", Count: skipped})
			}
		}

		if payload.SelfContact {
			// Only if the pointer is still free: never overwrite a newer choice.
			if err := tx.Model(&models.User{}).Where("id = ? AND self_contact_vcard_uid IS NULL", userID).
				Updates(map[string]any{"self_contact_vcard_uid": contact.VCardUID, "self_contact_ambient_set": payload.SelfAmbientSet}).Error; err != nil {
				return err
			}
		}

		if err := tx.Delete(&snap).Error; err != nil {
			return err
		}
		models.RecordAuditEvent(tx, models.AuditEntityContact, contact.VCardUID, models.AuditOpRestore, userID)
		res.Contact = contact
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// restoreRows re-creates one table's snapshotted rows, returning how many
// could not be (referent gone, or the natural key was re-created meanwhile).
func restoreRows(tx *gorm.DB, userID uint, table string, rows []map[string]any) (int, error) {
	restorer := snapshotRestorers[table]
	skipped := 0
	for _, row := range rows {
		if restorer.valid != nil {
			ok, err := restorer.valid(tx, userID, row)
			if err != nil {
				return skipped, err
			}
			if !ok {
				skipped++
				continue
			}
		}
		if _, hasUser := row["user_id"]; hasUser {
			row["user_id"] = userID // always the caller, never a value read back from the payload
		}
		inserted := false
		// A savepoint, so a constraint failure skips this row without
		// aborting the restore.
		serr := tx.Transaction(func(sp *gorm.DB) error {
			r := sp.Table(table).Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
			inserted = r.RowsAffected > 0
			return r.Error
		})
		if serr != nil || !inserted {
			skipped++
		}
	}
	return skipped, nil
}

// TrashEntry is one row of the trash listing.
type TrashEntry struct {
	Contact   models.Contact
	DeletedAt time.Time
}

// ListTrash returns the user's trashed, restorable contacts (those with a
// deletion snapshot), newest deletion first, plus the total.
func ListTrash(db *gorm.DB, userID uint, offset, limit int) ([]TrashEntry, int64, error) {
	base := db.Table("contact_deletion_snapshots").Where("user_id = ?", userID)
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var snaps []models.ContactDeletionSnapshot
	if err := db.Where("user_id = ?", userID).Order("trashed_at DESC, id").Offset(offset).Limit(limit).Find(&snaps).Error; err != nil {
		return nil, 0, err
	}
	entries := make([]TrashEntry, 0, len(snaps))
	for _, s := range snaps {
		var c models.Contact
		if err := db.Unscoped().Where("id = ? AND user_id = ? AND deleted_at IS NOT NULL", s.ContactID, userID).First(&c).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue // snapshot of a contact that is live or gone; the scheduled purge reaps it
			}
			return nil, 0, err
		}
		entries = append(entries, TrashEntry{Contact: c, DeletedAt: s.TrashedAt})
	}
	return entries, total, nil
}

// PurgedFiles are the on-disk files a purge made unreferenced. The caller
// removes them only after the purge transaction committed (ADR 0037 decision 5).
type PurgedFiles struct {
	Photos      []string
	Attachments []string
	Images      []ImageFile
}

// ImageFile names one gallery file and the user directory it lives under.
type ImageFile struct {
	UserID uint
	Name   string
}

func (f *PurgedFiles) merge(o PurgedFiles) {
	f.Photos = append(f.Photos, o.Photos...)
	f.Attachments = append(f.Attachments, o.Attachments...)
	f.Images = append(f.Images, o.Images...)
}

func appendImageFiles(dst []ImageFile, userID uint, names ...string) []ImageFile {
	for _, name := range names {
		if name != "" {
			dst = append(dst, ImageFile{UserID: userID, Name: name})
		}
	}
	return dst
}

// purgeContactInTx hard-deletes one trashed contact, its batch-soft-deleted
// children, its remaining soft-deleted rows keyed to it, and its snapshot.
func purgeContactInTx(tx *gorm.DB, userID uint, contact models.Contact, snap models.ContactDeletionSnapshot) (PurgedFiles, error) {
	var files PurgedFiles
	files.Photos = appendPhotoFiles(files.Photos, contact.Photo, contact.PhotoThumbnail)
	if err := tx.Unscoped().Model(&models.Attachment{}).
		Where("contact_vcard_uid = ? AND user_id = ?", contact.VCardUID, userID).
		Pluck("stored_name", &files.Attachments).Error; err != nil {
		return files, err
	}
	target := ContactCascadeTarget{ID: contact.ID, VCardUID: contact.VCardUID, UserID: userID}
	for _, step := range ContactCascadeRegistry() {
		if step.Mode != CascadeSoft {
			continue
		}
		query, args := step.Match(target)
		if err := tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE %s AND deleted_at IS NOT NULL", step.Table, query), args...).Error; err != nil {
			return files, err
		}
	}
	if err := tx.Unscoped().Delete(&models.Contact{}, contact.ID).Error; err != nil {
		return files, err
	}
	if err := tx.Delete(&snap).Error; err != nil {
		return files, err
	}
	models.RecordAuditEvent(tx, models.AuditEntityContact, contact.VCardUID, models.AuditOpPurge, userID)
	return files, nil
}

// PurgeContact permanently deletes one trashed contact now. Owner-scoped.
func PurgeContact(db *gorm.DB, userID, contactID uint) (PurgedFiles, error) {
	var files PurgedFiles
	err := db.Transaction(func(tx *gorm.DB) error {
		contact, snap, err := trashedContact(tx, userID, contactID)
		if err != nil {
			return err
		}
		files, err = purgeContactInTx(tx, userID, contact, snap)
		return err
	})
	return files, err
}

// EmptyTrash permanently deletes every trashed contact of the user, returning
// how many were purged. One transaction: it purges all or none.
func EmptyTrash(db *gorm.DB, userID uint) (PurgedFiles, int, error) {
	var files PurgedFiles
	n := 0
	err := db.Transaction(func(tx *gorm.DB) error {
		var snaps []models.ContactDeletionSnapshot
		if err := tx.Where("user_id = ?", userID).Find(&snaps).Error; err != nil {
			return err
		}
		for _, snap := range snaps {
			contact, s, err := trashedContact(tx, userID, snap.ContactID)
			if errors.Is(err, ErrContactNotInTrash) {
				if derr := tx.Delete(&snap).Error; derr != nil {
					return derr
				}
				continue
			}
			if err != nil {
				return err
			}
			f, err := purgeContactInTx(tx, userID, contact, s)
			if err != nil {
				return err
			}
			files.merge(f)
			n++
		}
		return nil
	})
	return files, n, err
}

// appendPhotoFiles adds a contact's on-disk photo names: the photo itself and
// a legacy file-based thumbnail (a "data:" thumbnail lives in the row).
func appendPhotoFiles(dst []string, photo, thumbnail string) []string {
	if photo != "" {
		dst = append(dst, photo)
	}
	if thumbnail != "" && !strings.HasPrefix(thumbnail, "data:") {
		dst = append(dst, thumbnail)
	}
	return dst
}
