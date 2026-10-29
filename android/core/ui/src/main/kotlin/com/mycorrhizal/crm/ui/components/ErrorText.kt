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
    val base = localizedReason(reasonOrNull()) ?: clientMessage()
    val requestId = (this as? ApiError.Server)?.requestId
    return if (requestId != null) {
        "$base ${stringResource(R.string.error_request_ref, requestId)}"
    } else {
        base
    }
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
