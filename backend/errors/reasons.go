package errors

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"mycorrhizal/logger"
)

// ReasonCode is a stable, machine-readable error reason (ADR 0038, issue
// #2095). Clients localize from it; `message` stays English.
type ReasonCode string

type reasonSpec struct {
	// Template is the English rendering; placeholders are {name}.
	Template string
	// Params is the declared param order. Android positional args (%N$s) are
	// numbered in this order.
	Params []string
}

// reasonAreas is the closed set of first segments a reason code may use. A
// new area is added here in the same PR as its first code.
var reasonAreas = []string{
	"common", "auth", "account", "admin", "import", "meerkat", "monica",
	"bundle", "csv", "vcard", "integration", "immich", "seafile", "paperless",
	"geopulse", "webdav", "calendar", "feed", "notification", "contact",
	"household", "relationship", "occasion", "photo", "attachment", "validation",
	"trash",
}

// Reason constants. Each must be a key of reasonRegistry (and vice versa);
// TestEveryReasonConstantRegistered enforces both directions.
const (
	// ReasonCommonTestOnly exists for tests only and is removed in #2100.
	ReasonCommonTestOnly ReasonCode = "common_test_only"

	ReasonImportCirclesNotFound          ReasonCode = "import_circles_not_found"
	ReasonImportTagsNotFound             ReasonCode = "import_tags_not_found"
	ReasonValidationCursorMalformed      ReasonCode = "validation_cursor_malformed"
	ReasonValidationActivityIDsRange     ReasonCode = "validation_activity_ids_range"
	ReasonRelationshipContactAmbiguous   ReasonCode = "relationship_contact_ambiguous"
	ReasonRelationshipContactMissing     ReasonCode = "relationship_contact_missing"
	ReasonRelationshipSelfEdge           ReasonCode = "relationship_self_edge"
	ReasonRelationshipPartialDateMissing ReasonCode = "relationship_partial_date_missing"
	ReasonRelationshipMonthOutOfRange    ReasonCode = "relationship_month_out_of_range"
	ReasonRelationshipDayOutOfRange      ReasonCode = "relationship_day_out_of_range"
	ReasonRelationshipEndBeforeStart     ReasonCode = "relationship_end_before_start"
	ReasonRelationshipNotSuggestion      ReasonCode = "relationship_not_suggestion"
	ReasonTrashUIDInUse                  ReasonCode = "trash_uid_in_use"
	ReasonTrashSnapshotUnsupported       ReasonCode = "trash_snapshot_unsupported"
	ReasonContactBulkNoOperations        ReasonCode = "contact_bulk_no_operations"
	ReasonContactBulkFieldNotEditable    ReasonCode = "contact_bulk_field_not_editable"
	ReasonContactBulkValueEmpty          ReasonCode = "contact_bulk_value_empty"
	ReasonContactBulkValueTooLong        ReasonCode = "contact_bulk_value_too_long"
	ReasonContactBulkFieldInSetAndClear  ReasonCode = "contact_bulk_field_in_set_and_clear"
	ReasonContactBulkCustomFieldNotFound ReasonCode = "contact_bulk_custom_field_not_found"
	ReasonContactBulkCustomFieldDupe     ReasonCode = "contact_bulk_custom_field_duplicate"
	ReasonContactBulkCustomFieldInBoth   ReasonCode = "contact_bulk_custom_field_in_both"
	ReasonContactBulkCustomFieldInvalid  ReasonCode = "contact_bulk_custom_field_invalid"
)

