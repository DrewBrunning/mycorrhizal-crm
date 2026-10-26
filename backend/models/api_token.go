package models

import (
	"time"

	"gorm.io/gorm"
)

type ApiToken struct {
	gorm.Model
	UserID     uint   `gorm:"not null"`
	Name       string `gorm:"not null"`
	TokenHash  string `gorm:"not null;unique" json:"-"`
	LastUsedAt *time.Time
	RevokedAt  *time.Time

	// ExpiresAt bounds how long a leaked token stays useful. NULL means no
	// expiry and exists only for rows predating this column; tokens created
	// through the API always get a concrete value.
	ExpiresAt *time.Time

	// Scope is the legacy single-scope column: "full" (default, near-full
	// REST access) or "carddav" (CardDAV/CalDAV only). New tokens also store
	// the explicit set in Scopes; EffectiveScopes falls back to this column
	// for rows that predate the scope set.
	Scope string `gorm:"not null;default:'full'"`

	// Scopes is the explicit access set (issue #2010). Empty/NULL on rows
	// created before the scope set existed; those fall back to Scope.
	Scopes []string `gorm:"type:text;serializer:json" json:"-"`
}
