package models

// Meerkat-specific import assistant DTOs (issue #550). The wizard is: upload
// the Meerkat SQLite file → pick which source user to import → fetch (map +
// build the preview in the background) → review (with the loss report) →
// confirm. Everything from the review step on is shared: see source_import.go.
//
// The uploaded file is held only as a temp file on disk for the session's
// lifetime and deleted on cancel/expiry/restart — see
// services.MeerkatImportManager and docs/security/data-retention-lifecycle.md.

// MeerkatSourceUser is one row of the uploaded database's users table, shown
// in the source-user picker. A Meerkat deployment can hold several users and
// the importer never silently mixes accounts (ADR-0007).
type MeerkatSourceUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Contacts int    `json:"contacts"`
}

// MeerkatEntityCounts is the whole-file tally shown after upload (across all
// source users) so the user sees the database's size before importing.
type MeerkatEntityCounts struct {
	Contacts      int `json:"contacts"`
	Relationships int `json:"relationships"`
	Notes         int `json:"notes"`
	Activities    int `json:"activities"`
	Reminders     int `json:"reminders"`
}

// MeerkatUploadResponse is returned once the uploaded file parses as a
// Meerkat database: a new session ID, the per-user list for the picker, the
// default (first) source user, and the whole-file totals.
type MeerkatUploadResponse struct {
	SessionID           string              `json:"session_id"`
	SourceUsers         []MeerkatSourceUser `json:"source_users"`
	DefaultSourceUserID *int64              `json:"default_source_user_id,omitempty"`
	Totals              MeerkatEntityCounts `json:"totals"`
}

// MeerkatFetchRequest starts the background map + preview build for a session,
// scoped to one source user (defaults to the first when nil).
type MeerkatFetchRequest struct {
	SessionID    string `json:"session_id" validate:"required"`
	SourceUserID *int64 `json:"source_user_id"`
}
