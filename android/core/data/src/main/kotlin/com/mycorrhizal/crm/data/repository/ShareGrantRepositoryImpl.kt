package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.domain.repository.ShareGrantRepository
import com.mycorrhizal.crm.model.network.IncomingShareGrant
import com.mycorrhizal.crm.model.network.IncomingShareGrantsPage
import com.mycorrhizal.crm.model.network.OutgoingShareGrant
import com.mycorrhizal.crm.model.network.OutgoingShareGrantsPage
import com.mycorrhizal.crm.model.network.ShareBlock
import com.mycorrhizal.crm.model.network.ShareGrantContent
import com.mycorrhizal.crm.model.network.ShareGrantInput
import com.mycorrhizal.crm.model.network.ShareGrantNotifications
import com.mycorrhizal.crm.model.network.ShareGrantPatch
import com.mycorrhizal.crm.network.ApiClient
import javax.inject.Inject

/** Online-only passthrough (see [ShareGrantRepository]): no Room mirror, no in-memory cache. */
class ShareGrantRepositoryImpl @Inject constructor(private val apiClient: ApiClient) :
    ShareGrantRepository {

    override suspend fun listIncoming(cursor: String?, limit: Int): Result<IncomingShareGrantsPage> =
        apiClient.listIncomingShareGrants(cursor, limit)

    override suspend fun listOutgoing(cursor: String?, limit: Int): Result<OutgoingShareGrantsPage> =
        apiClient.listOutgoingShareGrants(cursor, limit)

    override suspend fun create(input: ShareGrantInput): Result<OutgoingShareGrant> =
        apiClient.createShareGrant(input)

    override suspend fun update(
        id: String,
        patch: ShareGrantPatch,
        revision: Int?
    ): Result<OutgoingShareGrant> = apiClient.updateShareGrant(id, patch, revision)

    override suspend fun revoke(id: String): Result<OutgoingShareGrant> =
        apiClient.revokeShareGrant(id)

    override suspend fun reconfirm(id: String): Result<OutgoingShareGrant> =
        apiClient.reconfirmShareGrant(id)

    override suspend fun accept(id: String): Result<IncomingShareGrant> =
        apiClient.acceptShareGrant(id)

    override suspend fun decline(id: String, block: Boolean): Result<IncomingShareGrant> =
        apiClient.declineShareGrant(id, block)

    override suspend fun leave(id: String): Result<IncomingShareGrant> =
        apiClient.leaveShareGrant(id)

    override suspend fun content(id: String): Result<ShareGrantContent> =
        apiClient.getShareGrantContent(id)

    override suspend fun blocks(): Result<List<ShareBlock>> = apiClient.listShareBlocks()

    override suspend fun unblock(userId: Long): Result<Unit> = apiClient.deleteShareBlock(userId)

    override suspend fun notifications(): Result<ShareGrantNotifications> =
        apiClient.getShareGrantNotifications()

    override suspend fun markNotificationsRead(): Result<Unit> =
        apiClient.markShareGrantNotificationsRead()
}
