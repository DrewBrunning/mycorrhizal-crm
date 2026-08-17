package models

import (
	"gorm.io/gorm"
)

// SeafileConfig holds a user's Seafile connection settings — the exception
// where a small typed config table is acceptable, because the server URL +
// API token are genuinely per-user-global (one Seafile account per user), not
// per-contact (which is where ExternalIdentity.metadata lives).
//
// Mirrors ImmichConfig's exact template (immich_config.go): gorm.Model (soft
// delete — a user may remove their connection without losing its history),
// API token stored encrypted via services/credential_crypto.go. The plaintext
// token is never returned by the API — the config read endpoint returns only
// whether one is set.
//
// UserID carries only `index` here, NOT a GORM uniqueIndex tag: the model
// soft-deletes, and a plain GORM unique index would be a non-partial one that
// a soft-deleted row keeps occupying (the T26 trap), blocking re-creating the
// connection. The real uniqueness is the migration's PARTIAL index
// (000026_seafile_configs.up.sql: `WHERE deleted_at IS NULL`).
type SeafileConfig struct {
	gorm.Model
	UserID uint `gorm:"not null;index" json:"-"`

	// BaseURL is the Seafile server root (typically a private/self-hosted
	// address — see config.SeafileBlockPrivateURLs for the SSRF policy). A web
	// URL, so it is `httpurl`-validated (T41); NormalizeSeafileBaseURL in
	// services/seafile_service.go independently requires http/https and is what
	// rejects a scheme-less value (there is no way to guess http vs https).
	BaseURL string `gorm:"column:base_url;not null" json:"base_url" validate:"required,min=1,max=2000,httpurl"`

	// APITokenEncrypted is the AES-256-GCM-encrypted Seafile API token (issued
	// natively by Seafile under the account's API tokens page).
	APITokenEncrypted string `gorm:"column:api_token_encrypted;not null;default:''" json:"-"`
}

// SeafileConfigInput is the DTO for creating/updating a SeafileConfig.
// APIToken is write-only: empty means "keep the stored token unchanged" on
// update; on create it must be non-empty.
type SeafileConfigInput struct {
	BaseURL  string `json:"base_url" validate:"required,min=1,max=2000,httpurl"`
	APIToken string `json:"api_token" validate:"max=512"`
}

// HasAPIToken reports whether a usable (non-empty) encrypted token is stored.
func (c *SeafileConfig) HasAPIToken() bool {
	return c.APITokenEncrypted != ""
}
