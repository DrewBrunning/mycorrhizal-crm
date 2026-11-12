package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.model.network.OccasionObligation
import com.mycorrhizal.crm.model.network.OccasionObligationInput
import com.mycorrhizal.crm.model.network.OccasionObligationsResponse
import com.mycorrhizal.crm.network.ApiClient
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class OccasionObligationRepositoryImplTest {

    private val apiClient = mockk<ApiClient>()
    private val repository = OccasionObligationRepositoryImpl(apiClient)

    private fun ob(id: String, deleted: Boolean = false) =
        OccasionObligation(id = id, entityId = "e1", deleted = deleted)

    @Test
    fun `listForContact follows next_cursor until exhausted and drops tombstones`() = runTest {
        coEvery { apiClient.listOccasionObligations("e1", null, any()) } returns Result.success(
            OccasionObligationsResponse(listOf(ob("a"), ob("t", deleted = true)), nextCursor = "c1")
        )
        coEvery { apiClient.listOccasionObligations("e1", "c1", any()) } returns Result.success(
            OccasionObligationsResponse(listOf(ob("b")), nextCursor = "")
        )

        val result = repository.listForContact("e1")

        assertEquals(listOf("a", "b"), result.getOrThrow().map { it.id })
        coVerify(exactly = 2) { apiClient.listOccasionObligations("e1", any(), any()) }
    }

    @Test
    fun `listForContact fails when a later page fails rather than returning a partial list`() =
        runTest {
            coEvery { apiClient.listOccasionObligations("e1", null, any()) } returns Result.success(
                OccasionObligationsResponse(listOf(ob("a")), nextCursor = "c1")
            )
            coEvery { apiClient.listOccasionObligations("e1", "c1", any()) } returns
                Result.failure(RuntimeException("offline"))

            assertTrue(repository.listForContact("e1").isFailure)
        }

    @Test
    fun `listForContact stops on a repeated cursor instead of looping forever`() = runTest {
        coEvery { apiClient.listOccasionObligations("e1", any(), any()) } returns Result.success(
            OccasionObligationsResponse(listOf(ob("a")), nextCursor = "same")
        )
        val result = repository.listForContact("e1")
        assertTrue(result.isSuccess)
        coVerify(atMost = 3) { apiClient.listOccasionObligations("e1", any(), any()) }
    }

    @Test
    fun `create update and delete pass straight through`() = runTest {
        val input = OccasionObligationInput(entityId = "e1", kind = "card", label = "x")
        coEvery { apiClient.createOccasionObligation(input) } returns Result.success(ob("n"))
        coEvery { apiClient.updateOccasionObligation("n", input) } returns Result.success(ob("n"))
        coEvery { apiClient.deleteOccasionObligation("n") } returns Result.success(Unit)

        assertEquals("n", repository.create(input).getOrThrow().id)
        assertEquals("n", repository.update("n", input).getOrThrow().id)
        assertTrue(repository.delete("n").isSuccess)
    }
}
