package models

import "gorm.io/gorm"

// Attachment is one file/document attached to a contact (N7, docs/fork-plan/
// tickets/29-N7-attachments.md). The row is metadata; the bytes live on disk
// under a server-generated UUID filename (StoredName) in the configured
// attachments directory — never a user-supplied name reaching filepath.Join.
//
// User-authored content -> soft-delete per T26. The on-disk file is removed
// at delete time by the controllers/cascade (the model has no access to the
// attachments directory); the tombstone keeps the T17 change feed convergent.
type Attachment struct {
	gorm.Model
	UserID          uint   `gorm:"not null;index" json:"-"`
	ContactVCardUID string `gorm:"column:contact_vcard_uid;not null;index" json:"contact_vcard_uid"`

	// StoredName is the server-generated UUID filename on disk. Never derived
	// from user input (OriginalName is display-only).
	StoredName string `gorm:"column:stored_name;not null" json:"-"`

	OriginalName string `gorm:"column:original_name;not null" json:"original_name"`
	ContentType  string `gorm:"column:content_type;not null" json:"content_type"`
	SizeBytes    int64  `gorm:"column:size_bytes;not null" json:"size_bytes"`
}
