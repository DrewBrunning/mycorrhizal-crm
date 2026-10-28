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
}

// Reason constants. Each must be a key of reasonRegistry (and vice versa);
// TestEveryReasonConstantRegistered enforces both directions.
const (
	// ReasonCommonTestOnly exists for tests only and is removed in #2100.
	ReasonCommonTestOnly ReasonCode = "common_test_only"

	// Bulk contact edits (issue #1996, controllers/bulk_update_fields.go).
	ReasonContactBulkNoFields           ReasonCode = "contact_bulk_no_fields"
	ReasonContactBulkFieldNotEditable   ReasonCode = "contact_bulk_field_not_editable"
	ReasonContactBulkValueEmpty         ReasonCode = "contact_bulk_value_empty"
	ReasonContactBulkValueTooLong       ReasonCode = "contact_bulk_value_too_long"
	ReasonContactBulkFieldInSetAndClear ReasonCode = "contact_bulk_field_in_set_and_clear"
	ReasonContactBulkCustomFieldUnknown ReasonCode = "contact_bulk_custom_field_unknown"
	ReasonContactBulkCustomFieldDupe    ReasonCode = "contact_bulk_custom_field_duplicate"
	ReasonContactBulkCustomValueInvalid ReasonCode = "contact_bulk_custom_value_invalid"
	ReasonContactBulkCustomFieldInBoth  ReasonCode = "contact_bulk_custom_field_in_both"

	// Relationship edge endpoints and periods (controllers/relationship_edge_controller.go).
	ReasonRelationshipEndpointBoth         ReasonCode = "relationship_endpoint_both"
	ReasonRelationshipEndpointMissing      ReasonCode = "relationship_endpoint_missing"
	ReasonRelationshipSelfLink             ReasonCode = "relationship_self_link"
	ReasonRelationshipDateEmpty            ReasonCode = "relationship_date_empty"
	ReasonRelationshipDateMonthRange       ReasonCode = "relationship_date_month_range"
	ReasonRelationshipDateDayRange         ReasonCode = "relationship_date_day_range"
	ReasonRelationshipDatesReversed        ReasonCode = "relationship_dates_reversed"
	ReasonRelationshipNotPendingSuggestion ReasonCode = "relationship_not_pending_suggestion"

	// Shared request validation (activity cursors, id-count bounds).
	ReasonValidationCursorMalformed ReasonCode = "validation_cursor_malformed"
	ReasonValidationIDCountRange    ReasonCode = "validation_id_count_range"

	// Import groupings (services/import_groupings_service.go).
	ReasonImportCirclesNotFound ReasonCode = "import_circles_not_found"
	ReasonImportTagsNotFound    ReasonCode = "import_tags_not_found"
)

var reasonRegistry = map[ReasonCode]reasonSpec{
	ReasonCommonTestOnly: {Template: "Test {n}", Params: []string{"n"}},

	ReasonContactBulkNoFields:           {Template: "Choose at least one field to change"},
	ReasonContactBulkFieldNotEditable:   {Template: "{field} cannot be bulk-edited", Params: []string{"field"}},
	ReasonContactBulkValueEmpty:         {Template: "{field} cannot be empty; list it in clear to empty it", Params: []string{"field"}},
	ReasonContactBulkValueTooLong:       {Template: "{field} length limit is {limit}", Params: []string{"field", "limit"}},
	ReasonContactBulkFieldInSetAndClear: {Template: "{field} is in both set and clear", Params: []string{"field"}},
	ReasonContactBulkCustomFieldUnknown: {Template: "Unknown or not-owned custom field definition"},
	ReasonContactBulkCustomFieldDupe:    {Template: "A custom field definition is listed more than once"},
	ReasonContactBulkCustomValueInvalid: {Template: "{reason}", Params: []string{"reason"}},
	ReasonContactBulkCustomFieldInBoth:  {Template: "A custom field definition cannot be in both custom_set and custom_clear"},

	ReasonRelationshipEndpointBoth:         {Template: "Specify either an existing contact or a new contact for {field}, not both", Params: []string{"field"}},
	ReasonRelationshipEndpointMissing:      {Template: "Specify either an existing contact or a new contact for {field}", Params: []string{"field"}},
	ReasonRelationshipSelfLink:             {Template: "Source and target must be different contacts"},
	ReasonRelationshipDateEmpty:            {Template: "{field} must carry at least a year, month or day", Params: []string{"field"}},
	ReasonRelationshipDateMonthRange:       {Template: "{field} month must be 1–12", Params: []string{"field"}},
	ReasonRelationshipDateDayRange:         {Template: "{field} day must be 1–31", Params: []string{"field"}},
	ReasonRelationshipDatesReversed:        {Template: "End date precedes the start date"},
	ReasonRelationshipNotPendingSuggestion: {Template: "Relationship is not a pending suggestion"},

	ReasonValidationCursorMalformed: {Template: "{field} cursor id is malformed", Params: []string{"field"}},
	ReasonValidationIDCountRange:    {Template: "{field} count range: {min}–{max}", Params: []string{"field", "min", "max"}},

	ReasonImportCirclesNotFound: {Template: "One or more circles were not found"},
	ReasonImportTagsNotFound:    {Template: "One or more tags were not found"},
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
