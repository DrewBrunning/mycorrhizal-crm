package models

import (
	"time"

	"gorm.io/gorm"
)

type Webhook struct {
	gorm.Model
	UserID uint     `gorm:"not null;index"`
	Name   string   `gorm:"not null"`
	URL    string   `gorm:"not null"`
	Events []string `gorm:"type:text;serializer:json"`
	Secret string   `gorm:"not null"`
	// No `gorm:"default:true"` tag here on purpose: the SQL default lives in
	// the hand-written migration (`is_active INTEGER NOT NULL DEFAULT 1`) per
	// this project's convention of not using GORM AutoMigrate for schema. A
	// literal `default:` tag makes GORM's Create() unconditionally overwrite
	// a Go zero-value (false) with the parsed default on every insert,
	// regardless of Select()/Omit() — it previously made it impossible to
	// create a webhook that starts inactive.
	IsActive bool
}

type WebhookDelivery struct {
	gorm.Model
	WebhookID   uint   `gorm:"not null;index"`
	EventType   string `gorm:"not null"`
	Payload     string `gorm:"not null"`
	StatusCode  *int
	Error       *string
	Attempts    int `gorm:"default:1"`
	NextRetryAt *time.Time
}
