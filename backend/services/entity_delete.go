package services

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"mycorrhizal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Entity delete cascade (issue #2295, ADR 0043 decision 6, ADR 0035).
//
// EntityCascadeRegistry is the declarative twin of ContactCascadeRegistry for
// the tables that reference entities.id. entity_delete_test.go proves it covers
// every table in the real migrated schema that references an entity, so adding
// an entity-referencing table without a step fails CI instead of silently
// orphaning rows on delete.

// EntityCascadeTarget identifies the entity being deleted. BatchID is the one
// new deletion batch id stamped on every cascade-soft-deleted child.
type EntityCascadeTarget struct {
	ID      string
	UserID  uint
	BatchID string
}

// EntityCascadeStep is one dependent table's cleanup; it has the shape of
// CascadeStep, keyed by an EntityCascadeTarget.
type EntityCascadeStep struct {
	Table string
	// NewModel returns a zero model for the default delete (unused when Apply is set).
	NewModel func() any
	Mode     CascadeMode
	// Match returns the WHERE clause selecting this entity's rows, always
	// scoped by user_id (trap #5).
	Match func(t EntityCascadeTarget) (string, []any)
	// Apply replaces the default delete (used by the soft link step, which
	// stamps the batch id as it tombstones).
	Apply func(tx *gorm.DB, t EntityCascadeTarget, now time.Time) error
	// Reason is the recorded justification for the step's mode (ADR 0004/0035).
	Reason string
	// SnapshotSkip, on a hard step, records why its rows are NOT carried in the
	// entity deletion snapshot.
	SnapshotSkip string
}

// EntityCascadeRegistry returns the ordered list of every entities.id-dependent
// table and how an entity delete treats it. A fresh slice per call.
func EntityCascadeRegistry() []EntityCascadeStep {
	return []EntityCascadeStep{
		{
			Table: "contact_entity_links", NewModel: func() any { return &models.ContactEntityLink{} },
			Mode: CascadeSoft,
			Match: func(t EntityCascadeTarget) (string, []any) {
				return "entity_id = ? AND user_id = ?", []any{t.ID, t.UserID}
			},
			Apply: func(tx *gorm.DB, t EntityCascadeTarget, now time.Time) error {
				// Only live links are tombstoned and stamped: a link the user
				// deleted earlier keeps a NULL batch and is not revived by restore.
				return tx.Exec(`UPDATE contact_entity_links
					SET deleted_at = ?, updated_at = ?, deletion_batch_id = ?,
					    revision = revision + 1, etag = 'e-' || id || '-' || (revision + 1)
					WHERE entity_id = ? AND user_id = ? AND deleted_at IS NULL`,
					now, now, t.BatchID, t.ID, t.UserID).Error
			},
			Reason: "ADR 0043 decision 6: a link carries authored data (role, dates, primary), so it soft-deletes with the entity's new deletion_batch_id and restore revives exactly that batch",
		},
		{
			Table: "field_values", NewModel: func() any { return &models.FieldValue{} },
			Mode: CascadeHard,
			Match: func(t EntityCascadeTarget) (string, []any) {
				return "entity_id = ? AND user_id = ?", []any{t.ID, t.UserID}
			},
			Reason: "join-shaped (field definition x entity), trap #7; the removed values are written to entity_deletion_snapshots so a restore re-creates them",
		},
		{
			Table: "dismissed_entity_pairs", NewModel: func() any { return &models.DismissedEntityPair{} },
			Mode: CascadeHard,
			Match: func(t EntityCascadeTarget) (string, []any) {
				return "(id_low = ? OR id_high = ?) AND user_id = ?", []any{t.ID, t.ID, t.UserID}
			},
			SnapshotSkip: "derived dismissal memory; the suggestions endpoint regenerates candidates",
			Reason:       "ADR 0043 decision 8: join-shaped dismissal naming this entity on either side of the ordered id pair",
		},
	}
}

// entityFieldValueSnapshot is one hard-deleted custom field value.
type entityFieldValueSnapshot struct {
	FieldDefinitionID string          `json:"field_definition_id"`
	Value             json.RawMessage `json:"value"`
}

// EntityDeletionPayload is the JSON stored in EntityDeletionSnapshot.Payload.
type EntityDeletionPayload struct {
	FieldValues []entityFieldValueSnapshot `json:"field_values"`
	// MergedInto is the survivor's id when the entity was soft-deleted as an
	// entity-merge loser (issue #2299). Optional and additive: a payload without
	// it is a plain delete, and the snapshot version is unchanged.
	MergedInto string `json:"merged_into,omitempty"`
}

// ErrEntityNotInTrash is returned for an entity that is live, purged, owned by
// someone else, or soft-deleted without a snapshot (the controller maps all to 404).
var ErrEntityNotInTrash = errors.New("entity is not in the trash")

// ErrEntitySnapshotUnsupported means the snapshot payload version is newer than
// this binary understands; restore refuses rather than half-restoring.
var ErrEntitySnapshotUnsupported = errors.New("entity deletion snapshot version is not supported")

