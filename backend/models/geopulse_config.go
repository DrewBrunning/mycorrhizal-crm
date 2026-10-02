package models

import (
	"gorm.io/gorm"
)

// GeoPulseConfig holds a user's GeoPulse connection settings (issue #160, ADR
// 0033). Like PaperlessConfig/ImmichConfig, the base URL + API token are
// genuinely per-user-global, so a small typed config table is the right shape.
//
// Mirrors PaperlessConfig's template: gorm.Model (soft delete), the API token
// stored encrypted via services/credential_crypto.go and never returned by the
// API — the config read endpoint reports only whether one is set.
//
// UserID carries only `index` here, NOT a GORM uniqueIndex tag: the model
// soft-deletes, and a plain GORM unique index would be a non-partial one that
// a soft-deleted row keeps occupying (the T26 trap). The real uniqueness is the
// migration's PARTIAL index (000071_geopulse_configs.up.sql:
// `WHERE deleted_at IS NULL`).
//
// GeoPulse's own user id is deliberately not a column: the client discovers it
// from GET /api/users/me when it needs it.
type GeoPulseConfig struct {
	gorm.Model
	UserID uint `gorm:"not null;index" json:"-"`

	// BaseURL is the GeoPulse server root (typically a private/self-hosted
	// address — see config.GeoPulseBlockPrivateURLs for the SSRF policy).
	BaseURL string `gorm:"column:base_url;not null" json:"base_url" validate:"required,min=1,max=2000,httpurl"`

	// APIKeyEncrypted is the AES-256-GCM-encrypted GeoPulse API token (issued in
	// GeoPulse's own UI, Profile → Security).
	APIKeyEncrypted string `gorm:"column:api_key_encrypted;not null;default:''" json:"-"`
}

// GeoPulseConfigInput is the DTO for creating/updating a GeoPulseConfig.
// APIKey is write-only: empty means "keep the stored token unchanged" on update;
// on create it must be non-empty.
type GeoPulseConfigInput struct {
	BaseURL string `json:"base_url" validate:"required,min=1,max=2000,httpurl"`
	APIKey  string `json:"api_key" validate:"max=512"`
}

// HasAPIKey reports whether a usable (non-empty) encrypted token is stored.
func (c *GeoPulseConfig) HasAPIKey() bool {
	return c.APIKeyEncrypted != ""
}

// TableName pins the table to the migration's `geopulse_configs`. Without it
// GORM splits the acronym and derives `geo_pulse_configs` — the same silent
// name drift as ContactSyncLink.ETag (CLAUDE.md backend trap #1) — which only a
// real-migrated-schema test can see.
func (GeoPulseConfig) TableName() string { return "geopulse_configs" }
