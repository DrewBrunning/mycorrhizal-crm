package models

import "time"

// ImportableContactFields defines the valid target fields for import
var ImportableContactFields = []string{
	// Scalars
	"firstname", "lastname", "middle_name", "prefix", "suffix", "nickname", "gender",
	"birthday", "anniversary", "organization", "department", "job_title", "role",
	"how_we_met", "work_information", "contact_information",
	// Groupings. Separate targets since T3: "circles" materializes Circle +
	// CircleMember rows, "tags" materializes Tag + ContactTag rows. Before
	// that, tag-shaped headers were folded into "circles" because Tag had no
	// destination.
	"circles", "tags",
	// Multi-value values
	"email", "phone", "url", "impp",
	"address_street", "address_city", "address_region", "address_postal", "address_country",
	// Multi-value labels/types
	"email_label", "phone_label", "url_label", "impp_label", "address_label",
}

// ColumnMapping represents how a CSV column maps to a contact field
type ColumnMapping struct {
	CSVColumn    string `json:"csv_column" validate:"required"`
	ContactField string `json:"contact_field"` // Empty means "ignore this column"
	Group        int    `json:"group"`         // Multi-value entry index (0-based); ties value+label/parts of one entry together
}

// ImportUploadResponse is returned after CSV upload
type ImportUploadResponse struct {
	SessionID         string          `json:"session_id"`
	Headers           []string        `json:"headers"`
	SuggestedMappings []ColumnMapping `json:"suggested_mappings"`
	RowCount          int             `json:"row_count"`
	SampleData        [][]string      `json:"sample_data"` // First few rows for preview
}

// ImportPreviewRequest is sent to request a preview with mappings
type ImportPreviewRequest struct {
	SessionID string          `json:"session_id" validate:"required"`
	Mappings  []ColumnMapping `json:"mappings" validate:"required,dive"`
}

// DuplicateMatch describes a potential duplicate contact
type DuplicateMatch struct {
	ExistingContactID uint   `json:"existing_contact_id"`
	ExistingFirstname string `json:"existing_firstname"`
	ExistingLastname  string `json:"existing_lastname"`
	ExistingEmail     string `json:"existing_email"`
	ExistingPhone     string `json:"existing_phone"`
	MatchReason       string `json:"match_reason"` // "email", "name", or "phone"
}

// ImportScalarChange is one scalar field the "Merge" (update) action will
// overwrite on the matched existing contact, reported as a before/after pair
// (T96 — docs/fork-plan/tickets/140-T96-import-duplicate-merge-review.md).
// Field is a stable camelCase key so the client can group/label by it; Label
// is the human-readable fallback the server already knows.
type ImportScalarChange struct {
	Field string `json:"field"`
	Label string `json:"label"`
	Old   string `json:"old"`
	New   string `json:"new"`
}

// ImportAddedValue is one multi-valued entry the "Merge" action will append
// to the matched existing contact — a phone/email/address/url/impp the
// existing contact does not already carry. Kind is a stable lowercase token
// ("email", "phone", "address", "url", "impp"); Value is a display rendering.
type ImportAddedValue struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// ImportMergeDiff describes, per duplicate row, exactly what the "Merge"
// (update) action will change on the matched existing contact: which scalars
// get overwritten (incoming-wins-when-non-empty, the MergeImportedContact
// policy) and which multi-valued entries get appended (additive, T49). It is
// computed by services.ComputeImportMergeDiff from the same helpers the
// confirm path applies, so the preview can never describe a merge that the
// commit would not perform. Updated and Added deliberately have no omitempty:
// absent-vs-`[]` must stay distinguishable (CLAUDE.md frontend trap #8) for
// the clients that render the diff.
type ImportMergeDiff struct {
	Updated []ImportScalarChange `json:"updated"`
	Added   []ImportAddedValue   `json:"added"`
}

// ImportRowPreview represents one row in the import preview
type ImportRowPreview struct {
	RowIndex         int                    `json:"row_index"`
	ParsedContact    map[string]interface{} `json:"parsed_contact"`    // Parsed field values
	ValidationErrors []string               `json:"validation_errors"` // Any validation issues
	DuplicateMatch   *DuplicateMatch        `json:"duplicate_match"`   // Potential duplicate, if any
	SuggestedAction  string                 `json:"suggested_action"`  // "add", "skip", or "update"

	// MergeDiff is present exactly when DuplicateMatch is: what "update"
	// would change on the matched existing contact (see ImportMergeDiff).
	// Absent (nil) for rows with no duplicate or rows whose existing contact
	// could not be loaded.
	MergeDiff *ImportMergeDiff `json:"merge_diff"`

	// BatchDuplicateOf, when non-nil, holds the row_index of an EARLIER row in
	// the SAME import file that this row duplicates (T96's within-batch
	// detection — the same person imported twice in one file). These rows
	// default to "skip"; the user may still override to Keep Both.
	BatchDuplicateOf *int `json:"batch_duplicate_of"`

	// Diagnostics surfaces contactmodel.Diagnostic events (WP-71 Gap 4) from
	// the vcard4/vcard3/jscontact adapter that parsed this row — e.g. a
	// gracefully-dropped, no-target-home field (docs/fork-plan/00-overview.md
	// §0.5's degradation policy). Empty for CSV-import rows (which don't go
	// through an adapter at all). Additive: existing preview consumers that
	// don't know about this field are unaffected.
	Diagnostics []string `json:"diagnostics,omitempty"`
}

// ImportRecordsRequest is the request body for POST /contacts/import/records
// (T96): a batch of neutral Card/CRM records — the same shape the REST
// create/update endpoints accept, which is what Android's device-contacts
// import produces — to be run through the standard import preview pipeline
// (duplicate detection, per-row merge diff, within-batch detection) and then
// confirmed via the shared confirm endpoint.
type ImportRecordsRequest struct {
	Records []ContactRecordInput `json:"records" validate:"required,min=1,max=500"`
}

// ImportPreviewResponse contains the full preview data
type ImportPreviewResponse struct {
	SessionID      string             `json:"session_id"`
	Rows           []ImportRowPreview `json:"rows"`
	TotalRows      int                `json:"total_rows"`
	ValidRows      int                `json:"valid_rows"`
	DuplicateCount int                `json:"duplicate_count"`
	ErrorCount     int                `json:"error_count"`
}

// RowImportAction specifies what to do with each row
type RowImportAction struct {
	RowIndex int    `json:"row_index" validate:"min=0"`
	Action   string `json:"action" validate:"required,oneof=skip add update"`
}

// ImportConfirmRequest is sent to execute the import
type ImportConfirmRequest struct {
	SessionID string            `json:"session_id" validate:"required"`
	Actions   []RowImportAction `json:"actions" validate:"required,dive"`
}

// ImportResult summarizes what happened during import
type ImportResult struct {
	TotalProcessed int      `json:"total_processed"`
	Created        int      `json:"created"`
	Updated        int      `json:"updated"`
	Skipped        int      `json:"skipped"`
	Errors         []string `json:"errors"`
}

// ImportSession stores temporary import data server-side (not persisted to DB)
type ImportSession struct {
	ID        string
	UserID    uint
	Headers   []string
	Rows      [][]string
	CreatedAt time.Time
	ExpiresAt time.Time
	// Cached preview data (set after PreviewImport is called)
	Mappings      []ColumnMapping
	PreviewRows   []ImportRowPreview
	PreviewCached bool
}
