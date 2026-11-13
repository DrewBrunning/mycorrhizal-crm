package models

import (
	"fmt"
	"time"

	"mycorrhizal/contactmodel"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Link roles, closed per kind (ADR 0043 decision 2). A free-text qualifier
// belongs in RoleLabel, which never participates in matching.
var entityLinkRoles = map[string][]string{
	EntityKindOrganisation: {"employee", "owner", "contractor", "volunteer", "member", "other"},
	EntityKindPlace:        {"resident", "owner", "visitor", "other"},
	EntityKindPet:          {"owner", "carer", "other"},
}

// EntityLinkRoles returns the closed role vocabulary of kind (nil if unknown).
func EntityLinkRoles(kind string) []string {
	return append([]string(nil), entityLinkRoles[kind]...)
}

// ValidEntityLinkRole reports whether role is in kind's vocabulary.
func ValidEntityLinkRole(kind, role string) bool {
	for _, r := range entityLinkRoles[kind] {
		if r == role {
			return true
		}
	}
	return false
}

// ContactEntityLink attaches a contact (Contact.VCardUID) to an Entity.
//
// Soft delete is a deliberate, recorded exception to ADR 0004's join-row rule
// (docs/adrs/0043-reusable-entities.md decision 6): the link has no natural-key
// unique index (re-hire = two links) and its only unique index (primary) is
// partial on deleted_at IS NULL, while it carries authored data (role, dates,
// primary) that trash restore must bring back. DeletionBatchID marks the
// entity-delete batch that tombstoned it.
type ContactEntityLink struct {
	ID        string         `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	// DeletionBatchID is stamped when an entity delete tombstones the link.
	DeletionBatchID *string `gorm:"column:deletion_batch_id;index" json:"-"`

	UserID          uint   `gorm:"not null;index:idx_contact_entity_links_user_contact,priority:1" json:"-"`
	ContactVCardUID string `gorm:"column:contact_vcard_uid;not null;index:idx_contact_entity_links_user_contact,priority:2" json:"contact_vcard_uid"`
	EntityID        string `gorm:"not null;index" json:"entity_id"`
	// Kind is a copy of the entity's kind, immutable; it makes the
	// primary-per-kind index expressible.
	Kind      string `gorm:"not null;<-:create" json:"kind" validate:"required,oneof=organisation place pet"`
	Role      string `gorm:"not null" json:"role" validate:"required"`
	RoleLabel string `gorm:"not null" json:"role_label" validate:"max=100"`
	// IsPrimary: at most one live primary per contact per kind (partial unique
	// index). No gorm default, so an explicit false is written (issue #1240).
	IsPrimary bool `gorm:"not null" json:"is_primary"`
	// Period is the optional start/end; an all-empty range is stored as NULL.
	Period      *contactmodel.TemporalRange `gorm:"type:text;serializer:json" json:"period,omitempty"`
	Sensitivity string                      `gorm:"not null;default:normal;index" json:"sensitivity" validate:"required,oneof=normal private secret"`

	ETag     string `gorm:"column:etag;<-:create" json:"-"`
	Revision int64  `gorm:"column:revision;<-:create;not null;default:1" json:"revision"`

	revisionStampedOnCreate bool
}

// BeforeCreate generates the UUID.
func (l *ContactEntityLink) BeforeCreate(tx *gorm.DB) error {
	if l.ID == "" {
		l.ID = uuid.New().String()
	}
	return nil
}

// BeforeSave enforces the closed per-kind role vocabulary and stores an
// all-empty period as NULL.
func (l *ContactEntityLink) BeforeSave(tx *gorm.DB) error {
	if l.ID == "" && l.Kind == "" && l.Role == "" {
		return nil // bulk Model(&ContactEntityLink{}).Where(...).Update(s)
	}
	if !ValidEntityLinkRole(l.Kind, l.Role) {
		return fmt.Errorf("role %q is not valid for entity kind %q", l.Role, l.Kind)
	}
	if l.Period != nil && l.Period.IsEmpty() {
		l.Period = nil
	}
	return l.captureSensitivity(tx)
}

// AfterCreate stamps revision 1 and the ETag (ADR 0006).
func (l *ContactEntityLink) AfterCreate(tx *gorm.DB) error {
	l.Revision = 1
	l.ETag = fmt.Sprintf("e-%s-%d", l.ID, l.Revision)
	l.revisionStampedOnCreate = true
	return tx.Table("contact_entity_links").Where("id = ?", l.ID).UpdateColumns(map[string]any{"revision": l.Revision, "etag": l.ETag}).Error
}

// AfterSave bumps the revision with the CON-01 compare-and-swap.
func (l *ContactEntityLink) AfterSave(tx *gorm.DB) error {
	if l.ID == "" {
		return nil
	}
	// Issue #2564: withhold the projected section on a sensitivity raise.
	if err := l.markSensitivityReclassification(tx); err != nil {
		return err
	}
	if l.revisionStampedOnCreate {
		l.revisionStampedOnCreate = false
		return nil
	}
	newRevision, err := bumpRevisionCAS(tx, "ContactEntityLink", "contact_entity_links", l.ID, l.Revision, func(nr int64) string {
		return fmt.Sprintf("e-%s-%d", l.ID, nr)
	})
	if err != nil {
		return err
	}
	l.Revision = newRevision
	l.ETag = fmt.Sprintf("e-%s-%d", l.ID, newRevision)
	return nil
}
