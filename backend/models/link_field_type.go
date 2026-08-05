package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Link field type categories (LinkFieldType.Category).
const (
	LinkFieldTypeCategoryMessaging = "messaging"
	LinkFieldTypeCategorySocial    = "social"
	LinkFieldTypeCategoryOther     = "other"
)

// LinkFieldType is a user-configurable messaging/social link type — name,
// URI-template protocol, category, icon (docs/fork-plan/tickets/
// 43-T34-contact-field-linking.md). Deliberately NOT a hardcoded
// oneof-validated enum like this repo's usual registries (CLAUDE.md
// frontend-trap-4): the whole point is that a wrong or missing template is
// fixable by the user without a code change, closer to Monica's "Contact
// field types" screen than a backend enum mirror.
//
// Protocol is a URI template with a literal "{value}" placeholder (e.g.
// "https://wa.me/{value}") substituted with an OnlineService.User handle at
// render time. It is validated with `safeurl` so a `javascript:`/`data:`/
// etc. scheme can't be stored — but that only validates the TEMPLATE; the
// frontend must independently validate the SUBSTITUTED value before using
// it as an href (the template itself can't smuggle a bad scheme via the
// {value} slot, but this is defense in depth per the ticket's own trap).
//
// UUID-string-primary-key entity, following CadencePolicy's exact template
// (cadence_policy.go): ID generated in BeforeCreate, soft-deletes (T26 —
// user-authored configuration). The natural key (user_id, name) is a
// PARTIAL unique index (migration 000009) so a soft-deleted type never
// blocks re-creating one with the same name.
type LinkFieldType struct {
	ID        string         `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	UserID uint `gorm:"not null;index" json:"-"`

	Name     string `gorm:"not null" json:"name"`
	Protocol string `gorm:"not null;default:''" json:"protocol"`
	Category string `gorm:"not null" json:"category"`
	Icon     string `gorm:"column:icon" json:"icon,omitempty"`

	// IsDefault marks a seeded row (see LinkFieldTypeDefaults) so the UI can
	// tell defaults apart from user additions. Never client-settable —
	// CreateLinkFieldType always forces false, and UpdateLinkFieldType never
	// touches this field, so an edited default stays flagged as a default
	// (provenance, not "unmodified").
	IsDefault bool `gorm:"not null;default:false" json:"is_default"`

	// Position orders the list for display/reorder (Settings drag-reorder
	// UI, implemented as up/down controls — no drag-and-drop library exists
	// in this repo yet, and one type registry doesn't warrant adding one).
	Position int `gorm:"not null;default:0" json:"position"`
}

// BeforeCreate generates a UUID for new LinkFieldTypes, mirroring
// CadencePolicy's own BeforeCreate.
func (l *LinkFieldType) BeforeCreate(tx *gorm.DB) error {
	if l.ID == "" {
		l.ID = uuid.New().String()
	}
	return nil
}

// LinkFieldTypeDefaults are the per-user seed rows lazily created on a
// user's first ListLinkFieldTypes call (link_field_type_controller.go) —
// there is no existing "seed defaults on first fetch" pattern elsewhere in
// this repo to copy, so this is a fresh design for T34. Best-effort
// {value}-templated URLs; a handful (Discord, Wire, Session, GroupMe,
// Slack) have no stable public profile-by-handle URL and seed with an
// empty Protocol — user-editable, not blocking, per the ticket's own call.
// IsDefault is true and UserID/ID/timestamps are filled in at seed time.
var LinkFieldTypeDefaults = []LinkFieldType{
	// Messaging
	{Name: "Signal", Protocol: "https://signal.me/#p/{value}", Category: LinkFieldTypeCategoryMessaging, Icon: "mdiMessageLock", Position: 0},
	{Name: "Messenger", Protocol: "https://m.me/{value}", Category: LinkFieldTypeCategoryMessaging, Icon: "mdiFacebookMessenger", Position: 1},
	{Name: "WhatsApp", Protocol: "https://wa.me/{value}", Category: LinkFieldTypeCategoryMessaging, Icon: "mdiWhatsapp", Position: 2},
	{Name: "Telegram", Protocol: "https://t.me/{value}", Category: LinkFieldTypeCategoryMessaging, Icon: "mdiSend", Position: 3},
	{Name: "Discord", Protocol: "", Category: LinkFieldTypeCategoryMessaging, Icon: "mdiChatOutline", Position: 4},
	{Name: "Wire", Protocol: "", Category: LinkFieldTypeCategoryMessaging, Icon: "mdiChatOutline", Position: 5},
	{Name: "Session", Protocol: "", Category: LinkFieldTypeCategoryMessaging, Icon: "mdiChatOutline", Position: 6},
	{Name: "Line", Protocol: "https://line.me/ti/p/{value}", Category: LinkFieldTypeCategoryMessaging, Icon: "mdiChatOutline", Position: 7},
	{Name: "Element", Protocol: "https://matrix.to/#/{value}", Category: LinkFieldTypeCategoryMessaging, Icon: "mdiMatrix", Position: 8},
	{Name: "GroupMe", Protocol: "", Category: LinkFieldTypeCategoryMessaging, Icon: "mdiChatOutline", Position: 9},
	{Name: "Slack", Protocol: "", Category: LinkFieldTypeCategoryMessaging, Icon: "mdiSlack", Position: 10},
	// Social
	{Name: "X/Twitter", Protocol: "https://x.com/{value}", Category: LinkFieldTypeCategorySocial, Icon: "mdiTwitter", Position: 11},
	{Name: "BlueSky", Protocol: "https://bsky.app/profile/{value}", Category: LinkFieldTypeCategorySocial, Icon: "mdiButterflyOutline", Position: 12},
	{Name: "Instagram", Protocol: "https://instagram.com/{value}", Category: LinkFieldTypeCategorySocial, Icon: "mdiInstagram", Position: 13},
	{Name: "Threads", Protocol: "https://www.threads.net/@{value}", Category: LinkFieldTypeCategorySocial, Icon: "mdiAt", Position: 14},
	{Name: "TikTok", Protocol: "https://www.tiktok.com/@{value}", Category: LinkFieldTypeCategorySocial, Icon: "mdiMusicNote", Position: 15},
	{Name: "Reddit", Protocol: "https://www.reddit.com/user/{value}", Category: LinkFieldTypeCategorySocial, Icon: "mdiReddit", Position: 16},
	{Name: "Spotify", Protocol: "https://open.spotify.com/user/{value}", Category: LinkFieldTypeCategorySocial, Icon: "mdiSpotify", Position: 17},
	{Name: "Twitch", Protocol: "https://www.twitch.tv/{value}", Category: LinkFieldTypeCategorySocial, Icon: "mdiTwitch", Position: 18},
}
