package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * M25 settings-surface DTOs: user profile prefs (language/date-format/
 * password), webhooks, and ntfy/Gotify notification channels. Mirrors
 * `backend/models/dtos.go` and `backend/models/notification.go` by hand —
 * there is no dynamic type-list endpoint anywhere in this codebase
 * (`/CLAUDE.md` frontend trap #4), so the channel enum and webhook event
 * list below are hand-mirrored and must be kept in sync if the backend ever
 * changes them.
 */

/** PATCH /users/language request body — `{ language: "en" }`. */
@JsonClass(generateAdapter = true)
data class UpdateLanguageRequest(
    val language: String,
)

/** PATCH /users/date-format request body — `{ date_format: "eu" }`. */
@JsonClass(generateAdapter = true)
data class UpdateDateFormatRequest(
    @Json(name = "date_format") val dateFormat: String,
)

/** POST /users/change-password request body. */
@JsonClass(generateAdapter = true)
data class ChangePasswordRequest(
    @Json(name = "current_password") val currentPassword: String,
    @Json(name = "new_password") val newPassword: String,
)

/** Generic `{ message: "…" }` envelope used by the PATCH/POST user endpoints. */
@JsonClass(generateAdapter = true)
data class MessageResponse(
    val message: String? = null,
)

/** Notification-channel test endpoints accept `ntfy | gotify | push` — never `email`. */
val ALL_TEST_CHANNELS: List<String> = listOf("ntfy", "gotify", "push")

/**
 * GET/PUT /notifications/config response — the full per-user notification
 * setup. [gotifyToken] is write-only: the API returns only [gotifyHasToken],
 * never the token itself.
 */
@JsonClass(generateAdapter = true)
data class NotificationConfig(
    @Json(name = "ntfy_url") val ntfyUrl: String = "",
    @Json(name = "ntfy_topic") val ntfyTopic: String = "",
    @Json(name = "gotify_url") val gotifyUrl: String = "",
    @Json(name = "gotify_has_token") val gotifyHasToken: Boolean = false,
    @Json(name = "notify_ntfy") val notifyNtfy: Boolean = false,
    @Json(name = "notify_gotify") val notifyGotify: Boolean = false,
    @Json(name = "notify_push") val notifyPush: Boolean = false,
    @Json(name = "vapid_public_key") val vapidPublicKey: String = "",
)

/**
 * PUT /notifications/config request body — every field optional. Null values
 * are omitted from the JSON (Moshi's default), so an omitted field keeps its
 * stored value server-side; [gotifyToken] empty/null also keeps the stored
 * token. `notify_push` is deliberately absent from the Android surface (web
 * push is browser-specific — `/CLAUDE.md` M25 scope note); omitting it here
 * leaves it unchanged.
 */
@JsonClass(generateAdapter = true)
data class NotificationConfigInput(
    @Json(name = "ntfy_url") val ntfyUrl: String? = null,
    @Json(name = "ntfy_topic") val ntfyTopic: String? = null,
    @Json(name = "gotify_url") val gotifyUrl: String? = null,
    @Json(name = "gotify_token") val gotifyToken: String? = null,
    @Json(name = "notify_ntfy") val notifyNtfy: Boolean? = null,
    @Json(name = "notify_gotify") val notifyGotify: Boolean? = null,
)

/** POST /notifications/config/test request body. */
@JsonClass(generateAdapter = true)
data class NotificationTestChannelRequest(
    val channel: String,
)

/**
 * POST /notifications/config/test response. A diagnosed failure (unconfigured
 * channel, unreachable server, SSRF-blocked URL) is still an HTTP 200 with
 * `ok:false` — only transport/parse errors surface as Result.failure.
 */
@JsonClass(generateAdapter = true)
data class NotificationTestResult(
    val ok: Boolean = false,
    val error: String? = null,
)

/** A single webhook — the `GET/PUT /webhooks` response shape (never contains a secret). */
@JsonClass(generateAdapter = true)
data class Webhook(
    val id: Int = 0,
    val name: String = "",
    val url: String = "",
    val events: List<String> = emptyList(),
    @Json(name = "is_active") val isActive: Boolean = false,
    @Json(name = "created_at") val createdAt: String? = null,
)

/** POST /webhooks request/response body. [isActive] is a non-pointer bool server-side — always send it. */
@JsonClass(generateAdapter = true)
data class WebhookInput(
    val name: String,
    val url: String,
    val events: List<String>,
    @Json(name = "is_active") val isActive: Boolean,
)

/** POST /webhooks (201) response — the only response that carries the plaintext [secret]. */
@JsonClass(generateAdapter = true)
data class WebhookCreateResponse(
    val id: Int = 0,
    val name: String = "",
    val url: String = "",
    val events: List<String> = emptyList(),
    @Json(name = "is_active") val isActive: Boolean = false,
    @Json(name = "created_at") val createdAt: String? = null,
    val secret: String? = null,
)

/** A single webhook delivery record (test or event-triggered). */
@JsonClass(generateAdapter = true)
data class WebhookDelivery(
    val id: Int = 0,
    @Json(name = "webhook_id") val webhookId: Int = 0,
    @Json(name = "event_type") val eventType: String = "",
    @Json(name = "status_code") val statusCode: Int? = null,
    val error: String? = null,
    val attempts: Int = 0,
    @Json(name = "next_retry_at") val nextRetryAt: String? = null,
    @Json(name = "created_at") val createdAt: String? = null,
)

/** GET /webhooks — `{ webhooks: [...] }`. */
@JsonClass(generateAdapter = true)
data class WebhooksResponse(
    val webhooks: List<Webhook> = emptyList(),
)

/** GET /webhooks/:id/deliveries — `{ deliveries: [...] }`, most recent 50. */
@JsonClass(generateAdapter = true)
data class WebhookDeliveriesResponse(
    val deliveries: List<WebhookDelivery> = emptyList(),
)

/** POST /webhooks/:id/test — `{ delivery: {...} }`. */
@JsonClass(generateAdapter = true)
data class WebhookTestResponse(
    val delivery: WebhookDelivery? = null,
)

/**
 * API token lifecycle (issue #413's Android follow-up, #573). Mirrors web's
 * `frontend/src/api/apiTokens.ts` data model exactly. A token's plaintext is
 * only ever present on [ApiTokenCreateResponse] (create and rotate) — never
 * on the plain list/detail [ApiToken] shape, and never logged or persisted
 * beyond the one-shot reveal dialog.
 */
val API_TOKEN_SCOPES: List<String> = listOf("full", "carddav")

/** Selectable lifetimes; the backend caps this at 365 days ([MAX_API_TOKEN_EXPIRY_DAYS]). */
val API_TOKEN_EXPIRY_OPTIONS: List<Int> = listOf(30, 60, 90, 180, 365)

const val DEFAULT_API_TOKEN_EXPIRY_DAYS = 90
const val DEFAULT_API_TOKEN_SCOPE = "full"

/** GET/POST /api-tokens response for a token — never contains the hash or plaintext. */
@JsonClass(generateAdapter = true)
data class ApiToken(
    val id: Int = 0,
    val name: String = "",
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "last_used_at") val lastUsedAt: String? = null,
    @Json(name = "revoked_at") val revokedAt: String? = null,
    /** Null only for tokens created before expiry was introduced. */
    @Json(name = "expires_at") val expiresAt: String? = null,
    val scope: String = DEFAULT_API_TOKEN_SCOPE,
)

