package com.mycorrhizal.crm.ui.components

import androidx.compose.runtime.Composable
import androidx.compose.ui.res.stringResource
import com.mycorrhizal.crm.model.network.ErrorReason
import com.mycorrhizal.crm.model.network.ErrorReasonCode
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.ui.R

/**
 * The message to show for this error in the UI language (ADR 0038, issue
 * #2097). A known reason code renders its translated template with the server's
 * params; anything else falls back to [ApiError.displayMessage]. A 5xx also
 * gets the request reference, so a user can quote it to the operator.
 */
@Composable
fun ApiError.localizedMessage(): String {
    val base = localizedReason(reasonOrNull()) ?: displayMessage
    val requestId = (this as? ApiError.Server)?.requestId
    return if (requestId != null) {
        "$base ${stringResource(R.string.error_request_ref, requestId)}"
    } else {
        base
    }
}

private fun ApiError.reasonOrNull(): ErrorReason? = when (this) {
    is ApiError.Client -> reason
    is ApiError.Server -> reason
    is ApiError.PreconditionFailed -> reason
    else -> null
}

/**
 * The translated template for [reason], or null when this build does not know
 * the code or a param is missing or has the wrong type, so the caller falls back
 * to the server's English message.
 *
 * Every branch is an explicit `when` arm, not `Resources.getIdentifier`: that is
 * a lint `DiscouragedApi` finding, and R8 can strip strings that are only
 * referenced by name.
 */
