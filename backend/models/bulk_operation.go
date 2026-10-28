package models

// Bulk contact operations (N5 — N5).
// A single batch endpoint runs one action across many contacts with
// partial-success semantics instead of all-or-nothing: the response reports
// which contacts succeeded and which failed.
//
// Targets are Contact.VCardUID strings, not numeric IDs — membership rows
// (CircleMember/ContactTag) are keyed by VCardUID, and resolving through it
// scopes every row to the authenticated user (the ownership rule: a contact
// belonging to another user is reported as a failure, never silently skipped).

// BulkContactOperationInput is the request DTO for POST /contacts/bulk.
type BulkContactOperationInput struct {
	// Action is which bulk operation to run.
	Action string `json:"action" validate:"required,oneof=add_circle remove_circle add_tag remove_tag archive unarchive delete update_fields"`

	// VCardUIDs are the target contacts (Contact.VCardUID). Bounded to 500 so
	// a single request can't blow up the worker.
	VCardUIDs []string `json:"vcard_uids" validate:"required,min=1,max=500,dive,required,uuid4"`

	// CircleID/TagID identify the target circle/tag for the membership
	// actions; ignored otherwise.
	CircleID string `json:"circle_id,omitempty" validate:"omitempty,uuid4"`
	TagID    string `json:"tag_id,omitempty" validate:"omitempty,uuid4"`

	// Set, Clear, CustomSet and CustomClear drive the "update_fields" action
	// (issue #1996) and are ignored otherwise. Semantics, unambiguously:
	//   - a core field key present in Set is written (non-empty value);
	//   - a core field key listed in Clear is emptied;
	//   - a field in neither is left untouched;
	//   - the same key in both Set and Clear is a 400.
	// Set/Clear keys are restricted to BulkEditableFields (an explicit
	// allowlist, never mass assignment); the controller rejects unknown keys.
	Set         map[string]string `json:"set"`
	Clear       []string          `json:"clear" validate:"omitempty,max=20,dive,required"`
	CustomSet   []FieldValueInput `json:"custom_set" validate:"omitempty,max=50,dive"`
	CustomClear []string          `json:"custom_clear" validate:"omitempty,max=50,dive,required,uuid4"`
}

// BulkEditableFields is the allowlist of core contact fields the bulk
// "update_fields" action may write, with each field's maximum length (in
// runes), mirroring the Contact model's own validate tags. Multi-valued fields
// (addresses, emails, phones) are deliberately absent: setting "the city"
// would have to silently pick one address (issue #1996 defers that to a
// separate, explicit action).
var BulkEditableFields = map[string]int{
	"organization":        200,
	"department":          200,
	"job_title":           200,
	"role":                200,
	"gender":              100,
	"how_we_met":          1000,
	"work_information":    1000,
	"contact_information": 1000,
}

// BulkFailureEntry records one per-contact failure.
type BulkFailureEntry struct {
	VCardUID string `json:"vcard_uid"`
	Reason   string `json:"reason"`
}

// BulkOperationResult is the partial-success response.
type BulkOperationResult struct {
	Action    string             `json:"action"`
	Total     int                `json:"total"`
	Succeeded int                `json:"succeeded"`
	Failed    int                `json:"failed"`
	Failures  []BulkFailureEntry `json:"failures"`
}
