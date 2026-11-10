package com.mycorrhizal.crm.data.repository

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
import com.mycorrhizal.crm.network.ApiError
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class ShareGrantRepositoryImplTest {

    private val apiClient = mockk<ApiClient>()
    private val repository = ShareGrantRepositoryImpl(apiClient)

    @Test
    fun `lists forward cursor and limit`() = runTest {
        coEvery { apiClient.listIncomingShareGrants("c", 5) } returns
            Result.success(IncomingShareGrantsPage(listOf(IncomingShareGrant(id = "g1"))))
        coEvery { apiClient.listOutgoingShareGrants("c", 5) } returns
            Result.success(OutgoingShareGrantsPage(listOf(OutgoingShareGrant(id = "g2"))))

        assertEquals("g1", repository.listIncoming("c", 5).getOrThrow().shareGrants.single().id)
        assertEquals("g2", repository.listOutgoing("c", 5).getOrThrow().shareGrants.single().id)
    }

    @Test
    fun `owner mutations forward to the client`() = runTest {
        val input = ShareGrantInput(9, "contact", "uid", listOf("emails"))
        val patch = ShareGrantPatch(clearExpiry = true)
        coEvery {
            apiClient.createShareGrant(
                input
            )
        } returns Result.success(OutgoingShareGrant("g"))
        coEvery { apiClient.updateShareGrant("g", patch, 3) } returns
            Result.success(OutgoingShareGrant("g"))
        coEvery { apiClient.revokeShareGrant("g") } returns Result.success(OutgoingShareGrant("g"))
        coEvery {
            apiClient.reconfirmShareGrant(
                "g"
            )
        } returns Result.success(OutgoingShareGrant("g"))

        assertTrue(repository.create(input).isSuccess)
        assertTrue(repository.update("g", patch, 3).isSuccess)
        assertTrue(repository.revoke("g").isSuccess)
        assertTrue(repository.reconfirm("g").isSuccess)
        coVerify(exactly = 1) { apiClient.updateShareGrant("g", patch, 3) }
    }

    @Test
    fun `recipient actions forward to the client`() = runTest {
        coEvery { apiClient.acceptShareGrant("g") } returns Result.success(IncomingShareGrant("g"))
        coEvery { apiClient.leaveShareGrant("g") } returns Result.success(IncomingShareGrant("g"))
        coEvery { apiClient.declineShareGrant("g", true) } returns
            Result.success(IncomingShareGrant("g"))
        coEvery { apiClient.declineShareGrant("g", false) } returns
            Result.success(IncomingShareGrant("g"))

        assertTrue(repository.accept("g").isSuccess)
        assertTrue(repository.leave("g").isSuccess)
        assertTrue(repository.decline("g", block = true).isSuccess)
        assertTrue(repository.decline("g").isSuccess)
        coVerify { apiClient.declineShareGrant("g", true) }
    }

    @Test
    fun `content blocks and notifications forward and failures propagate`() = runTest {
        coEvery { apiClient.getShareGrantContent("g") } returns
            Result.success(ShareGrantContent(grantId = "g"))
        coEvery { apiClient.listShareBlocks() } returns Result.success(
            listOf(ShareBlock(7, "dana"))
        )
        coEvery { apiClient.deleteShareBlock(7) } returns Result.success(Unit)
        coEvery { apiClient.getShareGrantNotifications() } returns
            Result.failure(ApiError.Client(404, "x"))
        coEvery { apiClient.markShareGrantNotificationsRead() } returns Result.success(Unit)

        assertEquals("g", repository.content("g").getOrThrow().grantId)
        assertEquals("dana", repository.blocks().getOrThrow().single().username)
        assertTrue(repository.unblock(7).isSuccess)
        assertTrue(repository.notifications().isFailure)
        assertTrue(repository.markNotificationsRead().isSuccess)
        assertEquals(0, ShareGrantNotifications().pendingCount)
    }
}
