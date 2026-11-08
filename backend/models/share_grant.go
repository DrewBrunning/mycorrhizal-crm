package models

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ShareGrant entity types (ADR 0042 decision 4). Closed set, mirrored by the
// share_grants.entity_type CHECK.
const (
	ShareGrantEntityContact  = "contact"
	ShareGrantEntityNote     = "note"
	ShareGrantEntityActivity = "activity"
	ShareGrantEntityCircle   = "circle"
)

// ShareGrant statuses (ADR 0042 decision 7). Closed set, mirrored by the
// share_grants.status CHECK. ShareGrantStatusBlocked is internal: the sender
// sees it as pending, the recipient does not see the grant at all (decision 8).
// Declined, revoked and expired are terminal.
const (
	ShareGrantStatusPending  = "pending"
	ShareGrantStatusActive   = "active"
	ShareGrantStatusDeclined = "declined"
	ShareGrantStatusRevoked  = "revoked"
	ShareGrantStatusExpired  = "expired"
	ShareGrantStatusBlocked  = "blocked"
)

// ShareGrant revoked_reason values (ADR 0042 decision 10). Closed set, mirrored
// by the share_grants.revoked_reason CHECK.
const (
	ShareGrantReasonOwnerRevoked     = "owner_revoked"
	ShareGrantReasonEntityDeleted    = "entity_deleted"
	ShareGrantReasonRecipientDeleted = "recipient_deleted"
)

// ShareGrantLiveStatuses are the statuses the partial unique index on
// (from_user_id, to_user_id, entity_type, entity_id) covers: a grant in one of
// them is "live" and at most one may exist per tuple.
var ShareGrantLiveStatuses = []string{ShareGrantStatusPending, ShareGrantStatusActive, ShareGrantStatusBlocked}

// ShareGrant is a standing, read-only, live grant of one entity from
// FromUserID to ToUserID (ADR 0042, issue #1998). It is the authorization
// record only: it stores who may read what, never content, and has no payload
// or snapshot. It is separate from ContactShare, the frozen one-time copy,
// which it never converts or mutates.
//
// UUID-string-primary-key entity: ID generated in BeforeCreate.
//
// No soft delete (ADR 0004, ADR 0042 decision 13): Status is the lifecycle and
// its tombstone. Terminal rows (declined/revoked/expired) are kept for the
// retention window and then hard-deleted; deleteUserCascade removes a user's
// rows on either side unconditionally.
//
// No mass assignment: this model is never bound from a request body. The API
// binds dedicated DTOs (to_user_id, entity_type, entity_id, sections,
// expires_at on create; expires_at and sections on patch), and Status,
// FromUserID, timestamps, Revision, ReconfirmSections and the Revoked* fields
// are set only by the service through explicit column allowlists.
type ShareGrant struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	FromUserID uint `gorm:"not null" json:"from_user_id" validate:"required,nefield=ToUserID"`
	ToUserID   uint `gorm:"not null" json:"to_user_id" validate:"required"`

	EntityType string `gorm:"not null" json:"entity_type" validate:"required,oneof=contact note activity circle"`
	// EntityID is the entity's stable portable identity (Contact.VCardUID or
	// the UUID of a note, activity or circle), never an integer PK (decision 4).
	EntityID string `gorm:"not null" json:"entity_id" validate:"required"`

	// Sections is the granted contact sections; empty for non-contact types.
	// ReconfirmSections holds sections withheld until the owner re-confirms
	// (decision 5). Both are JSON text, never NULL.
	Sections          []string `gorm:"type:text;serializer:json;not null" json:"sections"`
	ReconfirmSections []string `gorm:"type:text;serializer:json;not null" json:"reconfirm_sections"`

	Status string `gorm:"not null" json:"status" validate:"required,oneof=pending active declined revoked expired blocked"`

	RespondedAt   *time.Time `json:"responded_at"`
	ExpiresAt     *time.Time `json:"expires_at"`
	RevokedAt     *time.Time `json:"revoked_at"`
	RevokedReason *string    `json:"revoked_reason" validate:"omitempty,oneof=owner_revoked entity_deleted recipient_deleted"`

	// Revision is the monotonic per-row write counter (ADR 0006). `<-:create`
	// keeps a plain Save from clobbering it; the service advances it with an
	// explicit UpdateColumn. 0 is never persisted: BeforeCreate starts it at 1.
	Revision int64 `gorm:"column:revision;<-:create;not null" json:"revision"`
}

// BeforeCreate generates the UUID, starts the revision at 1 and turns a nil
// section list into "[]" so the NOT NULL JSON columns never hold "null".
func (g *ShareGrant) BeforeCreate(tx *gorm.DB) error {
	if g.ID == "" {
		g.ID = uuid.New().String()
	}
	if g.Revision == 0 {
		g.Revision = 1
	}
	if g.Sections == nil {
		g.Sections = []string{}
	}
	if g.ReconfirmSections == nil {
		g.ReconfirmSections = []string{}
	}
	return nil
}

// ETag is the ADR 0006 revision token: e-{id}-{revision}.
func (g *ShareGrant) ETag() string {
	return fmt.Sprintf("e-%s-%d", g.ID, g.Revision)
}

// ShareBlock records that BlockerUserID blocked BlockedUserID from sending
// standing shares (ADR 0042 decision 8). Composite primary key, no surrogate
// id. An edge row: hard delete (ADR 0004), unblock removes it.
type ShareBlock struct {
	BlockerUserID uint      `gorm:"primaryKey;autoIncrement:false" json:"blocker_user_id"`
	BlockedUserID uint      `gorm:"primaryKey;autoIncrement:false" json:"blocked_user_id"`
	CreatedAt     time.Time `json:"created_at"`
}

// ShareGrantNotification kinds (ADR 0042 decision 11). Closed set, mirrored by
// the share_grant_notifications.kind CHECK.
const (
	ShareGrantNoticePending = "pending"
	ShareGrantNoticeRevoked = "revoked"
)

// ShareGrantNotification is the recipient's in-app notice that a standing
// share arrived (kind pending) or ended (kind revoked). It carries the sender
// id and the entity TYPE only: never an entity id, label or content, so a
// notice can never leak source data (the sender username is joined at read
// time). Hard delete (system-generated, not user content): the lifecycle job
// purges it after SHARE_GRANT_RETENTION_DAYS and deleteUserCascade removes it
// for either party.
type ShareGrantNotification struct {
	ID         uint       `gorm:"primarykey" json:"id"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	UserID     uint       `gorm:"not null" json:"-"`
	FromUserID uint       `gorm:"not null" json:"-"`
	Kind       string     `gorm:"not null" json:"kind"`
	EntityType string     `gorm:"not null" json:"entity_type"`
	ReadAt     *time.Time `json:"read_at"`
}
