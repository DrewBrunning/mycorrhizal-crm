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

        else -> null
    }
}
