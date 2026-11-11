package services

import (
	"encoding/json"
	"time"

	"mycorrhizal/models"

	"gorm.io/gorm"
)

// TrashedEntityEntry is one restorable soft-deleted entity (issue #2446).
type TrashedEntityEntry struct {
	Entity    models.Entity
	DeletedAt time.Time
	// MergedInto / MergedIntoName name the survivor when the entity is a merge
	// loser (ADR 0043 decision 9); empty for a plain delete or a survivor that
	// is itself gone.
	MergedInto     string
	MergedIntoName string
}

// ListTrashedEntities returns the caller's soft-deleted entities that have a
// deletion snapshot (the same restorable rule as ListTrash for contacts),
// newest deletion first. Every query is scoped by user_id.
func ListTrashedEntities(db *gorm.DB, userID uint, offset, limit int) ([]TrashedEntityEntry, int64, error) {
	base := func() *gorm.DB {
		return db.Unscoped().Model(&models.Entity{}).
			Joins("JOIN entity_deletion_snapshots s ON s.entity_id = entities.id AND s.user_id = entities.user_id").
			Where("entities.user_id = ? AND entities.deleted_at IS NOT NULL", userID)
	}
	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []models.Entity
	if err := base().Select("entities.*").Order("s.trashed_at DESC, s.id").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	entries := make([]TrashedEntityEntry, 0, len(rows))
	for _, e := range rows {
		var snap models.EntityDeletionSnapshot
		if err := db.Where("user_id = ? AND entity_id = ?", userID, e.ID).First(&snap).Error; err != nil {
			return nil, 0, err
		}
		entry := TrashedEntityEntry{Entity: e, DeletedAt: snap.TrashedAt}
		if snap.PayloadVersion == models.EntityDeletionSnapshotVersion {
			var payload EntityDeletionPayload
			if json.Unmarshal([]byte(snap.Payload), &payload) == nil && payload.MergedInto != "" {
				entry.MergedInto = payload.MergedInto
				var survivor models.Entity
				if err := db.Unscoped().Select("name").Where("id = ? AND user_id = ?", payload.MergedInto, userID).First(&survivor).Error; err == nil {
					entry.MergedIntoName = survivor.Name
				}
			}
		}
		entries = append(entries, entry)
	}
	return entries, total, nil
}
