package models

import (
	"fmt"
	"time"

	"github.com/google/uuid"
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

	// UUID is the stable portable identity the account bundle carries for a
	// note (issue #1260, ADR 0028 Decision 3). Generated in BeforeCreate when
	// empty; migration 000067 adds the column and backfills every existing
	// row. Kept off the existing note wire shape (json:"-") — the bundle DTO
	// reads the field directly.
	UUID string `gorm:"column:uuid;index" json:"-"`

	// Revision is the monotonic per-row write counter (issue #591, CON-01a —
	// docs/adrs/0006-revision-token-schema.md): starts at 1 on create,
	// incremented on every persisted write. Migration 000044 adds the column.
	// Exposed read-only on the wire as `revision`.
	Revision int64 `gorm:"column:revision;<-:create;not null;default:1" json:"revision"`

	// ETag is the CardDAV/CalDAV-style sync-conflict token derived from
	// Revision (ADR 0006): e-{id}-{revision}. Notes have no DAV surface yet,
	// but the token exists for the sync/merge surface that will need it.
	// Explicit column tag guards the `etag` vs GORM-derived `e_tag` mismatch
	// (CLAUDE.md backend trap 1). Migration 000044 adds the column.
	// `<-:create` (CON-01, ADR 0018): see Contact.ETag's comment
	// (models/contact.go) for why an ordinary Save()/Updates() must never be
	// able to write this column.
	ETag string `gorm:"column:etag;<-:create" json:"-"`

	// revisionStampedOnCreate: transient marker set by AfterCreate, consumed
	// by the AfterSave GORM fires right after on a Create (see
	// Contact.revisionStampedOnCreate). Keeps a create at revision 1.
	revisionStampedOnCreate bool

	// Deleted is the T17 change-feed tombstone marker, set by the list
	// handler when it reads a row with Unscoped() that has a non-null
	// deleted_at. gorm:"-" keeps it out of the schema; it exists purely so an
	// incremental sync client can apply the deletion.
	Deleted bool `gorm:"-" json:"deleted,omitempty"`
}

// BeforeCreate generates the stable portable UUID for a new note (issue
// #1260) when the caller did not supply one. An account-bundle import supplies
// the source row's UUID so a re-export is comparable; everything else gets a
// fresh one.
func (n *Note) BeforeCreate(tx *gorm.DB) error {
	if n.UUID == "" {
		n.UUID = uuid.New().String()
	}
	return nil
}

// AfterCreate stamps the initial revision and derives the ETag from it (ADR
// 0006), mirroring Contact.AfterCreate. UpdateColumns bypasses GORM's update
// hooks, so this cannot recursively trigger AfterSave. The marker tells the
// AfterSave that follows on a Create not to bump (a create is revision 1).
func (n *Note) AfterCreate(tx *gorm.DB) error {
	n.Revision = 1
	n.ETag = fmt.Sprintf("e-%d-%d", n.ID, n.Revision)
	n.revisionStampedOnCreate = true
	// tx.Table(...), not tx.Model(n): revision/etag are `<-:create` (CON-01,
	// ADR 0018), so a Model-scoped statement would have this write filtered
	// out by GORM's own field-permission check.
	return tx.Table("notes").Where("id = ?", n.ID).UpdateColumns(map[string]any{"revision": n.Revision, "etag": n.ETag}).Error
}

// AfterDelete advances updated_at on a soft delete so T17 change feeds see
// the tombstone. GORM's soft-delete UPDATE only writes deleted_at (soft_delete.go:
// SoftDeleteDeleteClause adds a single clause.Set for deleted_at), leaving
// updated_at at its pre-delete value — a feed cursor stored after the row was
// created would sit ahead of the tombstone and the deletion would silently
// never propagate (the exact trap T17 calls out). Unscoped so the UPDATE can
// touch a now-soft-deleted row; UpdateColumn bypasses hooks, so this cannot
// recurse. Hard deletes (Unscoped Delete) fire with a zero-value DeletedAt and
// are skipped. Bulk soft deletes do NOT: GORM's soft-delete clause sets
// DeletedAt on the zero-value receiver, so they reach auditAfterDelete, which
// skips the zero-identity event centrally (issue #1471).
func (n *Note) AfterDelete(tx *gorm.DB) error {
	if !n.DeletedAt.Valid {
		return nil
	}
	auditAfterDelete(tx, AuditEntityNote, fmt.Sprintf("%d", n.ID), n.UserID, n)
	return tx.Model(&Note{}).Unscoped().Where("id = ?", n.ID).UpdateColumn("updated_at", time.Now()).Error
}
