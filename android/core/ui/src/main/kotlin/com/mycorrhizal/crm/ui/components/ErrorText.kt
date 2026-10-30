package com.mycorrhizal.crm.ui.components

import androidx.compose.runtime.Composable
import androidx.compose.ui.res.stringResource
import com.mycorrhizal.crm.model.network.ErrorReason
import com.mycorrhizal.crm.model.network.ErrorReasonCode
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.NO_PENDING_TWO_FACTOR_BODY
import com.mycorrhizal.crm.ui.R

/**
 * The message to show for this error in the UI language (ADR 0038, issue
 * #2097). A known reason code renders its translated template with the server's
 * params; anything else falls back to [ApiError.displayMessage]. A 5xx also
 * gets the request reference, so a user can quote it to the operator.
 */
@Composable
fun ApiError.localizedMessage(): String {
    val base = localizedReason(reasonOrNull()) ?: localizedValidationFields() ?: clientMessage()
    val requestId = (this as? ApiError.Server)?.requestId
    return if (requestId != null) {
        "$base ${stringResource(R.string.error_request_ref, requestId)}"
    } else {
        base
    }
}

/**
 * The translated per-field messages for a VALIDATION_ERROR (issue #2105), or
 * null when any entry cannot be rendered, in which case the caller falls back
 * to the English `details` map.
 */
@Composable
private fun ApiError.localizedValidationFields(): String? {
    val (errorCode, fieldReasons, details) = when (this) {
        is ApiError.Client -> Triple(errorCode, fieldReasons, details)
        is ApiError.Server -> Triple(errorCode, fieldReasons, details)
        else -> return null
    }
    if (errorCode != VALIDATION_ERROR && fieldReasons.isNullOrEmpty()) return null

    if (!fieldReasons.isNullOrEmpty()) {
        val messages = fieldReasons.values.map { localizedReason(it) }
        if (messages.all { it != null }) {
            return messages.filterNotNull().joinToString(". ")
        }
    }

    val values = details?.values?.filterNotNull()?.map { it.toString() }.orEmpty()
    return if (values.isEmpty()) null else values.joinToString(". ")
}

/**
 * Like [localizedMessage], but a 403 shows the server's own text. Use it only
 * where that refusal is the informative part (the OIDC "two-factor is not
 * available" refusal) and has no reason code yet.
 */
@Composable
fun ApiError.localizedMessageOrServerText(): String =
    if (this is ApiError.Client && code == 403 && reason == null && body.isNotBlank()) {
        body
    } else {
        localizedMessage()
    }

/**
 * The translated text for the errors the client words itself (no network,
 * session expiry, the generic HTTP fallbacks). Server-worded bodies that carry
 * no reason code (507, external-service, a plain 4xx message) stay as sent.
 */
