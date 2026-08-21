package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.model.network.AuditEvent
import com.mycorrhizal.crm.model.network.AuditEventsResponse
import com.mycorrhizal.crm.model.network.AuditUndoResponse
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class AuditRepositoryImplTest {

    private val apiClient = mockk<ApiClient>()
    private val repository = AuditRepositoryImpl(apiClient)

    @Test
    fun `list forwards the filters and returns the response`() = runTest {
        val event = AuditEvent(id = 1, entityType = "contact", entityId = "c1", operation = "update")
        coEvery {
            apiClient.getAuditEvents(entityType = "contact", entityId = "c1", limit = 20)
        } returns Result.success(AuditEventsResponse(auditEvents = listOf(event), total = 1))

        val result = repository.list(entityType = "contact", entityId = "c1", limit = 20)

        assertTrue(result.isSuccess)
        assertEquals(listOf(1L), result.getOrThrow().auditEvents.map { it.id })
        coVerify { apiClient.getAuditEvents(entityType = "contact", entityId = "c1", limit = 20) }
    }

    @Test
    fun `list failure propagates`() = runTest {
        coEvery {
            apiClient.getAuditEvents(entityType = null, entityId = null, limit = 20)
        } returns Result.failure(ApiError.Server(500, "boom"))

        val result = repository.list(entityType = null, entityId = null, limit = 20)

        assertTrue(result.isFailure)
    }

    @Test
    fun `undo discards the response message and returns Unit on success`() = runTest {
        coEvery { apiClient.undoAuditEvent(42L) } returns Result.success(AuditUndoResponse(message = "undone"))

        val result = repository.undo(42L)

        assertTrue(result.isSuccess)
        assertEquals(Unit, result.getOrThrow())
    }

    @Test
    fun `undo failure propagates`() = runTest {
        coEvery { apiClient.undoAuditEvent(42L) } returns Result.failure(ApiError.Client(404, "not found"))

        val result = repository.undo(42L)

        assertTrue(result.isFailure)
    }
}
