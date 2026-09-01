package models

// Monica import assistant DTOs (issue #549). The wizard is: connect (URL +
// API token) → fetch (a background snapshot pull with progress) → review
// (per-contact add/skip/update over a preview that already shows the loss
// report) → confirm → result. Server-side state, including the API token,
// lives only in services.MonicaImportManager's in-memory sessions.
//
// The phase and result token vocabularies are mirrored by hand in
// frontend/src/api/monicaImport.ts and backend/openapi.yaml — there is no
// dynamic type-list endpoint by design (CLAUDE.md frontend trap #4).

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

// Monica import wizard phases, in order. "failed" is terminal for the fetch;
// the wizard can restart the fetch from "connecting"/"failed".
const (
	MonicaPhaseConnecting            = "connecting"
	MonicaPhaseFetchingContacts      = "fetching_contacts"
	MonicaPhaseFetchingActivities    = "fetching_activities"
	MonicaPhaseFetchingNotes         = "fetching_notes"
	MonicaPhaseFetchingReminders     = "fetching_reminders"
	MonicaPhaseFetchingExtras        = "fetching_extras"
	MonicaPhaseFetchingRelationships = "fetching_relationships"
	MonicaPhaseBuildingPreview       = "building_preview"
	MonicaPhaseReady                 = "ready"
	MonicaPhaseImporting             = "importing"
	MonicaPhaseImportingPhotos       = "importing_photos"
	MonicaPhaseDone                  = "done"
	MonicaPhaseFailed                = "failed"
	MonicaPhaseCancelled             = "cancelled" // an in-flight import was cancelled; the transaction rolled back
)

// MonicaImportStatus is the poll payload for the fetch/import progress bar.
type MonicaImportStatus struct {
	SessionID  string              `json:"session_id"`
	Phase      string              `json:"phase"`
	PhaseDone  int                 `json:"phase_done"`
	PhaseTotal int                 `json:"phase_total"`
	Error      string              `json:"error,omitempty"`
	Result     *MonicaImportResult `json:"result,omitempty"` // set from phase "importing_photos" onwards
}

// MonicaRowRelatedCounts is the per-contact tally of graph entities the
// import will create for that contact, shown as chips in the review step so
// it never promises more than the confirm will produce.
// Notes already includes Monica tasks and debts (the mapper folds them into
// dated notes); Activities already includes Monica logged calls. There is no
// separate "extra notes" bucket, unlike upstream.
type MonicaRowRelatedCounts struct {
	Activities    int `json:"activities"`
	Notes         int `json:"notes"`
	Reminders     int `json:"reminders"`
	Relationships int `json:"relationships"`
	Gifts         int `json:"gifts"`
}

// MonicaImportRowPreview is one contact row of the review step: the shared
// ImportRowPreview (parsed fields, validation, duplicate match, merge diff)
// plus Monica-specific context.
type MonicaImportRowPreview struct {
	ImportRowPreview
	Related  MonicaRowRelatedCounts `json:"related"`
	HasPhoto bool                   `json:"has_photo"`
}

// MonicaImportIssue is one entry of the loss report shown before confirm —
// the #442 record/field/category shape from services.ImportIssue.
type MonicaImportIssue struct {
	Record   string `json:"record"`
	Field    string `json:"field"`
	Category string `json:"category"`
	Message  string `json:"message"`
}

// MonicaPreviewResponse is the full review-step payload.
type MonicaPreviewResponse struct {
	SessionID      string                   `json:"session_id"`
	Rows           []MonicaImportRowPreview `json:"rows"`
	TotalRows      int                      `json:"total_rows"`
	ValidRows      int                      `json:"valid_rows"`
	DuplicateCount int                      `json:"duplicate_count"`
	ErrorCount     int                      `json:"error_count"`
	Totals         MonicaRowRelatedCounts   `json:"totals"`
	// LossReport is every mapping issue, so the user sees what the import
	// cannot carry before they commit (issue #442). Never omitempty: an
	// empty import still returns [] so the client can render "nothing lost".
	LossReport []MonicaImportIssue `json:"loss_report"`
}

// MonicaConfirmRequest executes the import with the per-contact decisions
// from the review step. Reuses RowImportAction (skip|add|update); "update"
// merges the mapped contact into the row's DuplicateMatch.
type MonicaConfirmRequest struct {
	SessionID string            `json:"session_id" validate:"required"`
	Actions   []RowImportAction `json:"actions" validate:"required,dive"`
}

// MonicaImportResult is the outcome summary. Embeds the shared ImportResult
// (total/created/updated/skipped/errors) and adds the graph-entity counts and
// the deferred photo progress.
type MonicaImportResult struct {
	ImportResult
	RelationshipsCreated int `json:"relationships_created"`
	NotesCreated         int `json:"notes_created"`
	ActivitiesCreated    int `json:"activities_created"`
	RemindersCreated     int `json:"reminders_created"`
	GiftsCreated         int `json:"gifts_created"`
	CustomFieldsCreated  int `json:"custom_fields_created"`
	PhotosQueued         int `json:"photos_queued"`
	PhotosSaved          int `json:"photos_saved"`
	PhotosFailed         int `json:"photos_failed"`
}
