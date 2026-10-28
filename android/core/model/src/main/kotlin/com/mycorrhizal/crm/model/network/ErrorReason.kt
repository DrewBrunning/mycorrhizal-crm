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
}
