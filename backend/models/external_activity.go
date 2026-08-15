package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Provenance values stored on ExternalActivity.Provenance.
const (
	// ExternalActivityProvenanceExternal means the event was contributed by
	// the external system itself (a photo appeared, a file was watched).
	ExternalActivityProvenanceExternal = "external"
	// ExternalActivityProvenanceUser means the event was recorded by the user
	// (e.g. an integration-provided "mark as meaningful interaction").
	ExternalActivityProvenanceUser = "user"
)

// Sync state values stored on ExternalActivity.SyncState.
const (
	ExternalActivitySyncStateSynced  = "synced"
	ExternalActivitySyncStatePending = "pending"
)

// ExternalActivity is the generic integration substrate's event entity
// (docs/adrs/0001-neutral-hub-and-spoke-contact-model.md): something that happened
// in an external system, linkable into a contact's timeline  without
// the CRM owning the source data — the photo stays in Immich, the CRM stores
// a reference + a timeline entry.
//
// EntityID references Contact.VCardUID (never the numeric ID). SourceSystem +
// ExternalID are the natural key a re-sync upserts on; Type is an open
// classifier (photo-appearance, gps-visit, media-watched, ...); Payload is
// the JSON summary/provenance (e.g. {asset_id, thumbnail, url}).
//
// UUID-string-primary-key entity, generated in BeforeCreate like
// RelationshipEdge. No soft-delete: edge/join-shaped row keyed by a natural
// composite, so a re-import after deletion must not hit a unique-index ghost
// (T26).
type ExternalActivity struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	UserID uint `gorm:"not null;index" json:"-"`

	// EntityID is the contact this event concerns, by Contact.VCardUID.
	EntityID string `gorm:"column:entity_id;not null;index" json:"entity_id" validate:"required,uuid4"`

	// SourceSystem names the integration that contributed the event.
	SourceSystem string `gorm:"column:source_system;not null;index" json:"source_system" validate:"required,min=1,max=64"`

	// ExternalID is the event's identifier inside that system (an Immich
	// asset UUID, a Paperless document UUID, ...).
	ExternalID string `gorm:"column:external_id;not null" json:"external_id" validate:"required,min=1,max=255"`

	// Type is an open classifier ("photo-appearance", "media-watched", ...).
	Type string `gorm:"not null;index" json:"type" validate:"required,min=1,max=64"`

	// OccurredAt is when the event happened in the external system — the
	// timestamp the timeline sorts on.
	OccurredAt time.Time `gorm:"column:occurred_at;not null;index" json:"occurred_at" validate:"required"`

	// Payload is the JSON summary (e.g. {asset_id, thumb_url, name}).
	Payload map[string]interface{} `gorm:"type:text;serializer:json" json:"payload,omitempty"`

	// Provenance is who/what contributed the event; SyncState tracks whether
	// the external system has acknowledged it.
	Provenance string `gorm:"not null;default:external" json:"provenance" validate:"omitempty,oneof=external user"`
	SyncState  string `gorm:"column:sync_state;not null;default:synced" json:"sync_state" validate:"omitempty,oneof=synced pending"`
}

// TableName pins the singular table name.
func (ExternalActivity) TableName() string {
	return "external_activities"
}

// BeforeCreate generates a UUID for new activities, mirroring
// RelationshipEdge's own BeforeCreate.
func (a *ExternalActivity) BeforeCreate(tx *gorm.DB) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	return nil
}

// ExternalActivityInput is the DTO for creating/updating an
// ExternalActivity. SourceSystem + ExternalID form the natural key; a
// duplicate pair is a 409 in the controller.
type ExternalActivityInput struct {
	EntityID     string                 `json:"entity_id" validate:"required,uuid4"`
	SourceSystem string                 `json:"source_system" validate:"required,min=1,max=64"`
	ExternalID   string                 `json:"external_id" validate:"required,min=1,max=255"`
	Type         string                 `json:"type" validate:"required,min=1,max=64"`
	OccurredAt   time.Time              `json:"occurred_at" validate:"required"`
	Payload      map[string]interface{} `json:"payload,omitempty"`
	Provenance   string                 `json:"provenance,omitempty" validate:"omitempty,oneof=external user"`
	SyncState    string                 `json:"sync_state,omitempty" validate:"omitempty,oneof=synced pending"`
}
