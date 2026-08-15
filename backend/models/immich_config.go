package models

import (
	"time"

	"gorm.io/gorm"
)

// ImmichConfig holds a user's Immich connection settings — the
// exception where a small typed config table is acceptable, because the
// base URL + API key are genuinely per-user-global (one Immich instance per
// user), not per-contact (which is where ExternalIdentity.metadata lives).
//
// Mirrors CalendarSubscription's exact template (calendar_subscription.go):
// gorm.Model (soft delete — a user may remove their connection without
// losing its history), API key stored encrypted via services/credential_
// crypto.go. The plaintext API key is never returned by the API — the config
// read endpoint returns only whether one is set.
//
// UserID carries only `index` here, NOT a GORM uniqueIndex tag: the model
// soft-deletes, and a plain GORM unique index would be a non-partial one that
// a soft-deleted row keeps occupying (the T26 trap), blocking re-creating the
// connection. The real uniqueness is the migration's PARTIAL index
// (000006_immich_configs.up.sql: `WHERE deleted_at IS NULL`), the same pattern
// idx_contacts_vcard_uid_user uses.
type ImmichConfig struct {
	gorm.Model
	UserID uint `gorm:"not null;index" json:"-"`

	// BaseURL is the Immich server root (typically a private/self-hosted
	// address — see config.ImmichBlockPrivateURLs for the SSRF policy). A web
	// URL, so it is `httpurl`-validated (T41); NormalizeImmichBaseURL in
	// services/immich_service.go independently requires http/https and is what
	// rejects a scheme-less value (there is no way to guess http vs https).
	BaseURL string `gorm:"column:base_url;not null" json:"base_url" validate:"required,min=1,max=2000,httpurl"`

	// APIKeyEncrypted is the AES-256-GCM-encrypted Immich API key.
	APIKeyEncrypted string `gorm:"column:api_key_encrypted;not null;default:''" json:"-"`

	// SyncEnabled gates the scheduled enrichment sync (T16).
	SyncEnabled bool `gorm:"column:sync_enabled;default:true" json:"sync_enabled"`

	LastSyncedAt   *time.Time `gorm:"column:last_synced_at" json:"last_synced_at,omitempty"`
	LastSyncStatus string     `gorm:"column:last_sync_status;not null;default:''" json:"last_sync_status"`
	LastSyncError  string     `gorm:"column:last_sync_error;not null;default:''" json:"last_sync_error"`
}

// ImmichConfigInput is the DTO for creating/updating an ImmichConfig.
// APIKey is write-only: empty means "keep the stored key unchanged" on
// update; on create it must be non-empty.
type ImmichConfigInput struct {
	BaseURL     string `json:"base_url" validate:"required,min=1,max=2000,httpurl"`
	APIKey      string `json:"api_key" validate:"max=512"`
	SyncEnabled *bool  `json:"sync_enabled,omitempty"`
}

// HasAPIKey reports whether a usable (non-empty) encrypted key is stored.
func (c *ImmichConfig) HasAPIKey() bool {
	return c.APIKeyEncrypted != ""
}
