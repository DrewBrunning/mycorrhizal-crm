package com.mycorrhizal.crm.model.network

/**
 * A server error's stable reason (ADR 0038, issue #2097): [code] selects the
 * translated template and [params] fill it, in the registry's declared order.
 */
data class ErrorReason(val code: String, val params: Map<String, Any?>)

/**
 * Hand mirror of the backend error reason codes (frontend trap 4). Keep in
 * sync with backend/errors/reasons.go: the drift test in
 * backend/internal/compatci/android_error_reason_test.go fails on any
 * difference, including a code with no string in every locale.
 */
object ErrorReasonCode {
    const val COMMON_TEST_ONLY = "common_test_only"
    const val CONTACT_BULK_NO_FIELDS = "contact_bulk_no_fields"
    const val CONTACT_BULK_FIELD_NOT_EDITABLE = "contact_bulk_field_not_editable"
    const val CONTACT_BULK_VALUE_EMPTY = "contact_bulk_value_empty"
    const val CONTACT_BULK_VALUE_TOO_LONG = "contact_bulk_value_too_long"
    const val CONTACT_BULK_FIELD_IN_SET_AND_CLEAR = "contact_bulk_field_in_set_and_clear"
    const val CONTACT_BULK_CUSTOM_FIELD_UNKNOWN = "contact_bulk_custom_field_unknown"
    const val CONTACT_BULK_CUSTOM_FIELD_DUPLICATE = "contact_bulk_custom_field_duplicate"
    const val CONTACT_BULK_CUSTOM_VALUE_INVALID = "contact_bulk_custom_value_invalid"
    const val CONTACT_BULK_CUSTOM_FIELD_IN_BOTH = "contact_bulk_custom_field_in_both"
    const val RELATIONSHIP_ENDPOINT_BOTH = "relationship_endpoint_both"
    const val RELATIONSHIP_ENDPOINT_MISSING = "relationship_endpoint_missing"
    const val RELATIONSHIP_SELF_LINK = "relationship_self_link"
    const val RELATIONSHIP_DATE_EMPTY = "relationship_date_empty"
    const val RELATIONSHIP_DATE_MONTH_RANGE = "relationship_date_month_range"
    const val RELATIONSHIP_DATE_DAY_RANGE = "relationship_date_day_range"
    const val RELATIONSHIP_DATES_REVERSED = "relationship_dates_reversed"
    const val RELATIONSHIP_NOT_PENDING_SUGGESTION = "relationship_not_pending_suggestion"
    const val VALIDATION_CURSOR_MALFORMED = "validation_cursor_malformed"
    const val VALIDATION_ID_COUNT_RANGE = "validation_id_count_range"
    const val IMPORT_CIRCLES_NOT_FOUND = "import_circles_not_found"
    const val IMPORT_TAGS_NOT_FOUND = "import_tags_not_found"
}
