package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

// --- Issue #866 active-session inventory (server half); issue #957 is the
// first Android consumer, revoking this device's own session on logout.

/**
 * One active interactive login, as returned by `GET /api/v1/sessions`. [id]
 * is the opaque value also carried in the JWT's `sid` claim and is the
 * target for `DELETE /api/v1/sessions/{id}`. [current] marks the session
 * making the request — the client never decodes its own JWT locally to find
 * that out, it just looks for the row with this flag set.
 */
@JsonClass(generateAdapter = true)
data class SessionInfo(
    val id: String = "",
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "last_seen_at") val lastSeenAt: String? = null,
    @Json(name = "expires_at") val expiresAt: String? = null,
    @Json(name = "user_agent") val userAgent: String = "",
    val ip: String = "",
    val current: Boolean = false,
)

/** GET /api/v1/sessions — `{ sessions: [...] }`. */
@JsonClass(generateAdapter = true)
data class SessionsResponse(
    val sessions: List<SessionInfo> = emptyList(),
)
