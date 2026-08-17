package models

import "time"

// RecoveryCode is one hashed, single-use 2FA fallback code for a user (N8,
// migration 000024). The plaintext code is shown exactly once at enrollment /
// regeneration and never stored; only a SHA-256 hash rides here, and consuming
// a code deletes its row — which is what enforces single-use.
//
// Hard delete per /CLAUDE.md trap #7 (T26): the row's identity IS the
// (user, code-hash) natural key — a used code is gone, not merely marked, and
// a lingering dead row could never block anything anyway (the code itself is
// random, not a natural business key that could be re-created). The unique
// index on (user_id, code_hash) also makes duplicates structurally impossible.
//
// Deliberately does NOT embed gorm.Model: that would add a deleted_at column
// and soft-delete semantics, contradicting the hard-delete decision above and
// the migration (which has no deleted_at column).
type RecoveryCode struct {
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	UserID    uint   `gorm:"not null;index" json:"-"`
	CodeHash  string `gorm:"not null" json:"-"`
}
