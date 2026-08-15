package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/** POST /api/v1/login request body. Either [identifier] or legacy [email]. */
@JsonClass(generateAdapter = true)
data class LoginRequest(
    val identifier: String? = null,
    val email: String? = null,
    val password: String? = null,
)

/** POST /api/v1/login success body (the auth_token JWT arrives as an httpOnly cookie). */
@JsonClass(generateAdapter = true)
data class LoginResponse(
    val language: String? = null,
    @Json(name = "date_format") val dateFormat: String? = null,
)

/**
 * GET /api/v1/users/me — the authenticated user. Mirrors the OpenAPI
 * AdminUserResponse + enabled_contact_fields (CurrentUserResponse allOf).
 */
@JsonClass(generateAdapter = true)
data class UserProfile(
    val id: Int = 0,
    val username: String? = null,
    val email: String? = null,
    val language: String? = null,
    @Json(name = "date_format") val dateFormat: String? = null,
    @Json(name = "is_admin") val isAdmin: Boolean = false,
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "updated_at") val updatedAt: String? = null,
    @Json(name = "enabled_contact_fields") val enabledContactFields: List<String>? = null,
    // T90: the "Me" contact pointer (users.self_contact_vcard_uid). Added for
    // M14's network screen, which defaults its "start from" picker to the
    // self contact when one is set; the backend backfills it lazily on every
    // /users/me, so this is reliably populated for pre-000018 accounts too.
    @Json(name = "self_contact_vcard_uid") val selfContactVCardUid: String? = null,
)

// --- M26: account creation + password reset (all public, rate-limited) ---

/** POST /register body — `language` optional (defaults server-side). */
@JsonClass(generateAdapter = true)
data class RegisterRequest(
    val username: String,
    val email: String,
    val password: String,
    val language: String? = null,
)

/** POST /check-password-strength body. */
@JsonClass(generateAdapter = true)
data class CheckPasswordStrengthRequest(
    val password: String,
)

/**
 * POST /check-password-strength response — a raw, unwrapped PasswordStrength
 * object. [isValid] is entropy >= 50 bits; [feedback] is a human-readable
 * reason; [score] is 0..4.
 */
@JsonClass(generateAdapter = true)
data class PasswordStrength(
    @Json(name = "is_valid") val isValid: Boolean = false,
    val entropy: Double? = null,
    val score: Int = 0,
    val feedback: String? = null,
    @Json(name = "min_entropy") val minEntropy: Double? = null,
    @Json(name = "char_set_size") val charSetSize: Int? = null,
    val length: Int? = null,
)

/** POST /password-reset/request body. */
@JsonClass(generateAdapter = true)
data class PasswordResetRequest(
    val email: String,
)

/** POST /password-reset/confirm body. */
@JsonClass(generateAdapter = true)
data class PasswordResetConfirmRequest(
    val token: String,
    val password: String,
)
