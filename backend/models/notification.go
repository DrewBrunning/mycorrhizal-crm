package models

import (
	"time"

	"gorm.io/gorm"
)

// NotificationChannel is the wire name of one reminder delivery channel.
// The allowed set must stay in step with the frontend's channel list
// (frontend/src/api/notifications.ts and NotificationSettings.tsx) — there is
// no dynamic type-list endpoint in this codebase, by design.
type NotificationChannel string

const (
	ChannelEmail  NotificationChannel = "email"
	ChannelNtfy   NotificationChannel = "ntfy"
	ChannelGotify NotificationChannel = "gotify"
	ChannelPush   NotificationChannel = "push"
)

// AllNotificationChannels is the complete registry of channels, in dispatch
// order (email first, then the push-style channels). Keep in sync with the
// CHECK constraint in migration 000013 and the frontend copies.
var AllNotificationChannels = []NotificationChannel{
	ChannelEmail,
	ChannelNtfy,
	ChannelGotify,
	ChannelPush,
}

// NotificationDelivery records one reminder's delivery state for one channel,
// mirroring WebhookDelivery. A reminder is "due" for a channel when no row
// exists with that reminder_id + channel + status='sent'; a 'failed' row
// leaves it due so the next run retries.
//
// Hard-delete (an accessory of its reminder — not independently meaningful,
// per the T26 join/edge rule). The reminder FK cascades on hard delete; soft
// deletes are cleaned manually by the controllers' cascade (see
// deleteReminderDeliveries).
type NotificationDelivery struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	ReminderID uint       `gorm:"not null;index" json:"reminder_id"`
	Channel    string     `gorm:"not null;size:16;index" json:"channel"`
	SentAt     *time.Time `json:"sent_at"` // NULL means not yet delivered
	Status     string     `gorm:"not null;default:'pending'" json:"status"`
	Error      *string    `json:"error,omitempty"`
}

// NotificationConfig holds a user's connection settings for the push-style
// channels. Same shape/pattern as ImmichConfig (the §93.4 exception where a
// small typed per-user config table is right): soft-deletes, and the real
// uniqueness is the migration's PARTIAL index (000013: WHERE deleted_at IS
// NULL) so a removed config can be re-created. The Gotify token is stored
// encrypted via services/credential_crypto.go and never returned by the API.
type NotificationConfig struct {
	gorm.Model
	UserID uint `gorm:"not null;index" json:"-"`

	NtfyURL   string `gorm:"column:ntfy_url;not null;default:''" json:"ntfy_url"`
	NtfyTopic string `gorm:"column:ntfy_topic;not null;default:''" json:"ntfy_topic"`

	GotifyURL            string `gorm:"column:gotify_url;not null;default:''" json:"gotify_url"`
	GotifyTokenEncrypted string `gorm:"column:gotify_token_encrypted;not null;default:''" json:"-"`
}

// NotificationConfigInput is the DTO for creating/updating a NotificationConfig
// plus the per-user channel toggles (which live on users, not on the config
// row). GotifyToken is write-only: empty means "keep the stored token
// unchanged". URLs are optional (a user may use only one push-style channel)
// but, when present, must be valid http(s) URLs.
type NotificationConfigInput struct {
	NtfyURL      string `json:"ntfy_url" validate:"max=2000,safeurl"`
	NtfyTopic    string `json:"ntfy_topic" validate:"max=200"`
	GotifyURL    string `json:"gotify_url" validate:"max=2000,safeurl"`
	GotifyToken  string `json:"gotify_token" validate:"max=512"`
	NotifyNtfy   *bool  `json:"notify_ntfy,omitempty"`
	NotifyGotify *bool  `json:"notify_gotify,omitempty"`
	NotifyPush   *bool  `json:"notify_push,omitempty"`
}

// HasGotifyToken reports whether a usable (non-empty) encrypted token is stored.
func (c *NotificationConfig) HasGotifyToken() bool {
	return c.GotifyTokenEncrypted != ""
}

// PushSubscription is one device registered for Web Push via the browser Push
// API. Endpoint is the push service URL the browser handed us; p256dh/auth are
// the subscription's encryption keys.
//
// Hard-delete (transient device state, not user-authored content — a stale
// subscription is auto-removed when the push service reports 404/410, and
// tombstones would accumulate for nothing). No DeletedAt field; ID is explicit
// so the JSON response is the clean {id, created_at, ...} shape the API
// clients expect (gorm.Model's embedded fields serialize as capitalized Go
// names).
type PushSubscription struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	UserID      uint      `gorm:"not null;index" json:"-"`
	Endpoint    string    `gorm:"not null" json:"endpoint"`
	P256dh      string    `gorm:"not null" json:"p256dh"`
	Auth        string    `gorm:"not null" json:"auth"`
	DeviceLabel string    `gorm:"not null;default:''" json:"device_label"`
}

// PushSubscriptionInput is the DTO for registering a browser Push subscription.
type PushSubscriptionInput struct {
	Endpoint    string `json:"endpoint" validate:"required,max=2000,safeurl"`
	P256dh      string `json:"p256dh" validate:"required,max=1024"`
	Auth        string `json:"auth" validate:"required,max=1024"`
	DeviceLabel string `json:"device_label" validate:"max=200"`
}

// ServerSetting is one key/value row in the server-global settings store
// (currently: the generated Web Push VAPID keypair).
type ServerSetting struct {
	Key   string `gorm:"primaryKey" json:"key"`
	Value string `gorm:"not null" json:"-"`
}
