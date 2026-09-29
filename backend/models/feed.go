package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Feed credential constants (docs/adrs/0030-feeds-atom-emission.md decision 6).
//
// Kind selects the source: a single contact's merged timeline, or the
// aggregate over every non-archived contact the user owns. Detail selects how
// much user-authored text the feed carries: headlines (the localized type
// label plus contact names) or full (plus the entity's own text).
const (
	FeedKindContact   = "contact"
	FeedKindAggregate = "aggregate"

	FeedDetailHeadlines = "headlines"
	FeedDetailFull      = "full"

	// MaxActiveFeedsPerUser caps how many unrevoked feeds one account may hold.
	// Creation past the cap is refused (422); rotation of an active feed mints
	// a replacement, so it never grows the count.
	MaxActiveFeedsPerUser = 50
)

// Feed is a private, revocable Atom feed credential (issue #382, ADR 0030).
// It is a UUID-string-primary-key entity whose ID is generated in BeforeCreate,
// following Circle/LifeEvent, and it has no soft delete: revocation is
// `revoked_at`, exactly like ApiToken (ADR 0004 — a credential is neither
// authored content nor a join row, and keeping revoked rows preserves the
// audit trail).
//
// TokenHash is the SHA-256 hex of the plaintext token, which is returned only
// in the create/rotate response and never stored.
type Feed struct {
	ID        string    `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"-"`

	UserID uint `gorm:"column:user_id;not null;index" json:"-"`

	Name string `gorm:"not null" json:"name"`
	Kind string `gorm:"not null" json:"kind"`
	// EntityID is the contact VCardUID for kind=contact and "" for
	// kind=aggregate.
	EntityID string `gorm:"column:entity_id;not null;default:''" json:"entity_id"`
	// Detail is headlines or full. The column carries a storage default, but
	// the field deliberately has NO `gorm:"default:..."` tag: a non-zero tag
	// default makes GORM drop an explicit value and resurrect the default (the
	// PR #1240 trap), and the parity test enforces it. The controller sets it.
	Detail    string `gorm:"column:detail;not null" json:"detail"`
	TokenHash string `gorm:"column:token_hash;not null;unique" json:"-"`

	LastAccessedAt *time.Time `gorm:"column:last_accessed_at" json:"last_accessed_at"`
	RevokedAt      *time.Time `gorm:"column:revoked_at" json:"-"`
}

// TableName pins the table name to the migration's, independent of GORM's
// pluralization.
func (Feed) TableName() string { return "feeds" }

// BeforeCreate generates the UUID primary key for new feeds, mirroring
// Circle's own BeforeCreate.
func (f *Feed) BeforeCreate(tx *gorm.DB) error {
	if f.ID == "" {
		f.ID = uuid.New().String()
	}
	return nil
}
