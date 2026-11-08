package models

import "time"

// TrashedContact is one row of GET /api/v1/trash/contacts (issue #2002).
type TrashedContact struct {
	Contact   ContactSummary `json:"contact"`
	DeletedAt time.Time      `json:"deleted_at"`
	// PurgeAt is when the scheduled purge hard-deletes the contact; null when
	// DELETED_RETENTION_DAYS is 0 (keep forever).
	PurgeAt *time.Time `json:"purge_at"`
}

// TrashListResponse is the GET /api/v1/trash/contacts body. Collection fields
// carry no omitempty (frontend trap #8).
type TrashListResponse struct {
	Contacts      []TrashedContact `json:"contacts"`
	Total         int64            `json:"total"`
	Page          int              `json:"page"`
	Limit         int              `json:"limit"`
	RetentionDays int              `json:"retention_days"`
}

// TrashRestoreSkip reports rows a restore could not bring back.
type TrashRestoreSkip struct {
	Table  string `json:"table"`
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

// TrashRestoreResponse is the POST /api/v1/trash/contacts/{id}/restore body.
type TrashRestoreResponse struct {
	Contact ContactSummary     `json:"contact"`
	Skipped []TrashRestoreSkip `json:"skipped"`
	// RevokedGrants lists the standing-share grants the delete revoked
	// (reason entity_deleted) so the UI can offer to issue them again. Restore
	// never re-arms access (ADR 0042 decision 10, issue #2291).
	RevokedGrants []TrashRevokedShareGrant `json:"revoked_grants"`
}

// TrashRevokedShareGrant is one grant revoked by the contact's deletion: the
// recipient and the granted sections, never contact content.
type TrashRevokedShareGrant struct {
	ID         string     `json:"id"`
	ToUserID   uint       `json:"to_user_id"`
	ToUsername string     `json:"to_username"`
	Sections   []string   `json:"sections"`
	ExpiresAt  *time.Time `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
}

// TrashPurgeResponse is the body of the purge and empty-trash DELETEs.
type TrashPurgeResponse struct {
	Message string `json:"message"`
	Purged  int    `json:"purged"`
}
