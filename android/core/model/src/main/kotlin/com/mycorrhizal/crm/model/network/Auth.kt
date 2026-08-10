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
)
