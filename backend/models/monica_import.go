package models

// Monica-specific import assistant DTOs (issue #549) — the connect step
// (instance URL + API token) and the fetch options. Everything from the
// review step on is shared: see source_import.go.
//
// Compat aliases below keep the pre-rename Monica* names working (openapi
// schema names, the frontend mirror comments) while the canonical types moved
// to source_import.go.

// MonicaEntityCounts is the per-entity total the connect step reports so the
// user sees the account size before committing to a fetch. Mirrors
// monica.EntityCounts (kept as its own type so models does not import the
// monica package).
type MonicaEntityCounts struct {
	Contacts   int `json:"contacts"`
	Activities int `json:"activities"`
	Notes      int `json:"notes"`
	Reminders  int `json:"reminders"`
	Calls      int `json:"calls"`
	Tasks      int `json:"tasks"`
	Gifts      int `json:"gifts"`
	Debts      int `json:"debts"`
}

// MonicaConnectRequest carries the instance URL and API token entered in the
// UI. The token is third-party credential material: never logged, never
// persisted, dropped when the session ends.
type MonicaConnectRequest struct {
	BaseURL  string `json:"base_url" validate:"required,max=500"`
	APIToken string `json:"api_token" validate:"required,max=4000"`
}

// MonicaConnectResponse is returned once the URL + token validate: a new
// session ID, the account's entity counts, and a rough fetch-duration
// estimate (the client-side rate limit is ~1 request/second).
type MonicaConnectResponse struct {
	SessionID             string             `json:"session_id"`
	Totals                MonicaEntityCounts `json:"totals"`
	EstimatedFetchSeconds int                `json:"estimated_fetch_seconds"`
}

// MonicaFetchRequest starts the background snapshot fetch for a session.
//
// The reciprocal half of each bidirectional Monica relationship is always
// collapsed by the mapper (the local graph derives the inverse from one
// stored edge, per ADR-0007) — there is no toggle for it, unlike upstream.
type MonicaFetchRequest struct {
	SessionID string `json:"session_id" validate:"required"`
	// IncludeRelationships also pulls each contact's relationships (one
	// request per contact — the slowest part of a large account).
	IncludeRelationships bool `json:"include_relationships"`
	// IncludeExtras also pulls calls, tasks, gifts and debts (calls become
	// activities, the rest become dated notes).
	IncludeExtras bool `json:"include_extras"`
}

// --- Compat aliases for the shared types (canonical: source_import.go) ---

type MonicaImportStatus = SourceImportStatus
type MonicaRowRelatedCounts = SourceRelatedCounts
type MonicaImportRowPreview = SourceImportRowPreview
type MonicaImportIssue = SourceImportIssue
type MonicaPreviewResponse = SourceImportPreviewResponse
type MonicaConfirmRequest = SourceImportConfirmRequest
type MonicaImportResult = SourceImportResult

const (
	MonicaPhaseConnecting            = SourceImportPhaseConnecting
	MonicaPhaseFetchingContacts      = SourceImportPhaseFetchingContacts
	MonicaPhaseFetchingActivities    = SourceImportPhaseFetchingActivities
	MonicaPhaseFetchingNotes         = SourceImportPhaseFetchingNotes
	MonicaPhaseFetchingReminders     = SourceImportPhaseFetchingReminders
	MonicaPhaseFetchingExtras        = SourceImportPhaseFetchingExtras
	MonicaPhaseFetchingRelationships = SourceImportPhaseFetchingRelationships
	MonicaPhaseBuildingPreview       = SourceImportPhaseBuildingPreview
	MonicaPhaseReady                 = SourceImportPhaseReady
	MonicaPhaseImporting             = SourceImportPhaseImporting
	MonicaPhaseImportingPhotos       = SourceImportPhaseImportingPhotos
	MonicaPhaseDone                  = SourceImportPhaseDone
	MonicaPhaseFailed                = SourceImportPhaseFailed
	MonicaPhaseCancelled             = SourceImportPhaseCancelled
)
