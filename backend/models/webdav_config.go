package models

import (
	"gorm.io/gorm"
)

// WebDAVConfig holds a user's Nextcloud / ownCloud (WebDAV) connection
// settings — the exception where a small typed config table is acceptable,
// because the server URL + username + app password are genuinely per-user-
// global (one instance per user), not per-contact (which is where
// ExternalIdentity.metadata lives).
//
// Mirrors ImmichConfig's exact template (immich_config.go): gorm.Model (soft
// delete — a user may remove their connection without losing its history),
// the app password stored encrypted via services/credential_crypto.go. The
// plaintext password is never returned by the API — the config read endpoint
// returns only whether one is set.
//
// Only an app password is accepted, never the user's real account password —
// app passwords are Nextcloud/ownCloud's revocable, scoped credential shape
// and are the only form this integration accepts (a hard requirement, not a
// preference: the CRM never enters a password on the user's behalf).
//
// UserID carries only `index` here, NOT a GORM uniqueIndex tag: the model
// soft-deletes, and a plain GORM unique index would be a non-partial one that
// a soft-deleted row keeps occupying (the T26 trap), blocking re-creating the
// connection. The real uniqueness is the migration's PARTIAL index
// (000027_webdav_configs.up.sql: `WHERE deleted_at IS NULL`).
type WebDAVConfig struct {
	gorm.Model
	UserID uint `gorm:"not null;index" json:"-"`

	// BaseURL is the Nextcloud/ownCloud server root (typically a
	// private/self-hosted address — see config.WebDAVBlockPrivateURLs for the
	// SSRF policy). A web URL, so it is `httpurl`-validated (T41);
	// NormalizeWebDAVBaseURL in services/webdav_service.go independently
	// requires http/https and is what rejects a scheme-less value (there is no
	// way to guess http vs https).
	BaseURL string `gorm:"column:base_url;not null" json:"base_url" validate:"required,min=1,max=2000,httpurl"`

	// Username is the Nextcloud/ownCloud account whose app password this is —
	// required for WebDAV HTTP Basic auth (there is no token-only login).
	Username string `gorm:"not null;default:''" json:"username" validate:"required,min=1,max=200"`

	// AppPasswordEncrypted is the AES-256-GCM-encrypted app password.
	AppPasswordEncrypted string `gorm:"column:app_password_encrypted;not null;default:''" json:"-"`
}

// WebDAVConfigInput is the DTO for creating/updating a WebDAVConfig.
// AppPassword is write-only: empty means "keep the stored password unchanged"
// on update; on create it must be non-empty.
type WebDAVConfigInput struct {
	BaseURL     string `json:"base_url" validate:"required,min=1,max=2000,httpurl"`
	Username    string `json:"username" validate:"required,min=1,max=200"`
	AppPassword string `json:"app_password" validate:"max=512"`
}

// HasAppPassword reports whether a usable (non-empty) encrypted password is
// stored.
func (c *WebDAVConfig) HasAppPassword() bool {
	return c.AppPasswordEncrypted != ""
}

// TableName pins the table name: GORM's naming strategy would derive
// "web_dav_configs" from the acronym in "WebDAVConfig", which disagrees with
// the hand-written migration SQL (CLAUDE.md trap 1 — the same class of bug as
// ContactSyncLink.ETag). The migration 000027 creates webdav_configs.
func (WebDAVConfig) TableName() string {
	return "webdav_configs"
}
