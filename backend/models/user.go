package models

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	gorm.Model
	Username                 string     `gorm:"unique" validate:"required,min=1,max=50,no_at_sign"`
	Password                 string     `validate:"required,min=8,strong_password"`
	Email                    string     `gorm:"unique" validate:"required,email"`
	Language                 string     `gorm:"default:'en'" json:"language" validate:"omitempty,oneof=en de it es fr"`
	DateFormat               string     `gorm:"default:'eu'" json:"date_format" validate:"omitempty,oneof=eu us iso"`
	IsAdmin                  bool       `gorm:"default:false" json:"is_admin"`
	PasswordResetTokenHash   *string    `gorm:"column:password_reset_token_hash"`
	PasswordResetExpiresAt   *time.Time `gorm:"column:password_reset_expires_at"`
	PasswordResetRequestedAt *time.Time `gorm:"column:password_reset_requested_at"`
	EnabledContactFields     []string   `gorm:"type:text;serializer:json" json:"enabled_contact_fields"`
	OIDCSubject              *string    `gorm:"column:oidc_subject"`
	OIDCProvider             *string    `gorm:"column:oidc_provider"`

	// TokenVersion is embedded in every JWT issued for this user and checked on
	// each authenticated request. Bumping it invalidates all outstanding tokens,
	// which is how a password change or reset ends existing sessions — JWTs are
	// stateless, so there is nothing else to revoke.
	TokenVersion uint `gorm:"not null;default:0" json:"-"`

	// N9 notification-channel toggles (migration 000013). Email stays gated
	// per-reminder by Reminder.ByMail for backwards compatibility; these toggle
	// the push-style channels globally for the user. A channel dispatches when
	// its toggle is on AND the user has a usable per-channel config.
	NotifyNtfy   bool `gorm:"column:notify_ntfy;not null;default:false" json:"notify_ntfy"`
	NotifyGotify bool `gorm:"column:notify_gotify;not null;default:false" json:"notify_gotify"`
	NotifyPush   bool `gorm:"column:notify_push;not null;default:false" json:"notify_push"`

	// Self-contact VCardUID — every user gets a contact representing themselves
	// on registration (migration 000018). Null for pre-existing users until
	// they create or are assigned one. References contacts.vcard_uid.
	SelfContactVCardUID *string `gorm:"column:self_contact_vcard_uid" json:"self_contact_vcard_uid,omitempty"`
}
