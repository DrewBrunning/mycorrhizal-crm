package com.mycorrhizal.crm.model.network

/**
 * A server error's localizable reason (ADR 0038): the stable [code] from the
 * error envelope's `reason_code` and its flat [params]. Numbers arrive as
 * [Long] when whole, [Double] otherwise.
 */
data class ErrorReason(val code: String, val params: Map<String, Any?>)

/**
 * Error reason codes. Hand-mirrored from backend `errors.ReasonCodes()`
 * (CLAUDE.md trap #4; keep in sync with backend/errors/reasons.go): a code
 * added there must be added here, in `localizedMessage`, and as
 * `error_reason_<code>` in every `strings.xml`. Until then the client shows the
 * server's English message.
 */
object ErrorReasonCode {
    /** Test-only placeholder from #2095; removed in #2100. */
    const val COMMON_TEST_ONLY = "common_test_only"

    const val IMPORT_CIRCLES_NOT_FOUND = "import_circles_not_found"
    const val IMPORT_TAGS_NOT_FOUND = "import_tags_not_found"
    const val VALIDATION_CURSOR_MALFORMED = "validation_cursor_malformed"
    const val VALIDATION_ACTIVITY_IDS_RANGE = "validation_activity_ids_range"
    const val RELATIONSHIP_CONTACT_AMBIGUOUS = "relationship_contact_ambiguous"
    const val RELATIONSHIP_CONTACT_MISSING = "relationship_contact_missing"
    const val RELATIONSHIP_SELF_EDGE = "relationship_self_edge"
    const val RELATIONSHIP_PARTIAL_DATE_MISSING = "relationship_partial_date_missing"
    const val RELATIONSHIP_MONTH_OUT_OF_RANGE = "relationship_month_out_of_range"
    const val RELATIONSHIP_DAY_OUT_OF_RANGE = "relationship_day_out_of_range"
    const val RELATIONSHIP_END_BEFORE_START = "relationship_end_before_start"
    const val RELATIONSHIP_NOT_SUGGESTION = "relationship_not_suggestion"
    const val CONTACT_BULK_NO_OPERATIONS = "contact_bulk_no_operations"
    const val CONTACT_BULK_FIELD_NOT_EDITABLE = "contact_bulk_field_not_editable"
    const val CONTACT_BULK_VALUE_EMPTY = "contact_bulk_value_empty"
    const val CONTACT_BULK_VALUE_TOO_LONG = "contact_bulk_value_too_long"
    const val CONTACT_BULK_FIELD_IN_SET_AND_CLEAR = "contact_bulk_field_in_set_and_clear"
    const val CONTACT_BULK_CUSTOM_FIELD_NOT_FOUND = "contact_bulk_custom_field_not_found"
    const val CONTACT_BULK_CUSTOM_FIELD_DUPLICATE = "contact_bulk_custom_field_duplicate"
    const val CONTACT_BULK_CUSTOM_FIELD_IN_BOTH = "contact_bulk_custom_field_in_both"
    const val CONTACT_BULK_CUSTOM_FIELD_INVALID = "contact_bulk_custom_field_invalid"
}
