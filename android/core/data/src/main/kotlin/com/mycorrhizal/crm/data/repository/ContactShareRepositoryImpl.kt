package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.domain.repository.ContactShareRepository
import com.mycorrhizal.crm.model.network.ContactShare
import com.mycorrhizal.crm.model.network.ContactShareInput
import com.mycorrhizal.crm.model.network.ContactSharesPage
import com.mycorrhizal.crm.model.network.ImportConfirmRequest
import com.mycorrhizal.crm.model.network.ImportPreviewResponse
import com.mycorrhizal.crm.model.network.ImportResult
import com.mycorrhizal.crm.model.network.RowImportAction
import com.mycorrhizal.crm.model.network.UserDirectoryEntry
import com.mycorrhizal.crm.network.ApiClient
import javax.inject.Inject

/**
 * Online-only contact-share access — no Room mirror, since shares are
 * inherently online data (see ContactShareRepository's doc comment). Follows
 * the MergeRepositoryImpl precedent of a thin ApiClient passthrough.
 */
class ContactShareRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
) : ContactShareRepository {

    override suspend fun listIncoming(cursor: String?, limit: Int): Result<ContactSharesPage> =
        apiClient.listIncomingContactShares(cursor = cursor, limit = limit)

    override suspend fun listOutgoing(cursor: String?, limit: Int): Result<ContactSharesPage> =
        apiClient.listOutgoingContactShares(cursor = cursor, limit = limit)

    override suspend fun create(input: ContactShareInput): Result<ContactShare> =
        apiClient.createContactShare(input)

    override suspend fun accept(id: String): Result<ImportPreviewResponse> =
        apiClient.acceptContactShare(id)

    override suspend fun confirm(
        id: String,
        sessionId: String,
        actions: List<RowImportAction>,
    ): Result<ImportResult> =
        apiClient.confirmContactShare(id, ImportConfirmRequest(sessionId = sessionId, actions = actions))

    override suspend fun decline(id: String): Result<Unit> =
        apiClient.declineContactShare(id)

    override suspend fun userDirectory(): Result<List<UserDirectoryEntry>> =
        apiClient.getUserDirectory()
}
