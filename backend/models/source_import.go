package models

// Shared DTOs for the "source import" assistants — a live third-party system
// (Monica, issue #549) or an uploaded database (Meerkat, issue #550) pulled
// into a background session, reviewed with a loss report, then confirmed
// through the shared engine. Only the acquisition step differs between
// sources; everything from the review step on is this file.
//
// The phase and category vocabularies are mirrored by hand in
// frontend/src/api/sourceImport.ts and backend/openapi.yaml — there is no
// dynamic type-list endpoint by design (CLAUDE.md frontend trap #4).

// Source-import wizard phases, in rough order. Terminal: done / failed /
// cancelled. The fetch-* phases are Monica-specific; parsing_database /
// mapping are Meerkat-specific; the rest are shared.
const (
	SourceImportPhaseConnecting            = "connecting"
	SourceImportPhaseParsingDatabase       = "parsing_database"
	SourceImportPhaseMapping               = "mapping"
	SourceImportPhaseFetchingContacts      = "fetching_contacts"
	SourceImportPhaseFetchingActivities    = "fetching_activities"
	SourceImportPhaseFetchingNotes         = "fetching_notes"
	SourceImportPhaseFetchingReminders     = "fetching_reminders"
	SourceImportPhaseFetchingExtras        = "fetching_extras"
	SourceImportPhaseFetchingRelationships = "fetching_relationships"
	SourceImportPhaseBuildingPreview       = "building_preview"
	SourceImportPhaseReady                 = "ready"
	SourceImportPhaseImporting             = "importing"
	SourceImportPhaseImportingPhotos       = "importing_photos"
	SourceImportPhaseDone                  = "done"
	SourceImportPhaseFailed                = "failed"
	SourceImportPhaseCancelled             = "cancelled" // an in-flight import was cancelled; the transaction rolled back
)

// SourceImportStatus is the poll payload for the fetch/import progress bar.
type SourceImportStatus struct {
	SessionID  string              `json:"session_id"`
	Phase      string              `json:"phase"`
	PhaseDone  int                 `json:"phase_done"`
	PhaseTotal int                 `json:"phase_total"`
	Error      string              `json:"error,omitempty"`
	Result     *SourceImportResult `json:"result,omitempty"` // set from phase "importing"/"importing_photos" onwards
}

// SourceRelatedCounts is the per-contact tally of graph entities the import
// will create for that contact, shown as chips in the review step so it never
// promises more than the confirm will produce. Notes already folds in the
// source's "extra" record kinds (Monica tasks/debts; nothing extra for
// Meerkat); Activities already folds in Monica's logged calls.
type SourceRelatedCounts struct {
	Activities    int `json:"activities"`
	Notes         int `json:"notes"`
	Reminders     int `json:"reminders"`
	Relationships int `json:"relationships"`
	Gifts         int `json:"gifts"`
}

// SourceImportRowPreview is one contact row of the review step: the shared
// ImportRowPreview (parsed fields, validation, duplicate match, merge diff)
// plus source-import context.
type SourceImportRowPreview struct {
	ImportRowPreview
	Related  SourceRelatedCounts `json:"related"`
	HasPhoto bool                `json:"has_photo"`
}

// SourceImportIssue is one entry of the loss report shown before confirm —
// the #442 record/field/category shape from services.ImportIssue.
type SourceImportIssue struct {
	Record   string `json:"record"`
	Field    string `json:"field"`
	Category string `json:"category"`
	Message  string `json:"message"`
}

// SourceImportPreviewResponse is the full review-step payload.
type SourceImportPreviewResponse struct {
	SessionID      string                   `json:"session_id"`
	Rows           []SourceImportRowPreview `json:"rows"`
	TotalRows      int                      `json:"total_rows"`
	ValidRows      int                      `json:"valid_rows"`
	DuplicateCount int                      `json:"duplicate_count"`
	ErrorCount     int                      `json:"error_count"`
	Totals         SourceRelatedCounts      `json:"totals"`
	// LossReport is every mapping issue, so the user sees what the import
	// cannot carry before they commit (issue #442). Never omitempty: an empty
	// import still returns [] so the client can render "nothing lost".
	LossReport []SourceImportIssue `json:"loss_report"`
}

// SourceImportConfirmRequest executes the import with the per-contact
// decisions from the review step. Reuses RowImportAction (skip|add|update);
// "update" merges the mapped contact into the row's DuplicateMatch.
type SourceImportConfirmRequest struct {
	SessionID string            `json:"session_id" validate:"required"`
	Actions   []RowImportAction `json:"actions" validate:"required,dive"`
}

// SourceImportResult is the outcome summary. Embeds the shared ImportResult
// (total/created/updated/skipped/errors) and adds the graph-entity counts and
// (Monica only) the deferred photo progress — Meerkat leaves the photo
// counters at zero.
type SourceImportResult struct {
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
