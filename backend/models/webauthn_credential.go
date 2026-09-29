package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// WebAuthnCredential is one enrolled passkey / security key for a user
// (issue #593, migration 000069). A passkey is an alternative second factor to
// TOTP.
//
// Hard delete per /CLAUDE.md trap #7: the row's identity IS the
// (user_id, credential_id) natural key, so it mirrors RecoveryCode and
// ContactSyncLink. Deliberately no gorm.Model / DeletedAt — the migration has
// no deleted_at column. Revocation history comes from audit_events.
//
// UUID PK generated in BeforeCreate, like RelationshipEdge/Household.
type WebAuthnCredential struct {
	ID              string     `gorm:"primaryKey" json:"id"`
	UserID          uint       `gorm:"not null;index" json:"-"`
	CredentialID    []byte     `gorm:"column:credential_id;not null" json:"-"`
	PublicKey       []byte     `gorm:"column:public_key;not null" json:"-"`
	AttestationType string     `gorm:"column:attestation_type" json:"-"`
	AAGUID          []byte     `gorm:"column:aaguid" json:"-"`
	SignCount       uint32     `gorm:"column:sign_count" json:"-"`
	Transports      string     `gorm:"column:transports" json:"-"`
	BackupEligible  bool       `gorm:"column:backup_eligible" json:"-"`
	BackupState     bool       `gorm:"column:backup_state" json:"-"`
	Name            string     `json:"name"`
	CreatedAt       time.Time  `json:"created_at"`
	LastUsedAt      *time.Time `gorm:"column:last_used_at" json:"last_used_at"`
}

// BeforeCreate generates the UUID primary key.
func (w *WebAuthnCredential) BeforeCreate(tx *gorm.DB) error {
	if w.ID == "" {
		w.ID = uuid.New().String()
	}
	return nil
}

// TableName pins the migrated table name; GORM would derive
// `web_authn_credentials` from the acronym-cased struct name (trap #1).
func (WebAuthnCredential) TableName() string { return "webauthn_credentials" }
