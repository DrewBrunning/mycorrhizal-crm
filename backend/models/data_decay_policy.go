package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// DataDecayPolicy is an opt-in, per-contact rule to periodically re-verify
// stored info (address, phone, employer, relationship facts) is still
// accurate — issue #352, docs/adrs/0027-data-decay.md.
//
// Deliberately distinct from CadencePolicy (T19): cadence is about staying
// in touch (relationship maintenance), data decay is about the freshness of
// the facts themselves. A contact can have either, both, or neither.
//
// Health (next_due / overdue_by) is DERIVED, never stored:
// services/data_decay_service.go computes it from LastVerifiedAt (or
// CreatedAt, when never verified) + IntervalDays. There is deliberately no
// next_due column, matching CadencePolicy's precedent.
//
// LastVerifiedAt itself IS a stored column, unlike CadencePolicy's
// last-qualifying-interaction, which is derived from the Activity timeline.
// There is no existing "I verified this contact's info" event type in the
// timeline, and conflating a self-referential record-keeping check with the
// interaction timeline would be a category error — so this is the one
// deliberate deviation from "health is derived, never stored" (see ADR
// 0026).
//
// Soft-deletes (deleted_at), per T26: user-authored content, same shape as
// CadencePolicy/LifeEvent/Note. The natural key (user_id, entity_id) is
// enforced by a PARTIAL unique index (migration 000065) so a soft-deleted
// policy never blocks re-creating one for the same contact.
type DataDecayPolicy struct {
	ID        string         `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	UserID uint `gorm:"not null;index" json:"-"`

	// EntityID is the subject Contact, referenced by Contact.VCardUID — the
	// same graph invariant CadencePolicy.EntityID/LifeEvent.EntityID follow.
	EntityID string `gorm:"column:entity_id;not null;index" json:"entity_id" validate:"required,uuid4"`

	// IntervalDays is how often the contact's info should be re-verified,
	// e.g. 365. Positive; the health derivation adds it to LastVerifiedAt (or
	// CreatedAt, when never verified).
	IntervalDays int `gorm:"not null" json:"interval_days" validate:"required,gt=0,lte=3650"`

	// LastVerifiedAt is nil until the "confirm still current" action
	// (DataDecayController's Verify endpoint) is used for the first time —
	// see the type doc comment for why this is stored rather than derived.
	LastVerifiedAt *time.Time `gorm:"default:null" json:"last_verified_at,omitempty"`

	// Active lets a policy be paused without losing its history (e.g. "stop
	// nudging me about this contact" without deleting when it was last
	// verified). Deliberately no `gorm:"default:true"` tag: GORM skips a
	// zero-value Go field on Create whenever its tag carries a `default:`,
	// so an explicit Active: false in applyDataDecayInput would silently be
	// overridden back to the SQL column default (OccasionObligation.Active
	// has this exact latent bug — flagged separately, not fixed here). The
	// application layer (applyDataDecayInput) always sets a real value
	// before Create, so no GORM-level default is needed; migration 000065's
	// `DEFAULT 1` column default remains as schema-level documentation and a
	// safety net for any future raw-SQL insert.
	Active bool `gorm:"not null" json:"active"`

	// Deleted is the T17 change-feed tombstone marker, set by the list
	// handler when it reads a row with Unscoped() that has a non-null
	// deleted_at. gorm:"-" keeps it out of the schema.
	Deleted bool `gorm:"-" json:"deleted,omitempty"`
}

// BeforeCreate generates a UUID for new policies, mirroring
// CadencePolicy.BeforeCreate.
func (p *DataDecayPolicy) BeforeCreate(tx *gorm.DB) error {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	return nil
}

// AfterDelete advances updated_at on a soft delete so T17 change feeds see
// the tombstone, mirroring CadencePolicy.AfterDelete. Hard deletes and bulk
// deletes are skipped via the DeletedAt guard.
func (p *DataDecayPolicy) AfterDelete(tx *gorm.DB) error {
	if !p.DeletedAt.Valid {
		return nil
	}
	return tx.Model(&DataDecayPolicy{}).Unscoped().Where("id = ?", p.ID).UpdateColumn("updated_at", time.Now()).Error
}

// DataDecayHealth is the DERIVED read-model as carried on the dashboard
// composite — a wire mirror of services.DataDecayHealth (which this package
// cannot import — services imports models, so models must not import
// services), the same reasoning BriefingCadenceHealth documents.
type DataDecayHealth struct {
	NextDue   time.Time `json:"next_due"`
	OverdueBy int       `json:"overdue_by"`
}