@Composable
private fun localizedReason(reason: ErrorReason?): String? {
    reason ?: return null
    val field = reason.str("field")
    return when (reason.code) {
        ErrorReasonCode.COMMON_TEST_ONLY ->
            reason.int("n")?.let {
                stringResource(R.string.error_reason_common_test_only, it)
            }

        ErrorReasonCode.CONTACT_BULK_CUSTOM_FIELD_DUPLICATE ->
            stringResource(R.string.error_reason_contact_bulk_custom_field_duplicate)

        ErrorReasonCode.CONTACT_BULK_CUSTOM_FIELD_IN_BOTH ->
            stringResource(R.string.error_reason_contact_bulk_custom_field_in_both)

        ErrorReasonCode.CONTACT_BULK_CUSTOM_FIELD_INVALID ->
            field?.let {
                stringResource(R.string.error_reason_contact_bulk_custom_field_invalid, it)
            }

        ErrorReasonCode.CONTACT_BULK_CUSTOM_FIELD_NOT_FOUND ->
            stringResource(R.string.error_reason_contact_bulk_custom_field_not_found)

        ErrorReasonCode.CONTACT_BULK_FIELD_IN_SET_AND_CLEAR ->
            field?.let {
                stringResource(R.string.error_reason_contact_bulk_field_in_set_and_clear, it)
            }

        ErrorReasonCode.CONTACT_BULK_FIELD_NOT_EDITABLE ->
            field?.let {
                stringResource(R.string.error_reason_contact_bulk_field_not_editable, it)
            }

        ErrorReasonCode.CONTACT_BULK_NO_OPERATIONS ->
            stringResource(R.string.error_reason_contact_bulk_no_operations)

        ErrorReasonCode.CONTACT_BULK_VALUE_EMPTY ->
            field?.let {
                stringResource(R.string.error_reason_contact_bulk_value_empty, it)
            }

        ErrorReasonCode.CONTACT_BULK_VALUE_TOO_LONG -> {
            val limit = reason.int("limit")
            if (field != null && limit != null) {
                stringResource(R.string.error_reason_contact_bulk_value_too_long, field, limit)
            } else {
                null
            }
        }

        ErrorReasonCode.IMPORT_CIRCLES_NOT_FOUND ->
            stringResource(R.string.error_reason_import_circles_not_found)

        ErrorReasonCode.IMPORT_TAGS_NOT_FOUND ->
            stringResource(R.string.error_reason_import_tags_not_found)

        ErrorReasonCode.RELATIONSHIP_CONTACT_AMBIGUOUS ->
            stringResource(R.string.error_reason_relationship_contact_ambiguous)

        ErrorReasonCode.RELATIONSHIP_CONTACT_MISSING ->
            stringResource(R.string.error_reason_relationship_contact_missing)

        ErrorReasonCode.RELATIONSHIP_DAY_OUT_OF_RANGE ->
            stringResource(R.string.error_reason_relationship_day_out_of_range)

        ErrorReasonCode.RELATIONSHIP_END_BEFORE_START ->
            stringResource(R.string.error_reason_relationship_end_before_start)

        ErrorReasonCode.RELATIONSHIP_MONTH_OUT_OF_RANGE ->
            stringResource(R.string.error_reason_relationship_month_out_of_range)

        ErrorReasonCode.RELATIONSHIP_NOT_SUGGESTION ->
            stringResource(R.string.error_reason_relationship_not_suggestion)

        ErrorReasonCode.RELATIONSHIP_PARTIAL_DATE_MISSING ->
            stringResource(R.string.error_reason_relationship_partial_date_missing)

        ErrorReasonCode.RELATIONSHIP_SELF_EDGE ->
            stringResource(R.string.error_reason_relationship_self_edge)

        ErrorReasonCode.VALIDATION_ACTIVITY_IDS_RANGE ->
            stringResource(R.string.error_reason_validation_activity_ids_range)

        ErrorReasonCode.VALIDATION_CURSOR_MALFORMED ->
            stringResource(R.string.error_reason_validation_cursor_malformed)

        ErrorReasonCode.CIRCLE_COLOR_INVALID -> stringResource(
            R.string.error_reason_circle_color_invalid
        )

        ErrorReasonCode.MEERKAT_UPLOAD_MISSING ->
            stringResource(R.string.error_reason_meerkat_upload_missing)

        ErrorReasonCode.MEERKAT_SESSION_NOT_FOUND ->
            stringResource(R.string.error_reason_meerkat_session_not_found)

        ErrorReasonCode.MEERKAT_SESSION_NOT_OWNED ->
            stringResource(R.string.error_reason_meerkat_session_not_owned)

        ErrorReasonCode.MEERKAT_SESSION_EXPIRED ->
            stringResource(R.string.error_reason_meerkat_session_expired)

        ErrorReasonCode.MEERKAT_DB_SIZE ->
            stringResource(R.string.error_reason_meerkat_db_size)

        ErrorReasonCode.MEERKAT_UPLOAD_NOT_SQLITE ->
            stringResource(R.string.error_reason_meerkat_upload_not_sqlite)

        ErrorReasonCode.MEERKAT_UPLOAD_UNREADABLE ->
            stringResource(R.string.error_reason_meerkat_upload_unreadable)

        ErrorReasonCode.MEERKAT_FILE_NOT_MEERKAT ->
            stringResource(R.string.error_reason_meerkat_file_not_meerkat)

        ErrorReasonCode.MEERKAT_DB_UNREADABLE ->
            stringResource(R.string.error_reason_meerkat_db_unreadable)

        ErrorReasonCode.MEERKAT_DB_TOO_MANY_CONTACTS ->
            stringResource(R.string.error_reason_meerkat_db_too_many_contacts)

        ErrorReasonCode.MEERKAT_DB_TOO_MANY_RECORDS ->
            stringResource(R.string.error_reason_meerkat_db_too_many_records)

        ErrorReasonCode.MEERKAT_IMPORT_IN_PROGRESS ->
            stringResource(R.string.error_reason_meerkat_import_in_progress)

        ErrorReasonCode.MEERKAT_NOT_PREPARED ->
            stringResource(R.string.error_reason_meerkat_not_prepared)

        ErrorReasonCode.MEERKAT_API_ERROR ->
            reason.str(
                "reason"
            )?.let { stringResource(R.string.error_reason_meerkat_api_error, it) }

        ErrorReasonCode.MEERKAT_UPSTREAM_ERROR ->
            reason.str(
                "reason"
            )?.let { stringResource(R.string.error_reason_meerkat_upstream_error, it) }

        ErrorReasonCode.MEERKAT_SIGN_IN_INPUT_MISSING ->
            stringResource(R.string.error_reason_meerkat_sign_in_input_missing)

        ErrorReasonCode.MEERKAT_REMOTE_TOO_MANY_CONTACTS -> {
            val total = reason.int("total")
            val max = reason.int("max")
            if (total != null && max != null) {
                stringResource(R.string.error_reason_meerkat_remote_too_many_contacts, total, max)
            } else {
                null
            }
        }

        ErrorReasonCode.MEERKAT_VERSION_UNTESTED ->
            stringResource(R.string.error_reason_meerkat_version_untested)

        else -> null
    }
}

// Moshi decodes JSON numbers as Double, so an integral param is read through
// Number; a string that holds a number is accepted too.
private fun ErrorReason.str(name: String): String? = params[name]?.toString()

private fun ErrorReason.int(name: String): Int? = when (val value = params[name]) {
    is Number -> value.toInt()
    is String -> value.toIntOrNull()
    else -> null
}