// DeleteEntity soft-deletes an entity and cascades, in one transaction: the
// entity is tombstoned, its live links are tombstoned with one new batch id,
// its field values are hard-deleted into an encrypted snapshot, and dismissals
// naming it are removed. The snapshot exists iff the delete committed.
func DeleteEntity(db *gorm.DB, userID uint, entityID string, now time.Time) error {
	return deleteEntity(db, userID, entityID, now, "")
}

// deleteEntity is DeleteEntity with the optional merge-survivor id recorded in
// the snapshot, so a later restore can say the links stayed with the survivor.
func deleteEntity(db *gorm.DB, userID uint, entityID string, now time.Time, mergedInto string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var entity models.Entity
		if err := tx.Where("id = ? AND user_id = ?", entityID, userID).First(&entity).Error; err != nil {
			return err
		}
		target := EntityCascadeTarget{ID: entity.ID, UserID: userID, BatchID: uuid.NewString()}

		var values []models.FieldValue
		if err := tx.Where("entity_id = ? AND user_id = ?", entity.ID, userID).Order("id").Find(&values).Error; err != nil {
			return err
		}
		payload := EntityDeletionPayload{FieldValues: make([]entityFieldValueSnapshot, 0, len(values)), MergedInto: mergedInto}
		for _, v := range values {
			payload.FieldValues = append(payload.FieldValues, entityFieldValueSnapshot{FieldDefinitionID: v.FieldDefinitionID, Value: v.Value})
		}

		for _, step := range EntityCascadeRegistry() {
			if err := runEntityCascadeStep(tx, step, target, now); err != nil {
				return err
			}
		}
		if err := tx.Delete(&entity).Error; err != nil {
			return err
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		return tx.Create(&models.EntityDeletionSnapshot{
			UserID: userID, EntityID: entity.ID, DeletionBatchID: target.BatchID,
			TrashedAt: now, PayloadVersion: models.EntityDeletionSnapshotVersion, Payload: string(raw),
		}).Error
	})
}

func runEntityCascadeStep(tx *gorm.DB, step EntityCascadeStep, target EntityCascadeTarget, now time.Time) error {
	if step.Apply != nil {
		return step.Apply(tx, target, now)
	}
	query, args := step.Match(target)
	return tx.Where(query, args...).Delete(step.NewModel()).Error
}

// EntityRestoreResult is what a successful entity restore reports.
type EntityRestoreResult struct {
	Entity  models.Entity
	Skipped []RestoreSkip
	// SameNameLive is true when another live entity of the same kind already has
	// this name_key: not a conflict (names are not unique), reported as a warning.
	SameNameLive bool
	// LinksRevived counts links that came back with the entity; MergedInto is
	// the survivor's id when the entity was a merge loser.
	LinksRevived int
	MergedInto   string
}

