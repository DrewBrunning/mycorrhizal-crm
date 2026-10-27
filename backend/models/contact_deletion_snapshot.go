package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ContactDeletionSnapshotVersion is the payload shape restore understands. A
// snapshot with another version is refused (409) rather than half-restored
// (ADR 0037 decision 4).
const ContactDeletionSnapshotVersion = 1

// ContactDeletionSnapshot is the per-deletion record that makes a trashed
// contact restorable (ADR 0037). It is written in the same transaction as the
// delete cascade, so it exists if and only if the delete committed. Payload is
// the JSON of services.ContactDeletionPayload, encrypted at rest because the
// copied rows can include private/secret field values. Hard delete: the row
// lives exactly as long as the contact is in the trash.
type ContactDeletionSnapshot struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	UserID          uint      `gorm:"not null;uniqueIndex:idx_contact_deletion_snapshots_contact,priority:1" json:"-"`
	ContactID       uint      `gorm:"not null;uniqueIndex:idx_contact_deletion_snapshots_contact,priority:2" json:"contact_id"`
	ContactVCardUID string    `gorm:"column:contact_vcard_uid;not null" json:"contact_vcard_uid"`
	TrashedAt       time.Time `gorm:"not null" json:"trashed_at"`
	PayloadVersion  int       `gorm:"not null" json:"payload_version"`
	Payload         string    `gorm:"not null;serializer:encrypted" json:"-"`
}

func (s *ContactDeletionSnapshot) BeforeCreate(tx *gorm.DB) error {
	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	return nil
}
