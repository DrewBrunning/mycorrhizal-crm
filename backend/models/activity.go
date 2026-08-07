package models

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Conventional (not validated — same open-classifier reasoning as
// CRMEnvelope.Kind (WP-82) and HouseholdMember.Role (WP-83)) values for
// Activity.Type. docs/fork-plan/91-envelope-data-model.md §91.7 lists these
// with a trailing "…", signalling an open, extensible set rather than a
// closed system state.
const (
	InteractionTypeCall           = "call"
	InteractionTypeVideoCall      = "video_call"
	InteractionTypeVisit          = "visit"
	InteractionTypeMeal           = "meal"
	InteractionTypeGift           = "gift"
	InteractionTypePhoto          = "photo"
	InteractionTypeMessage        = "message"
	InteractionTypeSharedActivity = "shared_activity"
)

// Activity struct to represent shared activities with one or more contacts
// — "Interaction" in docs/fork-plan/91-envelope-data-model.md §91.7 ("what
// happened between us"). §91.7 generalizes rather than replaces this v1:
// UUID/Type/ExternalRef are additive columns alongside the existing int PK,
// following Contact.VCardUID's own precedent (contact.go) for adding a
// stable UUID identity to a table that already has production rows.
type Activity struct {
	gorm.Model
	UserID uint `gorm:"not null;index" json:"-"`

	// UUID is Interaction's stable external identity (§91.7 "id"), generated
	// in BeforeCreate for new rows; existing rows are backfilled by
	// migrations/000030_add_interaction_fields.
	UUID string `gorm:"column:uuid;index" json:"uuid,omitempty"`

	Title       string    `json:"title" validate:"required,min=1,max=200"`
	Description string    `json:"description" validate:"max=2000"`
	Location    string    `json:"location" validate:"max=300"`
	Date        time.Time `json:"date" validate:"required"`
	Contacts    []Contact `gorm:"many2many:activity_contacts;foreignKey:ID;joinForeignKey:ActivityID;References:ID;joinReferences:ContactID" json:"contacts,omitempty"`

	// Type is Interaction's classifier (§91.7), conventional/unvalidated —
	// see the InteractionType* constants above.
	Type string `json:"type,omitempty"`

	// ExternalRef optionally links this Interaction to an ExternalActivity
	// (§91.12, future WP-89) or an existing calendar_event_links row
	// (services/calendar_sync_service.go) — a plain opaque string reference,
	// no FK: the referenced tables belong to different, not-yet-built or
	// separately-owned subsystems.
	ExternalRef string `json:"external_ref,omitempty"`

	// ETag is the CalDAV sync-conflict token for this Interaction (T12a),
	// same role Contact.ETag plays for vCards. Explicit gorm column tag is
	// mandatory: without it GORM derives `e_tag` while migration 000041 names
	// the column `etag` — the exact silent mismatch that shipped broken for
	// ContactSyncLink.ETag and killed CardDAV incremental sync. Generated in
	// AfterCreate/AfterSave from ID + UpdatedAt (see below).
	ETag string `gorm:"column:etag" json:"-"`

	// Deleted is the T17 change-feed tombstone marker, set by the list
	// handler when it reads a row with Unscoped() that has a non-null
	// deleted_at. gorm:"-" keeps it out of the schema; it exists purely so an
	// incremental sync client can apply the deletion.
	Deleted bool `gorm:"-" json:"deleted,omitempty"`
}

// AfterDelete advances updated_at on a soft delete so T17 change feeds see
// the tombstone (see Note.AfterDelete's doc comment for the full rationale).
// Hard deletes and bulk deletes are skipped via the DeletedAt guard.
func (a *Activity) AfterDelete(tx *gorm.DB) error {
	if !a.DeletedAt.Valid {
		return nil
	}
	auditAfterDelete(tx, AuditEntityActivity, a.UUID, a.UserID, a)
	return tx.Model(&Activity{}).Unscoped().Where("id = ?", a.ID).UpdateColumn("updated_at", time.Now()).Error
}

// BeforeCreate generates a UUID for new Activities/Interactions, mirroring
// Household's own BeforeCreate (household.go) even though, unlike Household,
// Activity keeps its existing int primary key — only UUID is new here.
func (a *Activity) BeforeCreate(tx *gorm.DB) error {
	if a.UUID == "" {
		a.UUID = uuid.New().String()
	}
	return nil
}

// AfterCreate assigns the initial ETag now that ID/UpdatedAt are persisted,
// mirroring Contact.AfterCreate (contact.go). UpdateColumn bypasses GORM's
// update hooks, so this cannot recursively trigger AfterSave.
func (a *Activity) AfterCreate(tx *gorm.DB) error {
	a.ETag = fmt.Sprintf("e-%d-%d", a.ID, a.UpdatedAt.Unix())
	return tx.Model(a).UpdateColumn("etag", a.ETag).Error
}

// AfterSave refreshes the ETag on a real change only, exactly like
// Contact.AfterSave: recompute the would-be value from the persisted
// ID/UpdatedAt, and only write it back via UpdateColumn (no hook loop) when
// it actually differs from what's stored.
//
// Guard: a zero-value ID means this hook fired on a bulk
// Model(&Activity{}).Where(...).Update/Updates call, not on a real row — the
// receiver has no primary key, so UpdateColumn would widen to every row in
// the table. Never derive/write an ETag in that case (the caller is
// responsible for load-then-save updates that should refresh ETags).
func (a *Activity) AfterSave(tx *gorm.DB) error {
	if a.ID == 0 {
		return nil
	}
	// T18 audit fires first: the ETag UpdateColumn below swaps in a fresh
	// statement, which would otherwise wipe the audit's instance state.
	auditAfterSave(tx, AuditEntityActivity, a.UUID, a.UserID)
	newETag := fmt.Sprintf("e-%d-%d", a.ID, a.UpdatedAt.Unix())
	if newETag != a.ETag {
		a.ETag = newETag
		return tx.Model(a).UpdateColumn("etag", a.ETag).Error
	}
	return nil
}

// nonQualifyingInteractionTypes are passive/social-media-like interaction
// types that do not count toward a relationship-maintenance cadence (§91.10,
// future WP-94) — everything else counts by default. No consumer exists yet
// (cadence is unbuilt), matching how WP-80 defined RelationshipSource
// constants before WP-83 had a consumer for them.
var nonQualifyingInteractionTypes = map[string]bool{
	InteractionTypePhoto: true,
}

// Qualifying reports whether this Interaction counts toward a cadence policy
// (§91.7's "qualifying" field) — derived from Type, not stored.
func (a *Activity) Qualifying() bool {
	return !nonQualifyingInteractionTypes[a.Type]
}