var reasonRegistry = map[ReasonCode]reasonSpec{
	ReasonCommonTestOnly: {Template: "Test {n}", Params: []string{"n"}},

	ReasonImportCirclesNotFound:          {Template: "One or more circles were not found"},
	ReasonImportTagsNotFound:             {Template: "One or more tags were not found"},
	ReasonValidationCursorMalformed:      {Template: "The cursor id is malformed"},
	ReasonValidationActivityIDsRange:     {Template: "Must contain between 1 and 100 ids"},
	ReasonRelationshipContactAmbiguous:   {Template: "Specify either an existing contact or a new contact, not both"},
	ReasonRelationshipContactMissing:     {Template: "Specify an existing contact or a new contact"},
	ReasonRelationshipSelfEdge:           {Template: "The source and target must be different contacts"},
	ReasonRelationshipPartialDateMissing: {Template: "A date must include at least a year, month or day"},
	ReasonRelationshipMonthOutOfRange:    {Template: "The month must be between 1 and 12"},
	ReasonRelationshipDayOutOfRange:      {Template: "The day must be between 1 and 31"},
	ReasonRelationshipEndBeforeStart:     {Template: "The end date cannot be before the start date"},
	ReasonRelationshipNotSuggestion:      {Template: "This relationship is not a pending suggestion"},
	ReasonTrashUIDInUse:                  {Template: "A live contact already uses this contact's UID; merge or delete it first"},
	ReasonTrashSnapshotUnsupported:       {Template: "This contact's deletion snapshot is not supported by this version; it can still be deleted permanently"},
	ReasonContactBulkNoOperations:        {Template: "Provide at least one of set, clear, custom_set or custom_clear"},
	ReasonContactBulkFieldNotEditable:    {Template: "{field} is not bulk-editable", Params: []string{"field"}},
	ReasonContactBulkValueEmpty:          {Template: "{field} must not be empty; list it in clear to empty it", Params: []string{"field"}},
	ReasonContactBulkValueTooLong:        {Template: "{field} must be at most {limit} characters", Params: []string{"field", "limit"}},
	ReasonContactBulkFieldInSetAndClear:  {Template: "{field} cannot be in both set and clear", Params: []string{"field"}},
	ReasonContactBulkCustomFieldNotFound: {Template: "A custom field definition was not found or is not yours"},
	ReasonContactBulkCustomFieldDupe:     {Template: "A custom field definition appears more than once"},
	ReasonContactBulkCustomFieldInBoth:   {Template: "A custom field cannot be in both custom_set and custom_clear"},
	ReasonContactBulkCustomFieldInvalid:  {Template: "{field} has an invalid value", Params: []string{"field"}},
}

var reasonPlaceholder = regexp.MustCompile(`\{([^{}]*)\}`)

// WithReason attaches a reason code and params and sets Message and
// Details["reason"] to the rendered English template. If the code is unknown
// or the params do not match the registered names exactly, the error is left
// untouched (clients fall back to English) and an info line is logged; a
// request path never panics over this.
func (e *AppError) WithReason(code ReasonCode, params map[string]any) *AppError {
	spec, ok := reasonRegistry[code]
	if !ok || !paramsMatch(spec.Params, params) {
		logger.Logger.Info().
			Str(logger.FieldEvent, "reason_params_mismatch").
			Str(logger.FieldReasonCode, string(code)).
			Msg("reason params do not match registry; sending error without reason")
		return e
	}
	rendered := reasonPlaceholder.ReplaceAllStringFunc(spec.Template, func(m string) string {
		return fmt.Sprint(params[m[1:len(m)-1]])
	})
	e.ReasonCode = string(code)
	e.ReasonParams = params
	e.Message = rendered
	e.WithDetails("reason", rendered)
	return e
}

func paramsMatch(names []string, params map[string]any) bool {
	if len(names) != len(params) {
		return false
	}
	for _, n := range names {
		if _, ok := params[n]; !ok {
			return false
		}
	}
	return true
}

// ReasonCodes returns every registered code, sorted. Client drift tests
// consume it.
func ReasonCodes() []ReasonCode {
	out := make([]ReasonCode, 0, len(reasonRegistry))
	for c := range reasonRegistry {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return strings.Compare(string(out[i]), string(out[j])) < 0 })
	return out
}

// ReasonSpecFor returns the English template and declared param order.
func ReasonSpecFor(code ReasonCode) (template string, params []string, ok bool) {
	spec, ok := reasonRegistry[code]
	if !ok {
		return "", nil, false
	}
	return spec.Template, append([]string(nil), spec.Params...), true
}
