package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Gift statuses (§91.11 + ticket T20b). The state machine is linear
// (idea → purchased → given) but deliberately not enforced as one: an edit may
// move a gift between any two statuses. idea = "she mentioned she liked X"
// captured opportunistically (the whole point of the feature); purchased =
// bought but not yet handed over; given = what you actually gave, with its
// date; received = what they gave you — useful for reciprocity and for
// remembering to say thanks.
const (
	GiftStatusIdea      = "idea"
	GiftStatusPurchased = "purchased"
	GiftStatusGiven     = "given"
	GiftStatusReceived  = "received"
)

// Gift is a gift record against a contact (docs/fork-plan/
// 91-envelope-data-model.md §91.11, ticket T20b): "what did I give them last
// year?" — plus the occasion, an optional value with an explicit currency, and
// an optional link to the LifeEvent or Activity it relates to.
//
// UUID-string-primary-key entity, following LifeEvent's exact template
// (life_event.go): ID generated in BeforeCreate, soft-deletes. Gift records
// are user-authored content, not graph-adjacent join rows, so they soft-delete
// per T26 — and because they have no natural-key unique constraint (a contact
// may have many gifts), a soft-deleted row never blocks re-creation.
type Gift struct {
	ID        string         `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	UserID uint `gorm:"not null;index" json:"-"`

	// EntityID is the contact this gift concerns, referenced by
	// Contact.VCardUID — the same graph invariant every join/reference entity
	// follows (§90 D3). For a given/purchased gift the contact is the
	// recipient; for a received gift they are the giver. Kept as entity_id
	// (rather than §91.11's recipient_id) to match the LifeEvent/
	// ConversationAgenda/Preference template exactly, which is what T20b
	// instructs.
	EntityID string `gorm:"column:entity_id;not null;index" json:"entity_id" validate:"required,uuid4"`

	// Status is one of the GiftStatus* constants. Defaults to "idea" — a gift
	// idea is recorded opportunistically, mid conversation, without choosing a
	// state.
	Status string `gorm:"not null;default:idea;index" json:"status" validate:"omitempty,oneof=idea purchased given received"`

	// Occasion is free text ("birthday", "holiday", "housewarming", "their
	// 30th") — deliberately an open classifier rather than a closed enum: the
	// point of a personal CRM is that occasions are not enumerable.
	Occasion string `json:"occasion,omitempty" validate:"max=200"`

	// Description is the core content: for an idea, what they mentioned they
	// liked; for a given/received gift, what it actually was.
	Description string `gorm:"not null" json:"description" validate:"required,min=1,max=2000"`

	// URL is an optional link to the thing itself — the product page for an
	// idea captured as "she mentioned she liked this specific item" (T35).
	// User-supplied and rendered as an href, so it is `httpurl`-validated to
	// keep anything but an http:// or https:// scheme out of the stored value
	// (T41); the frontend re-checks at render time (utils/linkResolution.ts)
	// because a stored value predating validation is still possible. The column
	// tag is pinned explicitly per CLAUDE.md trap 1: GORM's initialism handling
	// happens to derive `url` here, but an acronym field is exactly the silent
	// name-mismatch class that broke ContactSyncLink.ETag (`e_tag` vs `etag`),
	// so the migration's spelling is stated rather than inferred.
	URL string `gorm:"column:url" json:"url,omitempty" validate:"omitempty,httpurl,max=2000"`

	// Notes is free-text context beyond what the gift is: sizing, where you
	// saw it, "check they still want this before buying" (T35). Deliberately
	// separate from Description, which stays the one-line "what it is".
	Notes string `json:"notes,omitempty" validate:"omitempty,max=2000"`

	// Date is when the gift was given or received. An idea may have no date; a
	// given gift normally records its handover date.
	Date *time.Time `gorm:"index" json:"date,omitempty"`

	// ValueCents/Currency are the optional value/price. Currency is explicit
	// (ISO 4217) rather than assumed — the pair is enforced to be set together
	// in the controller (T20b's trap: "be explicit about currency rather than
	// assuming one"), so a stored amount is never ambiguous about what it is
	// denominated in. ValueCents is the integer minor-unit amount.
	ValueCents int64  `json:"value_cents,omitempty"`
	Currency   string `json:"currency,omitempty" validate:"omitempty,len=3"`

	// LifeEventID/ActivityID are optional soft references (no FKs) to the
	// LifeEvent (UUID) or Activity (uint PK) this gift relates to — e.g. the
	// birthday life event the gift is for, or the interaction where it was
	// given. Both are verified to belong to the user in the controller. A gift
	// record is the durable object; logging a `gift` interaction
	// (Activity.Type) is a separate, optional timeline event, linked here when
	// one exists (T20b's trap decision).
	LifeEventID string `gorm:"column:life_event_id;index" json:"life_event_id,omitempty" validate:"omitempty,uuid4"`
	ActivityID  *uint  `gorm:"index" json:"activity_id,omitempty"`

	// Deleted is the T17 change-feed tombstone marker, set by the list
	// handler when it reads a row with Unscoped() that has a non-null
	// deleted_at. gorm:"-" keeps it out of the schema; it exists purely so an
	// incremental sync client can apply the deletion.
	Deleted bool `gorm:"-" json:"deleted,omitempty"`
}

// TableName pins the singular table name. GORM's pluralizer derives `gifts`
// from Gift, which happens to match the hand-written migration — but pinning
// it explicitly guards the same silent name-mismatch class that broke
// ConversationAgenda (GORM derived `conversation_agendas` against the
// migration's `conversation_agenda`).
func (Gift) TableName() string {
	return "gifts"
}

// AfterDelete advances updated_at on a soft delete so T17 change feeds see
// the tombstone (see Note.AfterDelete's doc comment for the full rationale).
// Hard deletes and bulk deletes are skipped via the DeletedAt guard.
func (g *Gift) AfterDelete(tx *gorm.DB) error {
	if !g.DeletedAt.Valid {
		return nil
	}
	auditAfterDelete(tx, AuditEntityGift, g.ID, g.UserID, g)
	return tx.Model(&Gift{}).Unscoped().Where("id = ?", g.ID).UpdateColumn("updated_at", time.Now()).Error
}

// BeforeCreate generates a UUID for new gift records, mirroring LifeEvent's
// own BeforeCreate.
func (g *Gift) BeforeCreate(tx *gorm.DB) error {
	if g.ID == "" {
		g.ID = uuid.New().String()
	}
	return nil
}
