package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Kind values conventionally stored on OccasionObligation.Kind — an open
// classifier (unvalidated, same reasoning as LifeEvent.Type/
// Preference.Category): "card", "gift", "invite" are the three docs/adrs/0024-occasions.md names, but the
// column accepts any string so a future kind doesn't need a migration.
const (
	OccasionObligationKindCard   = "card"
	OccasionObligationKindGift   = "gift"
	OccasionObligationKindInvite = "invite"
)

// OccasionObligation is a standing, recurring obligation toward a contact —
// "this contact is on my holiday card list every year", "get them a
// birthday gift, ordered two weeks ahead" (docs/adrs/0024-occasions.md). It is a
// decision the user made, not a fact about the contact's life (that's
// LifeEvent) or a specific act of giving (that's Gift) — the missing
// "standing rule" layer ADR 0024 identified.
//
// UUID-string-primary-key entity, following Preference's exact template
// (preference.go): ID generated in BeforeCreate, soft-deletes. No natural-key
// unique constraint (a contact may have several obligations of the same
// Kind), so a soft-deleted row never blocks re-creation. No Revision/ETag:
// unlike LifeEvent/Reminder, this entity has no CardDAV/CalDAV sync surface
// and no stated conditional-write requirement — it follows Gift/Preference/
// CadencePolicy's simpler precedent, not LifeEvent's.
type OccasionObligation struct {
	ID        string         `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	UserID uint `gorm:"not null;index" json:"-"`

	// EntityID is the subject Contact, referenced by Contact.VCardUID — the
	// same graph invariant LifeEvent/Preference/Gift.EntityID follow.
	EntityID string `gorm:"column:entity_id;not null;index" json:"entity_id" validate:"required,uuid4"`

	// Kind is the open classifier above. Label is the human-readable name for
	// *this* obligation, since a contact can have several of the same Kind
	// ("Christmas card" vs. "Anniversary card").
	Kind  string `gorm:"not null" json:"kind" validate:"required,max=100"`
	Label string `gorm:"not null" json:"label" validate:"required,max=200"`

	// AnchorMonth/AnchorDay are the annual recurrence anchor, in the
	// obligation's own right (ADR 0024 part 2 — reuses ADR 0015 Rule 3/6's
	// existing annual month/day recurrence verbatim, no RRULE-lite). Both nil
	// is valid (an obligation whose date isn't fixed yet, e.g. "annual summer
	// BBQ, date TBD") and simply never surfaces on the upcoming-occasions
	// widget. Lexical range validation only (1-12 / 1-31), matching the
	// existing `birthday` validator's own lexical-not-calendar-aware
	// approach — the controller enforces "both or neither".
	AnchorMonth *int `gorm:"column:anchor_month" json:"anchor_month,omitempty" validate:"omitempty,min=1,max=12"`
	AnchorDay   *int `gorm:"column:anchor_day" json:"anchor_day,omitempty" validate:"omitempty,min=1,max=31"`

	// LinkedLifeEventID is an optional soft reference (no FK, verified to
	// belong to the user in the controller) to the LifeEvent this obligation
	// is anchored to — e.g. the `anniversary` life event. Purely
	// informational/traceability; mirrors Gift.LifeEventID.
	LinkedLifeEventID string `gorm:"column:linked_life_event_id;index" json:"linked_life_event_id,omitempty" validate:"omitempty,uuid4"`

	// LeadTimeDays is how many days before the anchor date this obligation
	// should surface as due ("gift by 12/18" = a Dec 25 anchor with
	// LeadTimeDays: 7).
	LeadTimeDays int `gorm:"not null;default:0" json:"lead_time_days" validate:"gte=0,lte=365"`

	// Active retires an obligation without deleting its history (e.g. "we
	// stopped exchanging holiday cards") — distinct from soft-delete, which
	// removes it from every surface including its own history.
	Active bool `gorm:"not null;default:true" json:"active"`

	// Sensitivity reuses the cross-cutting normal/private/secret set
	// (RelationshipSensitivity* — relationship_edge.go), driving the
	// query-level filter every sensitivity-bearing entity uses: anything
	// above normal is excluded from any list/export that leaves the
	// instance, re-includable only via ?include_sensitive=true.
	Sensitivity string `gorm:"not null;default:normal;index" json:"sensitivity" validate:"required,oneof=normal private secret"`

	// Notes is free-text context, encrypted at rest — mirrors
	// Preference.Notes/Gift.Notes.
	Notes string `gorm:"serializer:encrypted" json:"notes,omitempty" validate:"omitempty,max=2000"`

	// Deleted is the T17 change-feed tombstone marker, set by the list
	// handler when it reads a row with Unscoped() that has a non-null
	// deleted_at. gorm:"-" keeps it out of the schema; it exists purely so an
	// incremental sync client can apply the deletion.
	Deleted bool `gorm:"-" json:"deleted,omitempty"`
}

// TableName pins the plural table name explicitly — GORM's pluralizer
// happens to derive `occasion_obligations` from the struct name, but pinning
// it guards the same silent name-mismatch class that broke ConversationAgenda
// (CLAUDE.md backend trap 1).
func (OccasionObligation) TableName() string {
	return "occasion_obligations"
}

// AfterDelete advances updated_at on a soft delete so T17 change feeds see
// the tombstone (see Note.AfterDelete's doc comment for the full rationale).
// Hard deletes and bulk deletes are skipped via the DeletedAt guard.
func (o *OccasionObligation) AfterDelete(tx *gorm.DB) error {
	if !o.DeletedAt.Valid {
		return nil
	}
	return tx.Model(&OccasionObligation{}).Unscoped().Where("id = ?", o.ID).UpdateColumn("updated_at", time.Now()).Error
}

// BeforeCreate generates a UUID for new OccasionObligations, mirroring
// Preference's own BeforeCreate.
func (o *OccasionObligation) BeforeCreate(tx *gorm.DB) error {
	if o.ID == "" {
		o.ID = uuid.New().String()
	}
	return nil
}
