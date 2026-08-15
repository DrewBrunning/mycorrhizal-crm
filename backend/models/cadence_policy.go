package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CadencePolicy is a relationship-maintenance rule — "stay in touch every N
// days" (docs/adrs/0001-neutral-hub-and-spoke-contact-model.md, T19).
//
// Relationship-driven, deliberately distinct from date-driven Reminders and
// action-driven Tasks: the cadence resets on a *qualifying interaction*
// (Activity.Qualifying()), never on completing a generated task — the CRM
// owns the cadence state, and an external task manager (Vikunja) is at most
// a passive recipient of "overdue" notifications.
//
// Health (next_due / overdue_by) is DERIVED from the timeline, never stored:
// services/cadence_service.go finds the most recent qualifying interaction
// for EntityID and adds TargetIntervalDays. There is deliberately no
// next_due column anywhere.
//
// QualifyingTypes is an open, non-exhaustive filter over Activity.Type (an
// open classifier itself). The authoritative definition of "qualifying" is
// Activity.Qualifying(); an empty list means every default-qualifying type
// counts (no filtering). See CadencePolicy.Qualifies for the composition.
//
// Soft-deletes (deleted_at), per T26: cadence policies are user-authored
// content, same shape as LifeEvent/Note. The natural key (user_id,
// entity_id) is enforced by a PARTIAL unique index (migration 000002) so a
// soft-deleted policy never blocks re-creating a cadence for the same
// contact.
type CadencePolicy struct {
	ID        string         `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	UserID uint `gorm:"not null;index" json:"-"`

	// EntityID is the subject Contact, referenced by Contact.VCardUID — the
	// same graph invariant LifeEvent.EntityID, RelationshipEdge.SourceID/
	// TargetID, and Preference.EntityID follow (§90 D3).
	EntityID string `gorm:"column:entity_id;not null;index" json:"entity_id" validate:"required,uuid4"`

	// TargetIntervalDays is how often the relationship should be maintained,
	// e.g. 30. Positive; the health derivation adds it to the most recent
	// qualifying interaction's date.
	TargetIntervalDays int `gorm:"not null" json:"target_interval_days" validate:"required,gt=0,lte=3650"`

	// QualifyingTypes lists which Interaction types reset the cadence
	// (call/visit/meal…); everything else is ignored. JSON-array column using
	// the same gorm serializer mechanism as RelationshipEdge.Metadata and
	// LifeEvent.RelatedEntityIDs. Empty means "all default-qualifying types"
	// — see Qualifies.
	QualifyingTypes []string `gorm:"type:text;serializer:json" json:"qualifying_types,omitempty"`

	// Deleted is the T17 change-feed tombstone marker, set by the list
	// handler when it reads a row with Unscoped() that has a non-null
	// deleted_at. gorm:"-" keeps it out of the schema; it exists purely so an
	// incremental sync client can apply the deletion.
	Deleted bool `gorm:"-" json:"deleted,omitempty"`
}

// BeforeCreate generates a UUID for new policies, mirroring LifeEvent's own
// BeforeCreate.
func (p *CadencePolicy) BeforeCreate(tx *gorm.DB) error {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	return nil
}

// AfterDelete advances updated_at on a soft delete so T17 change feeds see
// the tombstone (see Note.AfterDelete's doc comment for the full rationale).
// Hard deletes and bulk deletes are skipped via the DeletedAt guard.
func (p *CadencePolicy) AfterDelete(tx *gorm.DB) error {
	if !p.DeletedAt.Valid {
		return nil
	}
	return tx.Model(&CadencePolicy{}).Unscoped().Where("id = ?", p.ID).UpdateColumn("updated_at", time.Now()).Error
}

// Qualifies reports whether the given Activity counts as a cadence-resetting
// interaction for this policy. Two gates, both required:
//
//  1. Activity.Qualifying() — the single repo-wide definition of a
//     qualifying interaction. A type that is globally non-qualifying
//     (e.g. photo) can never be re-admitted by listing it here.
//  2. The policy's QualifyingTypes filter — when non-empty, the activity's
//     Type must be listed; when empty, every default-qualifying type passes.
//
// This is the ONLY consumer of Activity.Qualifying() (T19's explicit
// instruction: "use it, don't reimplement").
func (p *CadencePolicy) Qualifies(a *Activity) bool {
	if !a.Qualifying() {
		return false
	}
	if len(p.QualifyingTypes) == 0 {
		return true
	}
	for _, t := range p.QualifyingTypes {
		if t == a.Type {
			return true
		}
	}
	return false
}
