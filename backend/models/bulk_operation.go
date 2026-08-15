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
	Action string `json:"action" validate:"required,oneof=add_circle remove_circle add_tag remove_tag archive unarchive delete"`

	// VCardUIDs are the target contacts (Contact.VCardUID). Bounded to 500 so
	// a single request can't blow up the worker.
	VCardUIDs []string `json:"vcard_uids" validate:"required,min=1,max=500,dive,required,uuid4"`

	// CircleID/TagID identify the target circle/tag for the membership
	// actions; ignored otherwise.
	CircleID string `json:"circle_id,omitempty" validate:"omitempty,uuid4"`
	TagID    string `json:"tag_id,omitempty" validate:"omitempty,uuid4"`
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
