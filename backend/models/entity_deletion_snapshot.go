package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// EntityDeletionSnapshotVersion is the payload shape entity restore understands.
const EntityDeletionSnapshotVersion = 1

// EntityDeletionSnapshot is the ADR 0037 mechanism at entity scale (ADR 0043
// decision 6): written in the same transaction as the entity delete, it carries
// the hard-deleted custom field values (JSON payload, encrypted at rest because
// they can be private/secret) so a restore can re-create them, and the batch id
// stamped on the tombstoned links. Hard delete: it lives exactly as long as the
// entity is in the trash.
type EntityDeletionSnapshot struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	UserID          uint      `gorm:"not null;uniqueIndex:idx_entity_deletion_snapshots_entity,priority:1" json:"-"`
	EntityID        string    `gorm:"not null;uniqueIndex:idx_entity_deletion_snapshots_entity,priority:2" json:"entity_id"`
	DeletionBatchID string    `gorm:"column:deletion_batch_id;not null" json:"deletion_batch_id"`
	TrashedAt       time.Time `gorm:"not null" json:"trashed_at"`
	PayloadVersion  int       `gorm:"not null" json:"payload_version"`
	Payload         string    `gorm:"not null;serializer:encrypted" json:"-"`
}

func (s *EntityDeletionSnapshot) BeforeCreate(tx *gorm.DB) error {
	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	return nil
}
