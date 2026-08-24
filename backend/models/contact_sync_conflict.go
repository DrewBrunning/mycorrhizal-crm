package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ContactSyncConflict field keys stored on ContactSyncConflict.Field — the
// flat contact fields the CardDAV sync conflict detector tracks (issue #395).
// These MUST be kept in sync by hand with:
//   - services/contact_sync_service.go's syncConflictFieldKeys (the canonical
//     list the snapshot/diff iterates), and
//   - frontend/src/api/contactSyncConflicts.ts + the frontend's
//     `syncConflicts.field.*` i18n labels (this repo has no dynamic
//     type-list endpoint; every enum is a hardcoded frontend mirror).
//
// The set deliberately covers exactly the flat fields a user edits in the
// contact UI and that the CardDAV full-replace (ApplyRecordToContact) can
// overwrite. Card-only data with no flat home (SpeakToAs, PersonalInfo,
// SocialProfiles, ...) is out of scope: it is overwritten too, but there is
// no flat field a restore could write it back to.
const (
	SyncConflictFieldFirstname          = "firstname"
	SyncConflictFieldLastname           = "lastname"
	SyncConflictFieldMiddlename         = "middlename"
	SyncConflictFieldPrefix             = "prefix"
	SyncConflictFieldSuffix             = "suffix"
	SyncConflictFieldNickname           = "nickname"
	SyncConflictFieldOrganization       = "organization"
	SyncConflictFieldDepartment         = "department"
	SyncConflictFieldJobTitle           = "job_title"
	SyncConflictFieldRole               = "role"
	SyncConflictFieldEmail              = "email"
	SyncConflictFieldPhone              = "phone"
	SyncConflictFieldAddress            = "address"
	SyncConflictFieldURL                = "url"
	SyncConflictFieldIMPP               = "impp"
	SyncConflictFieldBirthday           = "birthday"
	SyncConflictFieldAnniversary        = "anniversary"
	SyncConflictFieldCircles            = "circles"
	SyncConflictFieldHowWeMet           = "how_we_met"
	SyncConflictFieldWorkInformation    = "work_information"
	SyncConflictFieldContactInformation = "contact_information"
)

// ContactSyncConflict status values stored on ContactSyncConflict.Status.
const (
	SyncConflictStatusPending   = "pending"
	SyncConflictStatusDismissed = "dismissed"
)

// ContactSyncConflict is one recorded instance of a CardDAV sync discarding
// a local edit (issue #395): the reconcile path detected that a field's
// current value differed from the last-synced value (i.e. a local edit) and
// that the incoming remote vCard carries a different value, then applied the
// full-replace policy and recorded the before/after here so the user can
// review and re-apply.
//
// LocalValue is the pre-sync value that was overwritten; RemoteValue is the
// synced value that replaced it. Array fields (email/phone/address/url/impp/
// circles) are stored as their JSON array, the same encoding the sync
// snapshot uses, so RestoreContactSyncConflict can write them straight back.
//
// System-generated, edge-shaped row (CLAUDE.md backend trap 7): no soft
// delete, no natural-key unique constraint — mirrors
// reach_out_suggestions/RelationshipEdge's precedent; "dismiss" is a status
// update, never a delete. UUID-string PK generated in BeforeCreate.
//
// Sensitivity: the values here are the user's own synced contact data, and
// the diff that produces them runs on stored fields only — never on the
// read-time projected view (RecordForContactFiltered), so private/secret
// relationship edges, hobby preferences, or custom-field values can never
// appear in a conflict notice.
type ContactSyncConflict struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	UserID uint `gorm:"not null;index" json:"-"`

	// SubscriptionID is the ContactSubscription this conflict came from;
	// ContactID the numeric Contact row (the same reference shape
	// ContactSyncLink.ContactID uses).
	SubscriptionID uint `gorm:"column:subscription_id;not null;index" json:"subscription_id"`
	ContactID      uint `gorm:"column:contact_id;not null;index" json:"contact_id"`

	// Field ∈ the SyncConflictField* constants above.
	Field string `gorm:"not null" json:"field" validate:"required"`

	// LocalValue/RemoteValue are the overwritten and overwriting values.
	LocalValue  string `gorm:"column:local_value;not null;default:''" json:"local_value"`
	RemoteValue string `gorm:"column:remote_value;not null;default:''" json:"remote_value"`

	// Status ∈ pending|dismissed (the SyncConflictStatus* constants above).
	Status string `gorm:"not null;default:pending;index" json:"status" validate:"required,oneof=pending dismissed"`
}

// BeforeCreate generates a UUID for new conflicts, mirroring
// ReachOutSuggestion/RelationshipEdge's own BeforeCreate.
func (c *ContactSyncConflict) BeforeCreate(tx *gorm.DB) error {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	return nil
}

// ContactSyncConflictResponse is a pending conflict enriched with the
// contact's display name/vcard UID/photo and the subscription's name — the
// same N+1-avoidance shape ReachOutSuggestionResponse uses for the
// dashboard.
type ContactSyncConflictResponse struct {
	ContactSyncConflict
	ContactID       uint   `json:"contact_id"`
	ContactVCardUID string `json:"contact_vcard_uid"`
	ContactName     string `json:"contact_name"`
	PhotoThumbnail  string `json:"photo_thumbnail,omitempty"`
	// SubscriptionName is the address book the conflict came from.
	SubscriptionName string `json:"subscription_name"`
}
