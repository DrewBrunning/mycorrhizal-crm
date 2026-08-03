package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Sync status values stored on ExternalIdentity.SyncStatus. These describe
// the *link's* own sync lifecycle (idle → syncing → synced/error), which is
// the concern of the integration that owns the link, not of this model.
const (
	ExternalIdentitySyncStatusIdle    = "idle"
	ExternalIdentitySyncStatusSyncing = "syncing"
	ExternalIdentitySyncStatusSynced  = "synced"
	ExternalIdentitySyncStatusError   = "error"
)

// ExternalIdentity is the generic integration substrate's identity-link
// entity (docs/fork-plan/91-envelope-data-model.md §91.12): "this contact IS
// this thing in that external system." One Contact (by VCardUID) may have
// many identities across many systems — the "identity hub" of §90 — so no
// integration ever grows bespoke columns (ticket T14's whole reason for
// existing).
//
// EntityID references Contact.VCardUID (models/contact.go), never the numeric
// ID — the graph invariant (§90 D3) every WP-80+ entity follows.
//
// System-specific data (e.g. an Immich person's name) belongs in Metadata,
// a map[string]interface{} JSON column following RelationshipEdge.Metadata's
// exact pattern (models/relationship_edge.go) — NOT in a per-system column.
// A column named immich_* here would mean the abstraction has failed.
//
// UUID-string-primary-key entity, generated in BeforeCreate like
// RelationshipEdge. No soft-delete: this is an edge/join-shaped row keyed by
// a natural (system, external_id, user_id) composite, so a dead link must be
// re-creatable after deletion without fighting a unique-index ghost (T26).
type ExternalIdentity struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	UserID uint `gorm:"not null;index" json:"-"`

	// EntityID is the contact this identity belongs to, by Contact.VCardUID.
	EntityID string `gorm:"column:entity_id;not null;index" json:"entity_id" validate:"required,uuid4"`

	// System names the integration ("immich", "paperless", ...) — an open
	// classifier, deliberately not a oneof: new integrations must not need a
	// schema or validator change to adopt the substrate.
	System string `gorm:"not null;index" json:"system" validate:"required,min=1,max=64"`

	// ExternalID is the identifier of this thing inside that system (an
	// Immich person UUID, a Paperless document UUID, ...).
	ExternalID string `gorm:"column:external_id;not null" json:"external_id" validate:"required,min=1,max=255"`

	// URL is an optional deep-link back into the external system, rendered as
	// the "open in Immich" affordance. User-influenced, so it is safeurl-
	// validated on write; anything this app *fetches* from it still goes
	// through the SSRF-guarded transport (httputil/fetch.go).
	URL string `json:"url,omitempty" validate:"omitempty,max=2000,safeurl"`

	// Metadata is the free-form JSON payload for system-specific data
	// (person name, photo count cache, sync offsets, ...). Same gorm
	// serializer mechanism as RelationshipEdge.Metadata.
	Metadata map[string]interface{} `gorm:"type:text;serializer:json" json:"metadata,omitempty"`

	// SyncStatus + LastSyncedAt track the integration's sync lifecycle.
	SyncStatus   string     `gorm:"column:sync_status;not null;default:idle" json:"sync_status" validate:"omitempty,oneof=idle syncing synced error"`
	LastSyncedAt *time.Time `gorm:"column:last_synced_at" json:"last_synced_at,omitempty"`
}

// TableName pins the singular table name (GORM would pluralize to
// external_identities, which matches — but pinning guards the silent
// name-mismatch class that broke ConversationAgenda).
func (ExternalIdentity) TableName() string {
	return "external_identities"
}

// BeforeCreate generates a UUID for new identities, mirroring
// RelationshipEdge's own BeforeCreate.
func (e *ExternalIdentity) BeforeCreate(tx *gorm.DB) error {
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	return nil
}

// ExternalIdentityInput is the DTO for creating/updating an
// ExternalIdentity. System + ExternalID are the natural key the unique
// constraint is built on; creating a duplicate pair is a 409 in the
// controller (like CircleMember), not a sniffed constraint error.
type ExternalIdentityInput struct {
	EntityID   string                 `json:"entity_id" validate:"required,uuid4"`
	System     string                 `json:"system" validate:"required,min=1,max=64"`
	ExternalID string                 `json:"external_id" validate:"required,min=1,max=255"`
	URL        string                 `json:"url,omitempty" validate:"omitempty,max=2000,safeurl"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	SyncStatus string                 `json:"sync_status,omitempty" validate:"omitempty,oneof=idle syncing synced error"`
}
