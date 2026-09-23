package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.model.network.ContactScoreResponse
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** Mirrors [GraphRepositoryImplTest]'s shape: a stateless pass-through over [ApiClient]. */
class ContactScoreRepositoryImplTest {

    private val apiClient = mockk<ApiClient>()
    private val repository = ContactScoreRepositoryImpl(apiClient)

    @Test
    fun `getScore passes through to the api client`() = runTest {
        val response = ContactScoreResponse(contactId = 5, score = 72, band = "moss")
        coEvery { apiClient.getContactScore(5) } returns Result.success(response)

        val result = repository.getScore(5)

        assertTrue(result.isSuccess)
        assertEquals(response, result.getOrThrow())
        coVerify { apiClient.getContactScore(5) }
    }

    @Test
    fun `getScore propagates a failure verbatim`() = runTest {
        coEvery { apiClient.getContactScore(999) } returns
            Result.failure(ApiError.Client(404, "Contact not found"))

        val result = repository.getScore(999)

        assertTrue(result.isFailure)
        val error = result.exceptionOrNull() as ApiError
        assertTrue(error is ApiError.Client)
        assertEquals(404, (error as ApiError.Client).code)
    }
}
