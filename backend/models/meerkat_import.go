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
	// AcceptUntested is the user's "Try anyway" for an API session whose
	// Meerkat version is not one the importer was tested against (issue #2013).
	// Ignored for uploaded databases.
	AcceptUntested bool `json:"accept_untested"`
}

// MeerkatConnectRequest carries the credentials entered for the live-API
// import (issue #2013): a Meerkat API token (preferred), or a username/email
// and password. They are third-party credential material: held in memory by
// the session only, never logged, never persisted, never echoed back, and
// dropped (the Meerkat session is logged out) when the session ends.
type MeerkatConnectRequest struct {
	BaseURL  string `json:"base_url" validate:"required,max=500"`
	APIToken string `json:"api_token" validate:"max=4000"`
	Username string `json:"username" validate:"max=255"`
	Password string `json:"password" validate:"max=1000"`
}

// MeerkatVersionInfo is the result of API feature detection. Meerkat's
// /health reports a hard-coded version, so the generation is inferred from
// the shape of the API. When Supported is false the user decides whether to
// try anyway (the flow is preview-only until they confirm).
type MeerkatVersionInfo struct {
	Generation string `json:"generation"`
	Supported  bool   `json:"supported"`
	Reason     string `json:"reason"`
	// ReasonCode is the translatable form of Reason ("meerkat_version_older_than_supported" |
	// "meerkat_version_newer_than_tested"; empty when Supported). Clients fall back to Reason
	// for a code they do not know.
	ReasonCode string `json:"reason_code"`
}

// MeerkatConnectResponse is returned once the credentials validate: a new
// session ID, the detected version, the authenticated source user and the
// contact count (the API exposes cheap totals for contacts only; the other
// counts are known after the fetch).
type MeerkatConnectResponse struct {
	SessionID  string              `json:"session_id"`
	Version    MeerkatVersionInfo  `json:"version"`
	SourceUser MeerkatSourceUser   `json:"source_user"`
	Totals     MeerkatEntityCounts `json:"totals"`
	// InsecureTransport is true when the URL is plain http:// to a non-local
	// host: the credential travels in clear text. The UI warns.
	InsecureTransport     bool `json:"insecure_transport"`
	EstimatedFetchSeconds int  `json:"estimated_fetch_seconds"`
}
