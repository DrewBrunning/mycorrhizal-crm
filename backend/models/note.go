package models

import (
	"time"

	"gorm.io/gorm"
)

// Note struct to represent notes attached to a contact
type Note struct {
	gorm.Model
	UserID    uint      `gorm:"not null;index" json:"-"`
	Content   string    `json:"content" validate:"required,min=1,max=5000"`
	Date      time.Time `json:"date" validate:"required"`
	ContactID *uint     `json:"contact_id"`
	Contact   Contact   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"contact,omitempty"`

	// Deleted is the T17 change-feed tombstone marker, set by the list
	// handler when it reads a row with Unscoped() that has a non-null
	// deleted_at. gorm:"-" keeps it out of the schema; it exists purely so an
	// incremental sync client can apply the deletion.
	Deleted bool `gorm:"-" json:"deleted,omitempty"`
}

// AfterDelete advances updated_at on a soft delete so T17 change feeds see
// the tombstone. GORM's soft-delete UPDATE only writes deleted_at (soft_delete.go:
// SoftDeleteDeleteClause adds a single clause.Set for deleted_at), leaving
// updated_at at its pre-delete value — a feed cursor stored after the row was
// created would sit ahead of the tombstone and the deletion would silently
// never propagate (the exact trap T17 calls out). Unscoped so the UPDATE can
// touch a now-soft-deleted row; UpdateColumn bypasses hooks, so this cannot
// recurse. Hard deletes (Unscoped Delete) and bulk deletes fire with a
// zero-value DeletedAt and are skipped.
func (n *Note) AfterDelete(tx *gorm.DB) error {
	if !n.DeletedAt.Valid {
		return nil
	}
	return tx.Model(&Note{}).Unscoped().Where("id = ?", n.ID).UpdateColumn("updated_at", time.Now()).Error
}
