package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.domain.repository.UserManagementRepository
import com.mycorrhizal.crm.model.network.AdminUser
import com.mycorrhizal.crm.model.network.AdminUserCreateInput
import com.mycorrhizal.crm.model.network.AdminUserUpdateInput
import com.mycorrhizal.crm.model.network.AdminUsersListResponse
import com.mycorrhizal.crm.network.ApiClient
import javax.inject.Inject

/**
 * Thin online-only passthrough to the admin user routes — no Room mirror (see
 * [UserManagementRepository]'s doc comment). Follows the ContactShareRepositoryImpl
 * precedent of a bare ApiClient facade.
 */
class UserManagementRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
) : UserManagementRepository {

    override suspend fun list(page: Int, limit: Int): Result<AdminUsersListResponse> =
        apiClient.listUsers(page = page, limit = limit)

    override suspend fun create(input: AdminUserCreateInput): Result<AdminUser> =
        apiClient.createUser(input)

    override suspend fun update(id: Int, input: AdminUserUpdateInput): Result<AdminUser> =
        apiClient.updateUser(id, input)

    override suspend fun delete(id: Int): Result<Unit> =
        apiClient.deleteUser(id)
}
