package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.TrashListResponse
import com.mycorrhizal.crm.model.network.TrashRestoreResponse
import com.mycorrhizal.crm.model.network.TrashedContact
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class TrashRepositoryImplTest {

    private val apiClient = mockk<ApiClient>()
    private val repository = TrashRepositoryImpl(apiClient)

    @Test
    fun `list forwards the window and returns the response`() = runTest {
        val row = TrashedContact(contact = ContactSummary(id = 3))
        coEvery { apiClient.getTrashedContacts(page = 2, limit = 50) } returns
            Result.success(TrashListResponse(contacts = listOf(row), total = 1))

        val result = repository.list(page = 2, limit = 50)

        assertEquals(listOf(3), result.getOrThrow().contacts.map { it.contact.id })
        coVerify { apiClient.getTrashedContacts(page = 2, limit = 50) }
    }

    @Test
    fun `restore purge and empty delegate and propagate failures`() = runTest {
        coEvery { apiClient.restoreTrashedContact(3) } returns
            Result.success(TrashRestoreResponse(contact = ContactSummary(id = 3)))
        coEvery { apiClient.purgeTrashedContact(3) } returns Result.success(Unit)
        coEvery { apiClient.emptyTrash() } returns Result.failure(ApiError.Server(500, "boom"))

        assertEquals(3, repository.restore(3).getOrThrow().contact.id)
        assertTrue(repository.purge(3).isSuccess)
        assertTrue(repository.empty().isFailure)
    }
}
