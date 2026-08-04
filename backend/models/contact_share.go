package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ContactShare status values.
const (
	ContactShareStatusPending  = "pending"
	ContactShareStatusAccepted = "accepted"
	ContactShareStatusDeclined = "declined"
)

// ContactShare is a one-time, filtered copy of a contact offered from one
// user to another user on the same instance (P1, docs/fork-plan/tickets/
// 31-P1-contact-sharing.md). This is NOT a standing/live share — Payload is
// a frozen snapshot serialized once at creation time; editing the original
// Contact afterward has no effect on a pending or accepted share. A
// standing/permissioned share is P1b (deferred, XL — see 37-deferred.md).
//
// UUID-string-primary-key entity, following LifeEvent/Household's template:
// ID generated in BeforeCreate.
//
// No soft delete: this ticket adds no delete/withdraw endpoint — declining
// is the "soft" outcome (flips Status, the row survives so the sender's
// offer isn't silently destroyed, per the ticket's own trap). The only place
// a ContactShare is ever removed is DeleteUser's cascade sweep
// (admin_user_controller.go), matching the Reminder/Note/Activity
// hard-delete-on-account-removal precedent there. It is NOT part of
// contact_controller.go's deleteContactAssociations: Payload has no FK to
// contacts (it is a frozen snapshot, not a live reference), so deleting the
// original Contact correctly leaves an already-shared copy untouched.
type ContactShare struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	FromUserID uint `gorm:"not null;index" json:"from_user_id"`
	ToUserID   uint `gorm:"not null;index" json:"to_user_id"`

	// ContactDisplayName is an immutable snapshot of the shared contact's
	// display name at share-creation time — Payload is opaque serialized
	// JSContact JSON, and the original Contact may later be renamed or
	// deleted; the inbox/outbox list needs a name to show without parsing
	// Payload on every list call.
	ContactDisplayName string `json:"contact_display_name"`

	// Payload is the T9-filtered JSContact export of the shared contact
	// (jscontact.Adapter{}.Export, via the same models.RecordForContactFiltered
	// / ApplyFieldSelection filter T9 built), serialized ONCE at
	// share-creation time and never re-derived. json:"-": never round-tripped
	// through the ordinary list/get JSON responses — the accept handler reads
	// it directly off the loaded DB row.
	//
	// No payload schema-version field: an old pending share whose Card shape
	// has since drifted degrades gracefully through ParseJSContact's existing
	// per-row validation-error path (surfaces as an error row in the accept
	// preview, safe to decline) rather than crashing — a deliberate choice,
	// not an oversight, given no other payload in this codebase is versioned
	// either.
	Payload string `gorm:"type:text;not null" json:"-"`

	Status      string     `gorm:"not null;default:'pending';index" json:"status" validate:"omitempty,oneof=pending accepted declined"`
	RespondedAt *time.Time `json:"responded_at,omitempty"`
}

// BeforeCreate generates a UUID for new ContactShares, mirroring
// LifeEvent/Household's own BeforeCreate.
func (s *ContactShare) BeforeCreate(tx *gorm.DB) error {
	if s.ID == "" {
		s.ID = uuid.New().String()
	}
	return nil
}

// ContactShareInput is the body of POST /contact-shares. Sections is
// validated against models.FieldSections (via FieldSelection.Enable) in the
// controller rather than a struct tag, so an unknown token 400s with the
// same message parseExportFieldSelection's query-param path produces,
// instead of duplicating the token list in a validate tag that could drift.
type ContactShareInput struct {
	ToUserID         uint     `json:"to_user_id" validate:"required"`
	VCardUID         string   `json:"vcard_uid" validate:"required,uuid4"`
	Sections         []string `json:"sections" validate:"required,min=1"`
	IncludeSensitive bool     `json:"include_sensitive"`
}