// RestoreEntity brings a trashed entity back in one transaction: it revives the
// entity, revives the links of its deletion batch (demoting a primary that
// collides, skipping links whose contact is gone), and re-creates the field
// values from the snapshot.
func RestoreEntity(db *gorm.DB, userID uint, entityID string, now time.Time) (*EntityRestoreResult, error) {
	res := &EntityRestoreResult{Skipped: []RestoreSkip{}}
	err := db.Transaction(func(tx *gorm.DB) error {
		var entity models.Entity
		err := tx.Unscoped().Where("id = ? AND user_id = ? AND deleted_at IS NOT NULL", entityID, userID).First(&entity).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrEntityNotInTrash
		}
		if err != nil {
			return err
		}
		var snap models.EntityDeletionSnapshot
		err = tx.Where("user_id = ? AND entity_id = ?", userID, entityID).First(&snap).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrEntityNotInTrash
		}
		if err != nil {
			return err
		}
		if snap.PayloadVersion != models.EntityDeletionSnapshotVersion {
			return ErrEntitySnapshotUnsupported
		}
		var payload EntityDeletionPayload
		dec := json.NewDecoder(bytes.NewReader([]byte(snap.Payload)))
		if err := dec.Decode(&payload); err != nil {
			return fmt.Errorf("decode entity deletion snapshot: %w", err)
		}

		// Un-delete without hooks, then Save so the revision/ETag advance.
		if err := tx.Unscoped().Model(&models.Entity{}).Where("id = ? AND user_id = ?", entity.ID, userID).
			UpdateColumn("deleted_at", nil).Error; err != nil {
			return err
		}
		entity.DeletedAt = gorm.DeletedAt{}
		if err := tx.Save(&entity).Error; err != nil {
			return err
		}

		var linkIDs []string
		if err := tx.Unscoped().Model(&models.ContactEntityLink{}).
			Where("entity_id = ? AND user_id = ? AND deleted_at IS NOT NULL AND deletion_batch_id = ?", entity.ID, userID, snap.DeletionBatchID).
			Order("created_at, id").Pluck("id", &linkIDs).Error; err != nil {
			return err
		}
		skips, err := reviveContactEntityLinks(tx, userID, linkIDs, now)
		if err != nil {
			return err
		}
		res.Skipped = append(res.Skipped, skips...)
		res.LinksRevived = len(linkIDs)
		for _, sk := range skips {
			if sk.Reason == RestoreReasonEntityGone || sk.Reason == RestoreReasonContactGone {
				res.LinksRevived -= sk.Count
			}
		}
		res.MergedInto = payload.MergedInto

		skippedValues := 0
		for _, v := range payload.FieldValues {
			var defs int64
			if err := tx.Model(&models.FieldDefinition{}).Where("id = ? AND user_id = ?", v.FieldDefinitionID, userID).Count(&defs).Error; err != nil {
				return err
			}
			if defs == 0 {
				skippedValues++
				continue
			}
			row := models.FieldValue{FieldDefinitionID: v.FieldDefinitionID, UserID: userID, EntityID: entity.ID, Value: v.Value}
			r := tx.Where("field_definition_id = ? AND entity_id = ?", row.FieldDefinitionID, row.EntityID).FirstOrCreate(&row)
			if r.Error != nil {
				return r.Error
			}
		}
		if skippedValues > 0 {
			res.Skipped = append(res.Skipped, RestoreSkip{Table: "field_values", Reason: "definition_gone", Count: skippedValues})
		}

		var same int64
		if err := tx.Model(&models.Entity{}).
			Where("user_id = ? AND kind = ? AND name_key = ? AND id <> ?", userID, entity.Kind, entity.NameKey, entity.ID).
			Count(&same).Error; err != nil {
			return err
		}
		res.SameNameLive = same > 0

		if err := tx.Delete(&snap).Error; err != nil {
			return err
		}
		res.Entity = entity
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// Skip reasons reported by reviveContactEntityLinks.
const (
	RestoreReasonPrimaryDemoted = "primary_demoted"
	RestoreReasonEntityGone     = "entity_deleted"
	RestoreReasonContactGone    = "contact_deleted"
)

// reviveContactEntityLinks un-deletes the given soft-deleted links (ADR 0043
// decision 6). Each is owner-scoped. A link whose entity or contact is not live
// is left deleted and reported. A revived link that was primary but would
// collide with a primary created in the meantime is revived with is_primary =
// false and reported, so restore never fails on the partial unique index.
func reviveContactEntityLinks(tx *gorm.DB, userID uint, ids []string, now time.Time) ([]RestoreSkip, error) {
	counts := map[string]int{}
	for _, id := range ids {
		var link models.ContactEntityLink
		err := tx.Unscoped().Where("id = ? AND user_id = ? AND deleted_at IS NOT NULL", id, userID).First(&link).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var live int64
		if err := tx.Model(&models.Entity{}).Where("id = ? AND user_id = ?", link.EntityID, userID).Count(&live).Error; err != nil {
			return nil, err
		}
		if live == 0 {
			counts[RestoreReasonEntityGone]++
			continue
		}
		if err := tx.Model(&models.Contact{}).Where("user_id = ? AND vcard_uid = ?", userID, link.ContactVCardUID).Count(&live).Error; err != nil {
			return nil, err
		}
		if live == 0 {
			counts[RestoreReasonContactGone]++
			continue
		}
		updates := map[string]any{"deleted_at": nil, "deletion_batch_id": nil, "updated_at": now}
		if link.IsPrimary {
			var clash int64
			if err := tx.Model(&models.ContactEntityLink{}).
				Where("user_id = ? AND contact_vcard_uid = ? AND kind = ? AND is_primary = 1 AND id <> ?", userID, link.ContactVCardUID, link.Kind, link.ID).
				Count(&clash).Error; err != nil {
				return nil, err
			}
			if clash > 0 {
				updates["is_primary"] = false
				counts[RestoreReasonPrimaryDemoted]++
			}
		}
		if err := tx.Unscoped().Model(&models.ContactEntityLink{}).Where("id = ? AND user_id = ?", link.ID, userID).
			Updates(updates).Error; err != nil {
			return nil, err
		}
		// revision/etag move with the revive (the delete bumped them too) so an
		// If-Match client never holds a stale ETag. Both are <-:create, which a
		// GORM update silently drops, so this is a raw statement like the delete
		// cascade's.
		if err := tx.Exec(`UPDATE contact_entity_links SET revision = revision + 1,
			etag = 'e-' || id || '-' || (revision + 1) WHERE id = ? AND user_id = ?`, link.ID, userID).Error; err != nil {
			return nil, err
		}
	}
	reasons := make([]string, 0, len(counts))
	for r := range counts {
		reasons = append(reasons, r)
	}
	sort.Strings(reasons)
	out := make([]RestoreSkip, 0, len(reasons))
	for _, r := range reasons {
		out = append(out, RestoreSkip{Table: "contact_entity_links", Reason: r, Count: counts[r]})
	}
	return out, nil
}
