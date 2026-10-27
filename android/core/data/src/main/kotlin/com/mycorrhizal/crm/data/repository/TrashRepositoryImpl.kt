package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.domain.repository.TrashRepository
import com.mycorrhizal.crm.model.network.TrashListResponse
import com.mycorrhizal.crm.model.network.TrashRestoreResponse
import com.mycorrhizal.crm.network.ApiClient
import javax.inject.Inject

/** Contact trash access over [ApiClient]. Stateless pass-through (server-scoped, uncached). */
class TrashRepositoryImpl @Inject constructor(private val apiClient: ApiClient) : TrashRepository {
    override suspend fun list(page: Int, limit: Int): Result<TrashListResponse> =
        apiClient.getTrashedContacts(page = page, limit = limit)

    override suspend fun restore(id: Int): Result<TrashRestoreResponse> =
        apiClient.restoreTrashedContact(id)

    override suspend fun purge(id: Int): Result<Unit> = apiClient.purgeTrashedContact(id)

    override suspend fun empty(): Result<Int> = apiClient.emptyTrash()
}