@Composable
private fun ApiError.clientMessage(): String = when (this) {
    is ApiError.Network -> stringResource(R.string.error_client_network)

    is ApiError.Timeout -> stringResource(R.string.error_client_timeout)

    is ApiError.Server ->
        if (displayMessage == body) body else stringResource(R.string.error_client_server, code)

    is ApiError.Client -> when (code) {
        401 -> if (body == NO_PENDING_TWO_FACTOR_BODY) {
            stringResource(R.string.error_client_no_pending_two_factor)
        } else {
            stringResource(R.string.error_client_session_expired)
        }

        403 -> stringResource(R.string.error_client_forbidden)

        404 -> stringResource(R.string.error_client_not_found)

        409 -> body.ifBlank { stringResource(R.string.error_client_conflict) }

        else -> body.ifBlank { stringResource(R.string.error_client_request_failed) }
    }

    is ApiError.PreconditionFailed -> stringResource(R.string.error_client_precondition_failed)

    is ApiError.Parse -> stringResource(R.string.error_client_parse)

    is ApiError.ArchivedProfile -> stringResource(R.string.error_client_archived_profile)

    is ApiError.Unknown -> stringResource(R.string.error_client_unknown)
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

        ErrorReasonCode.CONTACT_FIELD_OVERRIDE_STATE_INVALID ->
            field?.let {
                stringResource(R.string.error_reason_contact_field_override_state_invalid, it)
            }

        ErrorReasonCode.CONTACT_FIELD_OVERRIDE_UNKNOWN_KEY ->
            field?.let {
                stringResource(R.string.error_reason_contact_field_override_unknown_key, it)
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

        ErrorReasonCode.VALIDATION_REQUIRED ->
            field?.let { stringResource(R.string.error_reason_validation_required, it) }

        ErrorReasonCode.VALIDATION_EMAIL ->
            field?.let { stringResource(R.string.error_reason_validation_email, it) }

        ErrorReasonCode.VALIDATION_MIN -> {
            val min = reason.int("min")
            if (field != null && min != null) {
                stringResource(R.string.error_reason_validation_min, field, min)
            } else {
                null
            }
        }

        ErrorReasonCode.VALIDATION_MAX -> {
            val max = reason.int("max")
            if (field != null && max != null) {
                stringResource(R.string.error_reason_validation_max, field, max)
            } else {
                null
            }
        }

        ErrorReasonCode.VALIDATION_PHONE ->
            field?.let { stringResource(R.string.error_reason_validation_phone, it) }

        ErrorReasonCode.VALIDATION_BIRTHDAY ->
            field?.let { stringResource(R.string.error_reason_validation_birthday, it) }

        ErrorReasonCode.VALIDATION_STRONG_PASSWORD ->
            field?.let { stringResource(R.string.error_reason_validation_strong_password, it) }

        ErrorReasonCode.VALIDATION_UNIQUE_CIRCLES ->
            field?.let { stringResource(R.string.error_reason_validation_unique_circles, it) }

        ErrorReasonCode.VALIDATION_NO_AT_SIGN ->
            field?.let { stringResource(R.string.error_reason_validation_no_at_sign, it) }

        ErrorReasonCode.VALIDATION_URL ->
            field?.let { stringResource(R.string.error_reason_validation_url, it) }

        ErrorReasonCode.VALIDATION_SAFEURL ->
            field?.let { stringResource(R.string.error_reason_validation_safeurl, it) }

        ErrorReasonCode.VALIDATION_HTTPURL ->
            field?.let { stringResource(R.string.error_reason_validation_httpurl, it) }

        ErrorReasonCode.VALIDATION_RELATION_TYPE ->
            field?.let { stringResource(R.string.error_reason_validation_relation_type, it) }

        ErrorReasonCode.VALIDATION_GEOURI ->
            field?.let { stringResource(R.string.error_reason_validation_geouri, it) }

        ErrorReasonCode.VALIDATION_INVALID ->
            field?.let { stringResource(R.string.error_reason_validation_invalid, it) }

        ErrorReasonCode.TRASH_UID_IN_USE -> stringResource(
            R.string.error_reason_trash_uid_in_use
        )

        ErrorReasonCode.TRASH_SNAPSHOT_UNSUPPORTED -> stringResource(
            R.string.error_reason_trash_snapshot_unsupported
        )

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

        ErrorReasonCode.COMMON_DATABASE_ERROR -> stringResource(
            R.string.error_reason_common_database_error
        )

        ErrorReasonCode.COMMON_DATABASE_BUSY -> stringResource(
            R.string.error_reason_common_database_busy
        )

        ErrorReasonCode.AUTH_INVALID_CREDENTIALS -> stringResource(
            R.string.error_reason_auth_invalid_credentials
        )

        ErrorReasonCode.AUTH_TOKEN_EXPIRED -> stringResource(
            R.string.error_reason_auth_token_expired
        )

        ErrorReasonCode.AUTH_TOKEN_INVALID -> stringResource(
            R.string.error_reason_auth_token_invalid
        )

        ErrorReasonCode.COMMON_CLIENT_NOT_SUPPORTED ->
            reason.str(
                "required_version"
            )?.let { stringResource(R.string.error_reason_common_client_not_supported, it) }

        ErrorReasonCode.COMMON_RATE_LIMITED -> stringResource(
            R.string.error_reason_common_rate_limited
        )

        ErrorReasonCode.COMMON_PAYLOAD_TOO_LARGE -> stringResource(
            R.string.error_reason_common_payload_too_large
        )

        ErrorReasonCode.COMMON_IDEMPOTENCY_KEY_REUSED -> stringResource(
            R.string.error_reason_common_idempotency_key_reused
        )

        ErrorReasonCode.COMMON_REQUEST_IN_PROGRESS -> stringResource(
            R.string.error_reason_common_request_in_progress
        )

        ErrorReasonCode.COMMON_REQUEST_RESULT_UNAVAILABLE -> stringResource(
            R.string.error_reason_common_request_result_unavailable
        )

        ErrorReasonCode.VALIDATION_MISSING_FIELD ->
            field?.let { stringResource(R.string.error_reason_validation_missing_field, it) }

        ErrorReasonCode.COMMON_UNEXPECTED_ERROR -> stringResource(
            R.string.error_reason_common_unexpected_error
        )

        ErrorReasonCode.ACTIVITY_END_BEFORE_START -> stringResource(
            R.string.error_reason_activity_end_before_start
        )

        ErrorReasonCode.ACTIVITY_NOT_FOUND -> stringResource(
            R.string.error_reason_activity_not_found
        )

        ErrorReasonCode.CONTACT_ADDRESS_NOT_FOUND -> stringResource(
            R.string.error_reason_contact_address_not_found
        )

        ErrorReasonCode.ACCOUNT_API_TOKEN_NOT_FOUND -> stringResource(
            R.string.error_reason_account_api_token_not_found
        )

        ErrorReasonCode.ATTACHMENT_NOT_FOUND -> stringResource(
            R.string.error_reason_attachment_not_found
        )

        ErrorReasonCode.ATTACHMENT_FILE_NOT_FOUND -> stringResource(
            R.string.error_reason_attachment_file_not_found
        )

        ErrorReasonCode.ADMIN_AUDIT_EVENT_NOT_FOUND -> stringResource(
            R.string.error_reason_admin_audit_event_not_found
        )

        ErrorReasonCode.CONTACT_CADENCE_POLICY_NOT_FOUND -> stringResource(
            R.string.error_reason_contact_cadence_policy_not_found
        )

        ErrorReasonCode.CONTACT_CADENCE_POLICY_ALREADY_EXISTS -> stringResource(
            R.string.error_reason_contact_cadence_policy_already_exists
        )

        ErrorReasonCode.CALENDAR_SUBSCRIPTION_NOT_FOUND -> stringResource(
            R.string.error_reason_calendar_subscription_not_found
        )

        ErrorReasonCode.CIRCLE_NOT_FOUND -> stringResource(R.string.error_reason_circle_not_found)

        ErrorReasonCode.CIRCLE_MEMBERSHIP_NOT_FOUND -> stringResource(
            R.string.error_reason_circle_membership_not_found
        )

        ErrorReasonCode.CIRCLE_MEMBERSHIP_ALREADY_EXISTS -> stringResource(
            R.string.error_reason_circle_membership_already_exists
        )

        ErrorReasonCode.CONTACT_NOT_FOUND -> stringResource(R.string.error_reason_contact_not_found)

        ErrorReasonCode.CONTACT_SHARE_NOT_FOUND -> stringResource(
            R.string.error_reason_contact_share_not_found
        )

        ErrorReasonCode.CONTACT_SUBSCRIPTION_NOT_FOUND -> stringResource(
            R.string.error_reason_contact_subscription_not_found
        )

        ErrorReasonCode.CONTACT_CONVERSATION_AGENDA_ITEM_NOT_FOUND -> stringResource(
            R.string.error_reason_contact_conversation_agenda_item_not_found
        )

        ErrorReasonCode.CONTACT_DATA_DECAY_POLICY_NOT_FOUND -> stringResource(
            R.string.error_reason_contact_data_decay_policy_not_found
        )

        ErrorReasonCode.CONTACT_DATA_DECAY_POLICY_ALREADY_EXISTS -> stringResource(
            R.string.error_reason_contact_data_decay_policy_already_exists
        )

        ErrorReasonCode.AUTH_DEVICE_GRANT_NOT_FOUND -> stringResource(
            R.string.error_reason_auth_device_grant_not_found
        )

        ErrorReasonCode.NOTIFICATION_DEVICE_REGISTRATION_NOT_FOUND -> stringResource(
            R.string.error_reason_notification_device_registration_not_found
        )

        ErrorReasonCode.OCCASION_EVENT_ATTENDEE_NOT_FOUND -> stringResource(
            R.string.error_reason_occasion_event_attendee_not_found
        )

        ErrorReasonCode.OCCASION_EVENT_ATTENDEE_ALREADY_EXISTS -> stringResource(
            R.string.error_reason_occasion_event_attendee_already_exists
        )

        ErrorReasonCode.CONTACT_EXTERNAL_ACTIVITY_NOT_FOUND -> stringResource(
            R.string.error_reason_contact_external_activity_not_found
        )

        ErrorReasonCode.CONTACT_EXTERNAL_ACTIVITY_ALREADY_EXISTS -> stringResource(
            R.string.error_reason_contact_external_activity_already_exists
        )

        ErrorReasonCode.INTEGRATION_EXTERNAL_IDENTITY_NOT_FOUND -> stringResource(
            R.string.error_reason_integration_external_identity_not_found
        )

        ErrorReasonCode.INTEGRATION_EXTERNAL_IDENTITY_ALREADY_EXISTS -> stringResource(
            R.string.error_reason_integration_external_identity_already_exists
        )

        ErrorReasonCode.FEED_NOT_FOUND -> stringResource(R.string.error_reason_feed_not_found)

        ErrorReasonCode.CONTACT_FIELD_DEFINITION_NOT_FOUND -> stringResource(
            R.string.error_reason_contact_field_definition_not_found
        )

        ErrorReasonCode.CONTACT_FIELD_DEFINITION_ALREADY_EXISTS -> stringResource(
            R.string.error_reason_contact_field_definition_already_exists
        )

        ErrorReasonCode.GEOPULSE_RESOURCE_NOT_FOUND -> stringResource(
            R.string.error_reason_geopulse_resource_not_found
        )

        ErrorReasonCode.CONTACT_GIFT_NOT_FOUND -> stringResource(
            R.string.error_reason_contact_gift_not_found
        )

        ErrorReasonCode.HOUSEHOLD_NOT_FOUND -> stringResource(
            R.string.error_reason_household_not_found
        )

        ErrorReasonCode.HOUSEHOLD_MEMBERSHIP_NOT_FOUND -> stringResource(
            R.string.error_reason_household_membership_not_found
        )

        ErrorReasonCode.HOUSEHOLD_MEMBERSHIP_ALREADY_EXISTS -> stringResource(
            R.string.error_reason_household_membership_already_exists
        )

        ErrorReasonCode.HOUSEHOLD_SUGGESTION_DISMISSAL_ALREADY_EXISTS -> stringResource(
            R.string.error_reason_household_suggestion_dismissal_already_exists
        )

        ErrorReasonCode.PHOTO_IMAGE_NOT_FOUND -> stringResource(
            R.string.error_reason_photo_image_not_found
        )

        ErrorReasonCode.IMMICH_LINK_NOT_FOUND -> stringResource(
            R.string.error_reason_immich_link_not_found
        )

        ErrorReasonCode.IMMICH_LINK_ALREADY_EXISTS -> stringResource(
            R.string.error_reason_immich_link_already_exists
        )

        ErrorReasonCode.IMMICH_PERSON_NOT_FOUND -> stringResource(
            R.string.error_reason_immich_person_not_found
        )

        ErrorReasonCode.CONTACT_LIFE_EVENT_NOT_FOUND -> stringResource(
            R.string.error_reason_contact_life_event_not_found
        )

        ErrorReasonCode.CONTACT_LINK_FIELD_TYPE_NOT_FOUND -> stringResource(
            R.string.error_reason_contact_link_field_type_not_found
        )

        ErrorReasonCode.CONTACT_LINK_FIELD_TYPE_ALREADY_EXISTS -> stringResource(
            R.string.error_reason_contact_link_field_type_already_exists
        )

        ErrorReasonCode.COMMON_MEMBER_NOT_FOUND -> stringResource(
            R.string.error_reason_common_member_not_found
        )

        ErrorReasonCode.NEXTCLOUD_FILE_OR_FOLDER_NOT_FOUND -> stringResource(
            R.string.error_reason_nextcloud_file_or_folder_not_found
        )

        ErrorReasonCode.NEXTCLOUD_LINK_NOT_FOUND -> stringResource(
            R.string.error_reason_nextcloud_link_not_found
        )

        ErrorReasonCode.NEXTCLOUD_LINK_ALREADY_EXISTS -> stringResource(
            R.string.error_reason_nextcloud_link_already_exists
        )

        ErrorReasonCode.CONTACT_NOTE_NOT_FOUND -> stringResource(
            R.string.error_reason_contact_note_not_found
        )

        ErrorReasonCode.OCCASION_EVENT_NOT_FOUND -> stringResource(
            R.string.error_reason_occasion_event_not_found
        )

        ErrorReasonCode.OCCASION_OBLIGATION_NOT_FOUND -> stringResource(
            R.string.error_reason_occasion_obligation_not_found
        )

        ErrorReasonCode.PAPERLESS_DOCUMENT_NOT_FOUND -> stringResource(
            R.string.error_reason_paperless_document_not_found
        )

        ErrorReasonCode.PAPERLESS_LINK_NOT_FOUND -> stringResource(
            R.string.error_reason_paperless_link_not_found
        )

        ErrorReasonCode.PAPERLESS_LINK_ALREADY_EXISTS -> stringResource(
            R.string.error_reason_paperless_link_already_exists
        )

        ErrorReasonCode.AUTH_PASSKEY_NOT_FOUND -> stringResource(
            R.string.error_reason_auth_passkey_not_found
        )

        ErrorReasonCode.AUTH_PASSKEY_ALREADY_EXISTS -> stringResource(
            R.string.error_reason_auth_passkey_already_exists
        )

        ErrorReasonCode.PHOTO_NOT_FOUND -> stringResource(R.string.error_reason_photo_not_found)

        ErrorReasonCode.ACCOUNT_PREFERENCE_NOT_FOUND -> stringResource(
            R.string.error_reason_account_preference_not_found
        )

        ErrorReasonCode.NOTIFICATION_PUSH_SUBSCRIPTION_NOT_FOUND -> stringResource(
            R.string.error_reason_notification_push_subscription_not_found
        )

        ErrorReasonCode.CONTACT_REACH_OUT_SUGGESTION_NOT_FOUND -> stringResource(
            R.string.error_reason_contact_reach_out_suggestion_not_found
        )

        ErrorReasonCode.ACCOUNT_RECIPIENT_USER_NOT_FOUND -> stringResource(
            R.string.error_reason_account_recipient_user_not_found
        )

        ErrorReasonCode.RELATIONSHIP_EDGE_NOT_FOUND -> stringResource(
            R.string.error_reason_relationship_edge_not_found
        )

        ErrorReasonCode.RELATIONSHIP_EDGE_ALREADY_EXISTS -> stringResource(
            R.string.error_reason_relationship_edge_already_exists
        )

        ErrorReasonCode.CONTACT_REMINDER_NOT_FOUND -> stringResource(
            R.string.error_reason_contact_reminder_not_found
        )

        ErrorReasonCode.CONTACT_REMINDER_COMPLETION_NOT_FOUND -> stringResource(
            R.string.error_reason_contact_reminder_completion_not_found
        )

        ErrorReasonCode.SEAFILE_LIBRARY_OR_FILE_NOT_FOUND -> stringResource(
            R.string.error_reason_seafile_library_or_file_not_found
        )

        ErrorReasonCode.SEAFILE_LINK_NOT_FOUND -> stringResource(
            R.string.error_reason_seafile_link_not_found
        )

        ErrorReasonCode.SEAFILE_LINK_ALREADY_EXISTS -> stringResource(
            R.string.error_reason_seafile_link_already_exists
        )

        ErrorReasonCode.AUTH_SESSION_NOT_FOUND -> stringResource(
            R.string.error_reason_auth_session_not_found
        )

        ErrorReasonCode.COMMON_SYNC_CONFLICT_NOT_FOUND -> stringResource(
            R.string.error_reason_common_sync_conflict_not_found
        )

        ErrorReasonCode.CONTACT_TAG_NOT_FOUND -> stringResource(
            R.string.error_reason_contact_tag_not_found
        )

        ErrorReasonCode.CONTACT_TAGGING_NOT_FOUND -> stringResource(
            R.string.error_reason_contact_tagging_not_found
        )

        ErrorReasonCode.CONTACT_TAGGING_ALREADY_EXISTS -> stringResource(
            R.string.error_reason_contact_tagging_already_exists
        )

        ErrorReasonCode.PHOTO_THUMBNAIL_NOT_FOUND -> stringResource(
            R.string.error_reason_photo_thumbnail_not_found
        )

        ErrorReasonCode.ACCOUNT_USER_NOT_FOUND -> stringResource(
            R.string.error_reason_account_user_not_found
        )

        ErrorReasonCode.ACCOUNT_USER_ALREADY_EXISTS -> stringResource(
            R.string.error_reason_account_user_already_exists
        )

        ErrorReasonCode.INTEGRATION_WEBHOOK_NOT_FOUND -> stringResource(
            R.string.error_reason_integration_webhook_not_found
        )

        ErrorReasonCode.CONTACT_SOME_NOT_FOUND -> stringResource(
            R.string.error_reason_contact_some_not_found
        )

        ErrorReasonCode.AUTH_ACCOUNT_LOCKED ->
            // Seconds as a string (the template says "{n} s"): a %d followed by a unit word
            // trips the PluralsCandidate lint, and an SI symbol needs no plural.
            reason.int("retry_after")?.let {
                stringResource(R.string.error_reason_auth_account_locked, it.toString())
            }

        ErrorReasonCode.AUTH_PASSWORD_BREACHED -> stringResource(
            R.string.error_reason_auth_password_breached
        )

        ErrorReasonCode.AUTH_PASSWORD_TOO_LONG -> stringResource(
            R.string.error_reason_auth_password_too_long
        )

        ErrorReasonCode.AUTH_PASSWORD_CHANGE_DISABLED_DEMO -> stringResource(
            R.string.error_reason_auth_password_change_disabled_demo
        )

        ErrorReasonCode.AUTH_RESET_TOKEN_INVALID -> stringResource(
            R.string.error_reason_auth_reset_token_invalid
        )

        ErrorReasonCode.AUTH_TWO_FACTOR_ALREADY_ENABLED -> stringResource(
            R.string.error_reason_auth_two_factor_already_enabled
        )

        ErrorReasonCode.AUTH_TWO_FACTOR_UNAVAILABLE_OIDC -> stringResource(
            R.string.error_reason_auth_two_factor_unavailable_oidc
        )

        ErrorReasonCode.AUTH_TWO_FACTOR_SETUP_REQUIRED -> stringResource(
            R.string.error_reason_auth_two_factor_setup_required
        )

        ErrorReasonCode.AUTH_TWO_FACTOR_CODE_INVALID -> stringResource(
            R.string.error_reason_auth_two_factor_code_invalid
        )

        ErrorReasonCode.AUTH_TWO_FACTOR_NOT_ENABLED -> stringResource(
            R.string.error_reason_auth_two_factor_not_enabled
        )

        ErrorReasonCode.AUTH_TWO_FACTOR_LOGIN_NOT_PENDING -> stringResource(
            R.string.error_reason_auth_two_factor_login_not_pending
        )

        ErrorReasonCode.AUTH_TWO_FACTOR_SESSION_INVALID -> stringResource(
            R.string.error_reason_auth_two_factor_session_invalid
        )

        ErrorReasonCode.AUTH_TWO_FACTOR_NO_LONGER_ENABLED -> stringResource(
            R.string.error_reason_auth_two_factor_no_longer_enabled
        )

        ErrorReasonCode.AUTH_PASSKEY_NAME_TOO_LONG -> stringResource(
            R.string.error_reason_auth_passkey_name_too_long
        )

        ErrorReasonCode.AUTH_PASSKEY_NAME_LENGTH_INVALID -> stringResource(
            R.string.error_reason_auth_passkey_name_length_invalid
        )

        ErrorReasonCode.AUTH_PASSKEY_REGISTRATION_NOT_STARTED -> stringResource(
            R.string.error_reason_auth_passkey_registration_not_started
        )

        ErrorReasonCode.AUTH_PASSKEY_REGISTRATION_FAILED -> stringResource(
            R.string.error_reason_auth_passkey_registration_failed
        )

        ErrorReasonCode.AUTH_PASSKEY_NONE_REGISTERED -> stringResource(
            R.string.error_reason_auth_passkey_none_registered
        )

        ErrorReasonCode.AUTH_PASSKEY_NONE_REGISTERED_SIGN_IN_AGAIN -> stringResource(
            R.string.error_reason_auth_passkey_none_registered_sign_in_again
        )

        ErrorReasonCode.AUTH_PASSKEY_LOGIN_NOT_STARTED -> stringResource(
            R.string.error_reason_auth_passkey_login_not_started
        )

        ErrorReasonCode.AUTH_PASSKEY_VERIFICATION_FAILED -> stringResource(
            R.string.error_reason_auth_passkey_verification_failed
        )

        ErrorReasonCode.AUTH_PASSKEY_NO_OTHER_REGISTERED -> stringResource(
            R.string.error_reason_auth_passkey_no_other_registered
        )

        ErrorReasonCode.VALIDATION_INVALID_JSON -> stringResource(
            R.string.error_reason_validation_invalid_json
        )

        ErrorReasonCode.VALIDATION_INVALID_REQUEST_BODY -> stringResource(
            R.string.error_reason_validation_invalid_request_body
        )

        ErrorReasonCode.IMPORT_SESSION_NOT_FOUND ->
            stringResource(R.string.error_reason_import_session_not_found)

        ErrorReasonCode.IMPORT_SESSION_EXPIRED ->
            stringResource(R.string.error_reason_import_session_expired)

        ErrorReasonCode.IMPORT_ENDPOINT_CSV_VCF_ONLY ->
            stringResource(R.string.error_reason_import_endpoint_csv_vcf_only)

        ErrorReasonCode.IMPORT_ENDPOINT_VCF_RECORDS_ONLY ->
            stringResource(R.string.error_reason_import_endpoint_vcf_records_only)

        ErrorReasonCode.IMPORT_PREVIEW_REQUIRED ->
            stringResource(R.string.error_reason_import_preview_required)

        ErrorReasonCode.IMPORT_FILE_MISSING ->
            stringResource(R.string.error_reason_import_file_missing)

        ErrorReasonCode.IMPORT_JSCONTACT_FILE_TOO_LARGE -> {
            val maxMb = reason.int("max_mb")
            if (maxMb != null) {
                stringResource(R.string.error_reason_import_jscontact_file_too_large, maxMb)
            } else {
                null
            }
        }

        ErrorReasonCode.IMPORT_FILE_NOT_JSCONTACT ->
            stringResource(R.string.error_reason_import_file_not_jscontact)

        ErrorReasonCode.CSV_FILE_TOO_LARGE -> {
            val maxMb = reason.int("max_mb")
            if (maxMb != null) {
                stringResource(R.string.error_reason_csv_file_too_large, maxMb)
            } else {
                null
            }
        }

        ErrorReasonCode.CSV_FILE_NOT_CSV ->
            stringResource(R.string.error_reason_csv_file_not_csv)

        ErrorReasonCode.VCARD_FILE_TOO_LARGE -> {
            val maxMb = reason.int("max_mb")
            if (maxMb != null) {
                stringResource(R.string.error_reason_vcard_file_too_large, maxMb)
            } else {
                null
            }
        }

        ErrorReasonCode.VCARD_FILE_NOT_VCF ->
            stringResource(R.string.error_reason_vcard_file_not_vcf)

        ErrorReasonCode.MONICA_SESSION_NOT_FOUND ->
            stringResource(R.string.error_reason_monica_session_not_found)

        ErrorReasonCode.MONICA_SESSION_EXPIRED ->
            stringResource(R.string.error_reason_monica_session_expired)

        ErrorReasonCode.MONICA_TOKEN_REJECTED ->
            stringResource(R.string.error_reason_monica_token_rejected)

        ErrorReasonCode.MONICA_URL_INVALID ->
            stringResource(R.string.error_reason_monica_url_invalid)

        ErrorReasonCode.MONICA_URL_PRIVATE ->
            stringResource(R.string.error_reason_monica_url_private)

        ErrorReasonCode.MONICA_NOT_MONICA_API ->
            stringResource(R.string.error_reason_monica_not_monica_api)

        ErrorReasonCode.MONICA_UNREACHABLE ->
            stringResource(R.string.error_reason_monica_unreachable)

        ErrorReasonCode.MONICA_TOO_MANY_CONTACTS -> {
            val total = reason.int("total")
            val max = reason.int("max")
            if (total != null && max != null) {
                stringResource(
                    R.string.error_reason_monica_too_many_contacts,
                    total.toString(),
                    max
                )
            } else {
                null
            }
        }

        ErrorReasonCode.MONICA_TOO_MANY_RECORDS -> {
            val total = reason.int("total")
            val max = reason.int("max")
            if (total != null && max != null) {
                stringResource(
                    R.string.error_reason_monica_too_many_records,
                    total.toString(),
                    max
                )
            } else {
                null
            }
        }

        ErrorReasonCode.MONICA_FETCH_IN_PROGRESS ->
            stringResource(R.string.error_reason_monica_fetch_in_progress)

        ErrorReasonCode.MONICA_NOT_FETCHED ->
            stringResource(R.string.error_reason_monica_not_fetched)

        ErrorReasonCode.BUNDLE_SESSION_NOT_FOUND ->
            stringResource(R.string.error_reason_bundle_session_not_found)

        ErrorReasonCode.BUNDLE_SESSION_EXPIRED ->
            stringResource(R.string.error_reason_bundle_session_expired)

        ErrorReasonCode.BUNDLE_UPLOAD_MISSING ->
            stringResource(R.string.error_reason_bundle_upload_missing)

        ErrorReasonCode.BUNDLE_SIZE ->
            stringResource(R.string.error_reason_bundle_size)

        ErrorReasonCode.BUNDLE_UPLOAD_UNREADABLE ->
            stringResource(R.string.error_reason_bundle_upload_unreadable)

        ErrorReasonCode.BUNDLE_NOT_BUNDLE ->
            stringResource(R.string.error_reason_bundle_not_bundle)

        ErrorReasonCode.BUNDLE_TOO_MANY_CONTACTS ->
            stringResource(R.string.error_reason_bundle_too_many_contacts)

        ErrorReasonCode.BUNDLE_TOO_MANY_RECORDS ->
            stringResource(R.string.error_reason_bundle_too_many_records)

        ErrorReasonCode.BUNDLE_IMPORT_IN_PROGRESS ->
            stringResource(R.string.error_reason_bundle_import_in_progress)

        ErrorReasonCode.BUNDLE_NOT_PREPARED ->
            stringResource(R.string.error_reason_bundle_not_prepared)

        ErrorReasonCode.AUTH_REQUIRED -> stringResource(
            R.string.error_reason_auth_required
        )

        ErrorReasonCode.AUTH_REGISTRATION_DISABLED -> stringResource(
            R.string.error_reason_auth_registration_disabled
        )

        ErrorReasonCode.AUTH_USER_ID_INVALID -> stringResource(
            R.string.error_reason_auth_user_id_invalid
        )

        ErrorReasonCode.AUTH_USER_NOT_FOUND -> stringResource(
            R.string.error_reason_auth_user_not_found
        )

        ErrorReasonCode.AUTH_AUTHORIZATION_TOKEN_REQUIRED -> stringResource(
            R.string.error_reason_auth_authorization_token_required
        )

        ErrorReasonCode.AUTH_AUTHORIZATION_HEADER_NOT_BEARER -> stringResource(
            R.string.error_reason_auth_authorization_header_not_bearer
        )

        ErrorReasonCode.AUTH_BEARER_TOKEN_INVALID -> stringResource(
            R.string.error_reason_auth_bearer_token_invalid
        )

        ErrorReasonCode.AUTH_BEARER_TOKEN_EXPIRED -> stringResource(
            R.string.error_reason_auth_bearer_token_expired
        )

        ErrorReasonCode.AUTH_BEARER_TOKEN_MALFORMED -> stringResource(
            R.string.error_reason_auth_bearer_token_malformed
        )

        ErrorReasonCode.AUTH_BEARER_TOKEN_SIGNATURE_INVALID -> stringResource(
            R.string.error_reason_auth_bearer_token_signature_invalid
        )

        ErrorReasonCode.AUTH_SESSION_EXPIRED -> stringResource(
            R.string.error_reason_auth_session_expired
        )

        ErrorReasonCode.AUTH_TOKEN_SCOPED_CARDDAV -> stringResource(
            R.string.error_reason_auth_token_scoped_carddav
        )

        ErrorReasonCode.AUTH_TOKEN_SCOPED_CALDAV -> stringResource(
            R.string.error_reason_auth_token_scoped_caldav
        )

        ErrorReasonCode.ADMIN_API_TOKEN_FORBIDDEN -> stringResource(
            R.string.error_reason_admin_api_token_forbidden
        )

        ErrorReasonCode.ADMIN_ACCESS_REQUIRED -> stringResource(
            R.string.error_reason_admin_access_required
        )

        ErrorReasonCode.ADMIN_USER_ID_INVALID -> stringResource(
            R.string.error_reason_admin_user_id_invalid
        )

        ErrorReasonCode.ADMIN_ROLE_CHANGE_OTHER_ADMIN_FORBIDDEN -> stringResource(
            R.string.error_reason_admin_role_change_other_admin_forbidden
        )

        ErrorReasonCode.ADMIN_LAST_ADMIN_REMOVAL_FORBIDDEN -> stringResource(
            R.string.error_reason_admin_last_admin_removal_forbidden
        )

        ErrorReasonCode.ADMIN_DELETE_SELF_FORBIDDEN -> stringResource(
            R.string.error_reason_admin_delete_self_forbidden
        )

        ErrorReasonCode.ADMIN_DELETE_OTHER_ADMIN_FORBIDDEN -> stringResource(
            R.string.error_reason_admin_delete_other_admin_forbidden
        )

        ErrorReasonCode.ACCOUNT_LANGUAGE_INVALID -> stringResource(
            R.string.error_reason_account_language_invalid
        )

        ErrorReasonCode.ACCOUNT_LANGUAGE_UNSUPPORTED ->
            reason.str("supported")?.let {
                stringResource(R.string.error_reason_account_language_unsupported, it)
            }

        ErrorReasonCode.ACCOUNT_DATE_FORMAT_INVALID -> stringResource(
            R.string.error_reason_account_date_format_invalid
        )

        ErrorReasonCode.ACCOUNT_DATE_FORMAT_UNSUPPORTED -> stringResource(
            R.string.error_reason_account_date_format_unsupported
        )

        ErrorReasonCode.ACCOUNT_TIME_FORMAT_INVALID -> stringResource(
            R.string.error_reason_account_time_format_invalid
        )

        ErrorReasonCode.ACCOUNT_TIME_FORMAT_UNSUPPORTED -> stringResource(
            R.string.error_reason_account_time_format_unsupported
        )

        ErrorReasonCode.ACCOUNT_DAV_PASSWORD_REQUIRES_NO_TWO_FACTOR -> stringResource(
            R.string.error_reason_account_dav_password_requires_no_two_factor
        )

        ErrorReasonCode.ACCOUNT_CURRENT_PASSWORD_INCORRECT -> stringResource(
            R.string.error_reason_account_current_password_incorrect
        )

        ErrorReasonCode.ACCOUNT_NEW_PASSWORD_SAME_AS_CURRENT -> stringResource(
            R.string.error_reason_account_new_password_same_as_current
        )

        ErrorReasonCode.ACCOUNT_DELETION_DISABLED_DEMO -> stringResource(
            R.string.error_reason_account_deletion_disabled_demo
        )

        ErrorReasonCode.ACCOUNT_SOLE_ADMIN_PROMOTION_REQUIRED -> stringResource(
            R.string.error_reason_account_sole_admin_promotion_required
        )

        ErrorReasonCode.ACCOUNT_API_TOKEN_SCOPES_REQUIRED -> stringResource(
            R.string.error_reason_account_api_token_scopes_required
        )

        ErrorReasonCode.ACCOUNT_API_TOKEN_SCOPE_UNKNOWN ->
            reason.str("scope")?.let {
                stringResource(R.string.error_reason_account_api_token_scope_unknown, it)
            }

        ErrorReasonCode.ACCOUNT_API_TOKEN_EXPIRY_CONFLICT -> stringResource(
            R.string.error_reason_account_api_token_expiry_conflict
        )

        ErrorReasonCode.ACCOUNT_API_TOKEN_NEVER_EXPIRES_NOT_ALLOWED -> stringResource(
            R.string.error_reason_account_api_token_never_expires_not_allowed
        )

        ErrorReasonCode.ACCOUNT_API_TOKEN_ALREADY_REVOKED -> stringResource(
            R.string.error_reason_account_api_token_already_revoked
        )

        ErrorReasonCode.VALIDATION_ID_NOT_POSITIVE_INTEGER -> stringResource(
            R.string.error_reason_validation_id_not_positive_integer
        )

        ErrorReasonCode.CONTACT_SHARE_WITH_SELF -> stringResource(
            R.string.error_reason_contact_share_with_self
        )

        ErrorReasonCode.CONTACT_SHARE_SECTION_UNKNOWN ->
            reason.str("section")?.let {
                stringResource(R.string.error_reason_contact_share_section_unknown, it)
            }

        ErrorReasonCode.CONTACT_SHARE_ALREADY_RESPONDED -> stringResource(
            R.string.error_reason_contact_share_already_responded
        )

        ErrorReasonCode.CONTACT_SHARE_SESSION_MISMATCH -> stringResource(
            R.string.error_reason_contact_share_session_mismatch
        )

        ErrorReasonCode.IMMICH_API_KEY_INVALID -> stringResource(
            R.string.error_reason_immich_api_key_invalid
        )

        ErrorReasonCode.IMMICH_BASE_URL_INVALID -> stringResource(
            R.string.error_reason_immich_base_url_invalid
        )

        ErrorReasonCode.IMMICH_REDIRECT -> stringResource(
            R.string.error_reason_immich_redirect
        )

        ErrorReasonCode.IMMICH_UPSTREAM_ERROR ->
            reason.str(
                "status"
            )?.let {
                stringResource(R.string.error_reason_immich_upstream_error, it)
            }

        ErrorReasonCode.IMMICH_RESPONSE_UNPARSEABLE -> stringResource(
            R.string.error_reason_immich_response_unparseable
        )

        ErrorReasonCode.IMMICH_UNREACHABLE -> stringResource(
            R.string.error_reason_immich_unreachable
        )

        ErrorReasonCode.IMMICH_API_KEY_REQUIRED -> stringResource(
            R.string.error_reason_immich_api_key_required
        )

        ErrorReasonCode.SEAFILE_API_TOKEN_INVALID -> stringResource(
            R.string.error_reason_seafile_api_token_invalid
        )

        ErrorReasonCode.SEAFILE_BASE_URL_INVALID -> stringResource(
            R.string.error_reason_seafile_base_url_invalid
        )

        ErrorReasonCode.SEAFILE_REDIRECT -> stringResource(
            R.string.error_reason_seafile_redirect
        )

        ErrorReasonCode.SEAFILE_UPSTREAM_ERROR ->
            reason.str(
                "status"
            )?.let {
                stringResource(R.string.error_reason_seafile_upstream_error, it)
            }

        ErrorReasonCode.SEAFILE_RESPONSE_UNPARSEABLE -> stringResource(
            R.string.error_reason_seafile_response_unparseable
        )

        ErrorReasonCode.SEAFILE_UNREACHABLE -> stringResource(
            R.string.error_reason_seafile_unreachable
        )

        ErrorReasonCode.SEAFILE_API_TOKEN_REQUIRED -> stringResource(
            R.string.error_reason_seafile_api_token_required
        )

        ErrorReasonCode.PAPERLESS_API_TOKEN_INVALID -> stringResource(
            R.string.error_reason_paperless_api_token_invalid
        )

        ErrorReasonCode.PAPERLESS_BASE_URL_INVALID -> stringResource(
            R.string.error_reason_paperless_base_url_invalid
        )

        ErrorReasonCode.PAPERLESS_REDIRECT -> stringResource(
            R.string.error_reason_paperless_redirect
        )

        ErrorReasonCode.PAPERLESS_UPSTREAM_ERROR ->
            reason.str(
                "status"
            )?.let {
                stringResource(R.string.error_reason_paperless_upstream_error, it)
            }

        ErrorReasonCode.PAPERLESS_RESPONSE_UNPARSEABLE -> stringResource(
            R.string.error_reason_paperless_response_unparseable
        )

        ErrorReasonCode.PAPERLESS_UNREACHABLE -> stringResource(
            R.string.error_reason_paperless_unreachable
        )

        ErrorReasonCode.PAPERLESS_API_TOKEN_REQUIRED -> stringResource(
            R.string.error_reason_paperless_api_token_required
        )

        ErrorReasonCode.GEOPULSE_API_TOKEN_INVALID -> stringResource(
            R.string.error_reason_geopulse_api_token_invalid
        )

        ErrorReasonCode.GEOPULSE_BASE_URL_INVALID -> stringResource(
            R.string.error_reason_geopulse_base_url_invalid
        )

        ErrorReasonCode.GEOPULSE_REDIRECT -> stringResource(
            R.string.error_reason_geopulse_redirect
        )

        ErrorReasonCode.GEOPULSE_UPSTREAM_ERROR ->
            reason.str(
                "status"
            )?.let {
                stringResource(R.string.error_reason_geopulse_upstream_error, it)
            }

        ErrorReasonCode.GEOPULSE_RESPONSE_UNPARSEABLE -> stringResource(
            R.string.error_reason_geopulse_response_unparseable
        )

        ErrorReasonCode.GEOPULSE_UNREACHABLE -> stringResource(
            R.string.error_reason_geopulse_unreachable
        )

        ErrorReasonCode.GEOPULSE_API_TOKEN_REQUIRED -> stringResource(
            R.string.error_reason_geopulse_api_token_required
        )

        ErrorReasonCode.WEBDAV_APP_PASSWORD_INVALID -> stringResource(
            R.string.error_reason_webdav_app_password_invalid
        )

        ErrorReasonCode.WEBDAV_BASE_URL_INVALID -> stringResource(
            R.string.error_reason_webdav_base_url_invalid
        )

        ErrorReasonCode.WEBDAV_REDIRECT -> stringResource(
            R.string.error_reason_webdav_redirect
        )

        ErrorReasonCode.WEBDAV_UPSTREAM_ERROR ->
            reason.str(
                "status"
            )?.let {
                stringResource(R.string.error_reason_webdav_upstream_error, it)
            }

        ErrorReasonCode.WEBDAV_RESPONSE_UNPARSEABLE -> stringResource(
            R.string.error_reason_webdav_response_unparseable
        )

        ErrorReasonCode.WEBDAV_UNREACHABLE -> stringResource(
            R.string.error_reason_webdav_unreachable
        )

        ErrorReasonCode.WEBDAV_APP_PASSWORD_REQUIRED -> stringResource(
            R.string.error_reason_webdav_app_password_required
        )

        ErrorReasonCode.IMMICH_PERSON_ID_REQUIRED -> stringResource(
            R.string.error_reason_immich_person_id_required
        )

        ErrorReasonCode.PAPERLESS_DOCUMENT_ID_REQUIRED -> stringResource(
            R.string.error_reason_paperless_document_id_required
        )

        ErrorReasonCode.WEBDAV_PATH_REQUIRED -> stringResource(
            R.string.error_reason_webdav_path_required
        )

        ErrorReasonCode.SEAFILE_REPO_AND_PATH_REQUIRED -> stringResource(
            R.string.error_reason_seafile_repo_and_path_required
        )

        ErrorReasonCode.GEOPULSE_DATE_REQUIRED -> stringResource(
            R.string.error_reason_geopulse_date_required
        )

        ErrorReasonCode.GEOPULSE_DATE_INVALID -> stringResource(
            R.string.error_reason_geopulse_date_invalid
        )

        ErrorReasonCode.GEOPULSE_TOKEN_UNDECRYPTABLE -> stringResource(
            R.string.error_reason_geopulse_token_undecryptable
        )

        ErrorReasonCode.IMMICH_SVG_UNSUPPORTED -> stringResource(
            R.string.error_reason_immich_svg_unsupported
        )

        ErrorReasonCode.INTEGRATION_REQUEST_INVALID -> stringResource(
            R.string.error_reason_integration_request_invalid
        )

        ErrorReasonCode.INTEGRATION_ID_INVALID -> stringResource(
            R.string.error_reason_integration_id_invalid
        )

        ErrorReasonCode.INTEGRATION_PASSWORD_REENTRY_REQUIRED -> stringResource(
            R.string.error_reason_integration_password_reentry_required
        )

        ErrorReasonCode.INTEGRATION_WEBHOOK_LIMIT -> stringResource(
            R.string.error_reason_integration_webhook_limit
        )

        ErrorReasonCode.CALENDAR_SUBSCRIPTION_LIMIT -> stringResource(
            R.string.error_reason_calendar_subscription_limit
        )

        ErrorReasonCode.CALENDAR_URL_SCHEME_INVALID -> stringResource(
            R.string.error_reason_calendar_url_scheme_invalid
        )

        ErrorReasonCode.CALENDAR_URL_INVALID -> stringResource(
            R.string.error_reason_calendar_url_invalid
        )

        ErrorReasonCode.CALENDAR_AUTH_FAILED -> stringResource(
            R.string.error_reason_calendar_auth_failed
        )

        ErrorReasonCode.CALENDAR_NOT_FOUND_AT_URL -> stringResource(
            R.string.error_reason_calendar_not_found_at_url
        )

        ErrorReasonCode.CALENDAR_PRIVATE_ADDRESS -> stringResource(
            R.string.error_reason_calendar_private_address
        )

        ErrorReasonCode.CALENDAR_RESPONSE_TOO_LARGE -> stringResource(
            R.string.error_reason_calendar_response_too_large
        )

        ErrorReasonCode.CALENDAR_DATA_UNPARSEABLE -> stringResource(
            R.string.error_reason_calendar_data_unparseable
        )

        ErrorReasonCode.CALENDAR_UNREACHABLE -> stringResource(
            R.string.error_reason_calendar_unreachable
        )

        ErrorReasonCode.CONTACT_SUBSCRIPTION_LIMIT -> stringResource(
            R.string.error_reason_contact_subscription_limit
        )

        ErrorReasonCode.CONTACT_SUBSCRIPTION_URL_SCHEME_INVALID -> stringResource(
            R.string.error_reason_contact_subscription_url_scheme_invalid
        )

        ErrorReasonCode.CONTACT_SUBSCRIPTION_URL_INVALID -> stringResource(
            R.string.error_reason_contact_subscription_url_invalid
        )

        ErrorReasonCode.CONTACT_SUBSCRIPTION_AUTH_FAILED -> stringResource(
            R.string.error_reason_contact_subscription_auth_failed
        )

        ErrorReasonCode.CONTACT_SUBSCRIPTION_NOT_FOUND_AT_URL -> stringResource(
            R.string.error_reason_contact_subscription_not_found_at_url
        )

        ErrorReasonCode.CONTACT_SUBSCRIPTION_PRIVATE_ADDRESS -> stringResource(
            R.string.error_reason_contact_subscription_private_address
        )

        ErrorReasonCode.CONTACT_SUBSCRIPTION_RESPONSE_TOO_LARGE -> stringResource(
            R.string.error_reason_contact_subscription_response_too_large
        )

        ErrorReasonCode.CONTACT_SUBSCRIPTION_DATA_UNPARSEABLE -> stringResource(
            R.string.error_reason_contact_subscription_data_unparseable
        )

        ErrorReasonCode.CONTACT_SUBSCRIPTION_UNREACHABLE -> stringResource(
            R.string.error_reason_contact_subscription_unreachable
        )

        ErrorReasonCode.NOTIFICATION_URL_INVALID -> stringResource(
            R.string.error_reason_notification_url_invalid
        )

        ErrorReasonCode.NOTIFICATION_PUSH_SUBSCRIPTION_LIMIT ->
            // Passed as a string: "%d <noun>s" trips the PluralsCandidate lint.
            reason.int(
                "max"
            )?.let {
                stringResource(
                    R.string.error_reason_notification_push_subscription_limit,
                    it.toString()
                )
            }

        ErrorReasonCode.NOTIFICATION_DEVICE_REGISTRATION_LIMIT ->
            // Passed as a string: "%d <noun>s" trips the PluralsCandidate lint.
            reason.int(
                "max"
            )?.let {
                stringResource(
                    R.string.error_reason_notification_device_registration_limit,
                    it.toString()
                )
            }

        ErrorReasonCode.FEED_LIMIT_REACHED ->
            // Passed as a string: "%d <noun>s" trips the PluralsCandidate lint.
            reason.int(
                "max"
            )?.let {
                stringResource(R.string.error_reason_feed_limit_reached, it.toString())
            }

        ErrorReasonCode.FEED_ENTITY_ID_NOT_ALLOWED -> stringResource(
            R.string.error_reason_feed_entity_id_not_allowed
        )

        else -> null
    }
}

/** Mirrors backend errors.ErrCodeValidation. */
private const val VALIDATION_ERROR = "VALIDATION_ERROR"

// Moshi decodes JSON numbers as Double, so an integral param is read through
// Number; a string that holds a number is accepted too.
private fun ErrorReason.str(name: String): String? = params[name]?.toString()

private fun ErrorReason.int(name: String): Int? = when (val value = params[name]) {
    is Number -> value.toInt()
    is String -> value.toIntOrNull()
    else -> null
}
