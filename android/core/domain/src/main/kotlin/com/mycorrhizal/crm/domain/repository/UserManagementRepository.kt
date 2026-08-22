package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.AdminUser
import com.mycorrhizal.crm.model.network.AdminUserCreateInput
import com.mycorrhizal.crm.model.network.AdminUserUpdateInput
import com.mycorrhizal.crm.model.network.AdminUsersListResponse

/**
 * Admin-only user management (issue #348). Online-first with no Room mirror —
 * like [ContactShareRepository], this surface is inherently server data (and
 * only ever reachable by an admin). Every call 403s server-side for a
 * non-admin caller, so the UI gating is a navigation affordance, not a guard.
 */
interface UserManagementRepository {
    /** GET /admin/users — paginated, id-ASC list. */
    suspend fun list(page: Int = 1, limit: Int = 100): Result<AdminUsersListResponse>

    /** POST /admin/users — create a user; returns the created user. */
    suspend fun create(input: AdminUserCreateInput): Result<AdminUser>

    /** PATCH /admin/users/{id} — update a user; returns the updated user. */
    suspend fun update(id: Int, input: AdminUserUpdateInput): Result<AdminUser>

    /** DELETE /admin/users/{id} — hard-deletes the account and all its data (T26). */
    suspend fun delete(id: Int): Result<Unit>
}
