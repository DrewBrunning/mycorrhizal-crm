package com.mycorrhizal.crm.ui.components

import androidx.compose.runtime.Composable
import androidx.compose.ui.res.stringResource
import com.mycorrhizal.crm.model.network.ErrorReason
import com.mycorrhizal.crm.model.network.ErrorReasonCode
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.ui.R

/**
 * The user-facing text for this error, in the UI language (ADR 0038, issue
 * #2097). A server error's `reason_code` + `reason_params` are rendered from an
 * explicit `when` over [ErrorReasonCode] (not `Resources.getIdentifier`, which
 * is lint `DiscouragedApi` and which R8 resource shrinking can break), the same
 * pattern as [scoreReasonText].
 *
 * Falls back to [ApiError.displayMessage] for an unknown code, a missing
 * parameter, or a server that predates reason codes. A 5xx with a request id
 * gets the translated "(ref: …)" suffix, matching the web client.
 */
@Composable
fun ApiError.localizedMessage(): String {
    val reason = when (this) {
        is ApiError.Client -> reason
        is ApiError.Server -> reason
        is ApiError.PreconditionFailed -> reason
        else -> null
    }
    val message = reasonText(reason) ?: displayMessage
    val ref = (this as? ApiError.Server)?.requestId?.takeIf { it.isNotBlank() }
    if (ref == null) return message
    val suffix = stringResource(R.string.error_request_ref, ref)
    return "$message $suffix"
}

/**
 * One branch per [ErrorReasonCode]. Args are taken in the registry's declared
 * param order (backend `reasonRegistry`), which is how `%N$` is numbered in
 * strings.xml; a missing param returns null so the caller falls back.
 */
@Composable
private fun reasonText(reason: ErrorReason?): String? {
    if (reason == null) return null
    return when (reason.code) {
        ErrorReasonCode.COMMON_TEST_ONLY -> reason.params["n"]?.let { n ->
            stringResource(R.string.error_reason_common_test_only, n)
        }

        ErrorReasonCode.IMPORT_CIRCLES_NOT_FOUND -> stringResource(
            R.string.error_reason_import_circles_not_found
        )

        ErrorReasonCode.IMPORT_TAGS_NOT_FOUND -> stringResource(
            R.string.error_reason_import_tags_not_found
        )

        ErrorReasonCode.VALIDATION_CURSOR_MALFORMED -> stringResource(
            R.string.error_reason_validation_cursor_malformed
        )

        ErrorReasonCode.VALIDATION_ACTIVITY_IDS_RANGE -> stringResource(
            R.string.error_reason_validation_activity_ids_range
        )

        ErrorReasonCode.RELATIONSHIP_CONTACT_AMBIGUOUS -> stringResource(
            R.string.error_reason_relationship_contact_ambiguous
        )

        ErrorReasonCode.RELATIONSHIP_CONTACT_MISSING -> stringResource(
            R.string.error_reason_relationship_contact_missing
        )

        ErrorReasonCode.RELATIONSHIP_SELF_EDGE -> stringResource(
            R.string.error_reason_relationship_self_edge
        )

        ErrorReasonCode.RELATIONSHIP_PARTIAL_DATE_MISSING -> stringResource(
            R.string.error_reason_relationship_partial_date_missing
        )

        ErrorReasonCode.RELATIONSHIP_MONTH_OUT_OF_RANGE -> stringResource(
            R.string.error_reason_relationship_month_out_of_range
        )

        ErrorReasonCode.RELATIONSHIP_DAY_OUT_OF_RANGE -> stringResource(
            R.string.error_reason_relationship_day_out_of_range
        )

        ErrorReasonCode.RELATIONSHIP_END_BEFORE_START -> stringResource(
            R.string.error_reason_relationship_end_before_start
        )

        ErrorReasonCode.RELATIONSHIP_NOT_SUGGESTION -> stringResource(
            R.string.error_reason_relationship_not_suggestion
        )

        ErrorReasonCode.CONTACT_BULK_NO_OPERATIONS -> stringResource(
            R.string.error_reason_contact_bulk_no_operations
        )

        ErrorReasonCode.CONTACT_BULK_FIELD_NOT_EDITABLE -> reason.params["field"]?.let { field ->
            stringResource(R.string.error_reason_contact_bulk_field_not_editable, field)
        }

        ErrorReasonCode.CONTACT_BULK_VALUE_EMPTY -> reason.params["field"]?.let { field ->
            stringResource(R.string.error_reason_contact_bulk_value_empty, field)
        }

        ErrorReasonCode.CONTACT_BULK_VALUE_TOO_LONG -> {
            val field = reason.params["field"]
            val limit = reason.params["limit"]
            if (field == null || limit == null) {
                null
            } else {
                stringResource(
                    R.string.error_reason_contact_bulk_value_too_long,
                    field,
                    limit
                )
            }
        }

        ErrorReasonCode.CONTACT_BULK_FIELD_IN_SET_AND_CLEAR -> reason.params["field"]?.let { field ->
            stringResource(R.string.error_reason_contact_bulk_field_in_set_and_clear, field)
        }

        ErrorReasonCode.CONTACT_BULK_CUSTOM_FIELD_NOT_FOUND -> stringResource(
            R.string.error_reason_contact_bulk_custom_field_not_found
        )

        ErrorReasonCode.CONTACT_BULK_CUSTOM_FIELD_DUPLICATE -> stringResource(
            R.string.error_reason_contact_bulk_custom_field_duplicate
        )

        ErrorReasonCode.CONTACT_BULK_CUSTOM_FIELD_IN_BOTH -> stringResource(
            R.string.error_reason_contact_bulk_custom_field_in_both
        )

        ErrorReasonCode.CONTACT_BULK_CUSTOM_FIELD_INVALID -> reason.params["field"]?.let { field ->
            stringResource(R.string.error_reason_contact_bulk_custom_field_invalid, field)
        }

        else -> null
    }
}
