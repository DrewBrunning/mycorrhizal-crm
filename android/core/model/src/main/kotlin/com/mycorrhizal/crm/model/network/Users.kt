package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * GET /api/v1/admin/users/{id} row and the per-user payload of the admin list —
 * mirrors the backend's AdminUserResponse (never carries the password hash).
 */
@JsonClass(generateAdapter = true)
data class AdminUser(
    val id: Int = 0,
    val username: String? = null,
    val email: String? = null,
    val language: String? = null,
    @Json(name = "date_format") val dateFormat: String? = null,
    @Json(name = "is_admin") val isAdmin: Boolean = false,
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "updated_at") val updatedAt: String? = null,
)

/** GET /api/v1/admin/users — the paginated user list response. */
@JsonClass(generateAdapter = true)
data class AdminUsersListResponse(
    val users: List<AdminUser> = emptyList(),
    val total: Int = 0,
    val page: Int = 1,
    val limit: Int = 25,
    @Json(name = "total_pages") val totalPages: Int = 0,
)

/** POST /api/v1/admin/users body — mirrors AdminUserCreateInput. */
@JsonClass(generateAdapter = true)
data class AdminUserCreateInput(
    val username: String,
    val email: String,
    val password: String,
    @Json(name = "is_admin") val isAdmin: Boolean = false,
)

/**
 * PATCH /api/v1/admin/users/{id} body — mirrors AdminUserUpdateInput. Every
 * field is nullable and, like Go's `omitempty` pointers, a null is omitted
 * from the wire (Moshi's generated adapter drops nulls), so an edit only sends
 * what the form filled in.
 */
@JsonClass(generateAdapter = true)
data class AdminUserUpdateInput(
    val username: String? = null,
    val email: String? = null,
    val password: String? = null,
    @Json(name = "is_admin") val isAdmin: Boolean? = null,
)
