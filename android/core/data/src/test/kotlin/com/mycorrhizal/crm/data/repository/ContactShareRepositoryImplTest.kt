package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.model.network.ContactShare
import com.mycorrhizal.crm.model.network.ContactShareInput
import com.mycorrhizal.crm.model.network.ContactSharesPage
import com.mycorrhizal.crm.model.network.ImportConfirmRequest
import com.mycorrhizal.crm.model.network.ImportPreviewResponse
import com.mycorrhizal.crm.model.network.ImportResult
import com.mycorrhizal.crm.model.network.RowImportAction
import com.mycorrhizal.crm.model.network.UserDirectoryEntry
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class ContactShareRepositoryImplTest {

    private val apiClient = mockk<ApiClient>()
    private val repository = ContactShareRepositoryImpl(apiClient)

    @Test
    fun `listIncoming forwards cursor and limit`() = runTest {
        val share = ContactShare(id = "s1", contactDisplayName = "Alice")
        coEvery {
            apiClient.listIncomingContactShares(cursor = "c1", limit = 10)
        } returns Result.success(ContactSharesPage(contactShares = listOf(share), total = 1, limit = 10))

        val result = repository.listIncoming(cursor = "c1", limit = 10)

        assertTrue(result.isSuccess)
        assertEquals(listOf("s1"), result.getOrThrow().contactShares.map { it.id })
    }

    @Test
    fun `listOutgoing failure propagates`() = runTest {
        coEvery {
            apiClient.listOutgoingContactShares(cursor = null, limit = 20)
        } returns Result.failure(ApiError.Server(500, "boom"))

        val result = repository.listOutgoing(cursor = null, limit = 20)

        assertTrue(result.isFailure)
    }

    @Test
    fun `create forwards the input`() = runTest {
        val input = ContactShareInput(toUserId = 5, vcardUid = "u1", sections = listOf("emails"))
        coEvery { apiClient.createContactShare(input) } returns Result.success(
            ContactShare(id = "s2", toUserId = 5),
        )

        val result = repository.create(input)

        assertTrue(result.isSuccess)
        assertEquals(5L, result.getOrThrow().toUserId)
    }

    @Test
    fun `accept returns the import preview`() = runTest {
        coEvery { apiClient.acceptContactShare("s1") } returns Result.success(ImportPreviewResponse())

        val result = repository.accept("s1")

        assertTrue(result.isSuccess)
    }

    @Test
    fun `confirm wraps the session id and actions into the request`() = runTest {
        val actions = listOf(RowImportAction(rowIndex = 0, action = "import"))
        coEvery {
            apiClient.confirmContactShare("s1", ImportConfirmRequest(sessionId = "sess1", actions = actions))
        } returns Result.success(ImportResult())

        val result = repository.confirm("s1", "sess1", actions)

        assertTrue(result.isSuccess)
        coVerify {
            apiClient.confirmContactShare("s1", ImportConfirmRequest(sessionId = "sess1", actions = actions))
        }
    }

    @Test
    fun `decline delegates to the api client`() = runTest {
        coEvery { apiClient.declineContactShare("s1") } returns Result.success(Unit)

        val result = repository.decline("s1")

        assertTrue(result.isSuccess)
    }

    @Test
    fun `userDirectory returns the directory list`() = runTest {
        coEvery { apiClient.getUserDirectory() } returns Result.success(
            listOf(UserDirectoryEntry(id = 1, username = "alice")),
        )

        val result = repository.userDirectory()

        assertTrue(result.isSuccess)
        assertEquals(listOf("alice"), result.getOrThrow().map { it.username })
    }
}
