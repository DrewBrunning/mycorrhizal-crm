package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.model.network.BulkContactOperationInput
import com.mycorrhizal.crm.model.network.BulkOperationResult
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.CRMEnvelope
import com.mycorrhizal.crm.model.network.ContactMergePreviewResponse
import com.mycorrhizal.crm.model.network.ContactMergeRequest
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class MergeRepositoryImplTest {

    private val apiClient = mockk<ApiClient>()
    private val repository = MergeRepositoryImpl(apiClient)

    @Test
    fun `preview forwards the request and returns the response`() = runTest {
        val request = ContactMergeRequest(keepId = 1, mergeId = 2)
        coEvery { apiClient.previewMerge(request) } returns Result.success(
            ContactMergePreviewResponse(keepId = 1, mergeId = 2),
        )

        val result = repository.preview(request)

        assertTrue(result.isSuccess)
        assertEquals(1L, result.getOrThrow().keepId)
    }

    @Test
    fun `preview failure propagates`() = runTest {
        val request = ContactMergeRequest(keepId = 1, mergeId = 2)
        coEvery { apiClient.previewMerge(request) } returns Result.failure(ApiError.Client(409, "conflict"))

        val result = repository.preview(request)

        assertTrue(result.isFailure)
    }

    @Test
    fun `commit forwards the request and returns the merged contact`() = runTest {
        val request = ContactMergeRequest(keepId = 1, mergeId = 2)
        val response = ContactRecordResponse(card = Card(uid = "1"), crm = CRMEnvelope())
        coEvery { apiClient.commitMerge(request) } returns Result.success(response)

        val result = repository.commit(request)

        assertTrue(result.isSuccess)
        assertEquals("1", result.getOrThrow().card?.uid)
    }

    @Test
    fun `commit failure propagates`() = runTest {
        val request = ContactMergeRequest(keepId = 1, mergeId = 2)
        coEvery { apiClient.commitMerge(request) } returns Result.failure(ApiError.Server(500, "boom"))

        val result = repository.commit(request)

        assertTrue(result.isFailure)
    }
}

class BulkOperationRepositoryImplTest {

    private val apiClient = mockk<ApiClient>()
    private val repository = BulkOperationRepositoryImpl(apiClient)

    @Test
    fun `run forwards the input and returns the result`() = runTest {
        val input = BulkContactOperationInput(action = "archive", vcardUids = listOf("u1", "u2"))
        coEvery { apiClient.bulkOperation(input) } returns Result.success(
            BulkOperationResult(action = "archive", total = 2, succeeded = 2, failed = 0),
        )

        val result = repository.run(input)

        assertTrue(result.isSuccess)
        assertEquals(2, result.getOrThrow().succeeded)
    }

    @Test
    fun `run failure propagates`() = runTest {
        val input = BulkContactOperationInput(action = "archive", vcardUids = listOf("u1"))
        coEvery { apiClient.bulkOperation(input) } returns Result.failure(ApiError.Client(400, "bad request"))

        val result = repository.run(input)

        assertTrue(result.isFailure)
    }
}