/** POST /api-tokens request body. [expiresInDays]/[scope] omitted apply the backend defaults. */
@JsonClass(generateAdapter = true)
data class ApiTokenInput(
    val name: String,
    @Json(name = "expires_in_days") val expiresInDays: Int? = null,
    val scope: String? = null,
)

/**
 * POST /api-tokens (201) and POST /api-tokens/:id/rotate (201) response — the
 * only responses that carry the plaintext [token], shown exactly once.
 */
@JsonClass(generateAdapter = true)
data class ApiTokenCreateResponse(
    val id: Int = 0,
    val name: String = "",
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "last_used_at") val lastUsedAt: String? = null,
    @Json(name = "revoked_at") val revokedAt: String? = null,
    @Json(name = "expires_at") val expiresAt: String? = null,
    val scope: String = DEFAULT_API_TOKEN_SCOPE,
    val token: String? = null,
)

/** GET /api-tokens — `{ tokens: [...] }`. */
@JsonClass(generateAdapter = true)
data class ApiTokensResponse(
    val tokens: List<ApiToken> = emptyList(),
)

/** POST /api-tokens/revoke-all — `{ revoked: N }`, the count of tokens actually revoked. */
@JsonClass(generateAdapter = true)
data class RevokeAllApiTokensResponse(
    val revoked: Int = 0,
)
