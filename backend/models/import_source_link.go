package models

import "time"

// ImportSourceLinkEntityKind values stored on ImportSourceLink.EntityKind —
// the local entity classes a source import can produce.
const (
	ImportSourceLinkKindContact      = "contact"
	ImportSourceLinkKindRelationship = "relationship"
	ImportSourceLinkKindHousehold    = "household"
	ImportSourceLinkKindCircle       = "circle"
	ImportSourceLinkKindTag          = "tag"
	ImportSourceLinkKindGift         = "gift"
	ImportSourceLinkKindPreference   = "preference"
	ImportSourceLinkKindActivity     = "activity"
	ImportSourceLinkKindNote         = "note"
	ImportSourceLinkKindReminder     = "reminder"
	ImportSourceLinkKindCustomField  = "custom_field"
)

// ImportSourceLink is the idempotency ledger for source imports (issues #351,
// #353): one row per local entity an import created, keyed by its identity in
// the SOURCE system. Re-running the same import skips any (system, external_id)
// pair already present, so an import never duplicates (CON-04 / issue #459
// applied to source imports).
//
// Hard-delete, graph-adjacent join-row class (ADR 0004): the row is a ledger
// fact, not user-authored content, and its natural key (system, external_id,
// user_id) must be re-creatable after deletion. uint primary key, no UUID —
// it is bookkeeping, never referenced by another table.
//
// Every field carries an explicit gorm:"column:..." tag: GORM's name
// derivation disagrees with hand-written migration SQL for acronyms/IDs
// (CLAUDE.md backend trap #1).
type ImportSourceLink struct {
	ID        uint      `gorm:"column:id;primarykey" json:"id"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`

	UserID uint `gorm:"column:user_id;not null;index;uniqueIndex:idx_import_source_links_system_ext_user,priority:1" json:"-"`

	// System is the importing source ("meerkat", "monica", ...) — an open
	// classifier, deliberately not a oneof: a third importer must not need a
	// validator or schema change to join the framework.
	System string `gorm:"column:system;not null;uniqueIndex:idx_import_source_links_system_ext_user,priority:2" json:"system" validate:"required,min=1,max=64"`

	// ExternalID is the source row's identity, namespaced per entity kind
	// (e.g. "contact/7", "note/12") so different kinds never collide.
	ExternalID string `gorm:"column:external_id;not null;uniqueIndex:idx_import_source_links_system_ext_user,priority:3" json:"external_id" validate:"required,min=1,max=255"`

	// EntityKind is the local entity class the row became
	// (ImportSourceLinkKind* constants above).
	EntityKind string `gorm:"column:entity_kind;not null" json:"entity_kind"`

	// EntityUID is the local identity: a Contact.VCardUID, a UUID-PK entity's
	// ID, or "id:<n>" for a uint-PK row (notes/reminders). Informational.
	EntityUID string `gorm:"column:entity_uid;not null" json:"entity_uid"`
}
