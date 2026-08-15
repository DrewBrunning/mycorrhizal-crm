package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ConversationAgenda is "things to bring up next time I see them" — contextual
// memory surfaced on the contact view (docs/adrs/0001-neutral-hub-and-spoke-contact-model.md
// ). It is deliberately NOT date-scheduled: an agenda item
// has no due date and no completion cron (that is what distinguishes it from a
// Reminder). It is surfaced by context, not by time, and is resolved by
// marking it discussed — optionally linking the interaction (Activity) that
// covered it, which then feeds the timeline.
//
// UUID-string-primary-key entity, following LifeEvent's exact template
// (life_event.go): ID generated in BeforeCreate, soft-deletes. Agenda items
// are user-authored content (a thought about a conversation), not a
// graph-adjacent join row, so they soft-delete per T26 — and because they have
// no natural-key unique constraint (a contact may have many agenda items), a
// soft-deleted row never blocks re-creation, which is the trap that forces
// hard-delete on join rows.
type ConversationAgenda struct {
	ID        string         `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	UserID uint `gorm:"not null;index" json:"-"`

	// EntityID is the subject Contact, referenced by Contact.VCardUID — the
	// same graph invariant every join/reference entity follows. An
	// agenda item is keyed to the contact it will be discussed with, never to
	// a date.
	EntityID string `gorm:"column:entity_id;not null;index" json:"entity_id" validate:"required,uuid4"`

	// Content is the free-text item. ReferenceURL is an optional link/reference
	// to whatever the item is about — a web page, so it is `httpurl`-validated
	// (T41) to reject anything but http/https. No due date anywhere in this
	// model.
	Content      string `gorm:"not null" json:"content" validate:"required,min=1,max=2000"`
	ReferenceURL string `json:"reference_url,omitempty" validate:"omitempty,httpurl,max=2000"`

	// DiscussedAt is the "resolved/discussed flag with the date it was
	// discussed". nil = still open (surfaced in the open list); set =
	// resolved, but the row stays visible so "we talked about this on the 3rd"
	// remains answerable. Deliberately not a completion cron — the item is
	// resolved by context (the next conversation), never by a timer.
	DiscussedAt *time.Time `gorm:"index" json:"discussed_at,omitempty"`

	// ActivityID optionally links the interaction that covered this item
	// (Activity's uint PK). Soft reference, no FK — Activities belong to a
	// separately-owned subsystem and soft-delete independently. When set, the
	// timeline already carries that interaction; this just ties the two
	// surfaces together.
	ActivityID *uint `gorm:"index" json:"activity_id,omitempty"`

	// Deleted is the T17 change-feed tombstone marker, set by the list
	// handler when it reads a row with Unscoped() that has a non-null
	// deleted_at. gorm:"-" keeps it out of the schema; it exists purely so an
	// incremental sync client can apply the deletion.
	Deleted bool `gorm:"-" json:"deleted,omitempty"`
}

// TableName pins the singular table name. GORM's pluralizer derives
// `conversation_agendas` from ConversationAgenda, but the hand-written
// migration 000003 names the table `conversation_agenda` — the same silent
// name-mismatch class as ContactSyncLink.ETag's column. Explicit is mandatory
// here or the real-DB round-trip test fails on every write.
func (ConversationAgenda) TableName() string {
	return "conversation_agenda"
}

// AfterDelete advances updated_at on a soft delete so T17 change feeds see
// the tombstone (see Note.AfterDelete's doc comment for the full rationale).
// Hard deletes and bulk deletes are skipped via the DeletedAt guard. The PK
// is a UUID string, so no numeric conversion is needed.
func (c *ConversationAgenda) AfterDelete(tx *gorm.DB) error {
	if !c.DeletedAt.Valid {
		return nil
	}
	return tx.Model(&ConversationAgenda{}).Unscoped().Where("id = ?", c.ID).UpdateColumn("updated_at", time.Now()).Error
}

// BeforeCreate generates a UUID for new agenda items, mirroring LifeEvent's
// own BeforeCreate.
func (c *ConversationAgenda) BeforeCreate(tx *gorm.DB) error {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	return nil
}
