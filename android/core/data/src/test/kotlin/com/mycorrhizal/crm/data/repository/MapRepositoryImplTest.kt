package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.model.network.ContactMapResponse
import com.mycorrhizal.crm.model.network.GeocodeDraftRequest
import com.mycorrhizal.crm.model.network.GeocodeDraftResponse
import com.mycorrhizal.crm.model.network.MapConfig
import com.mycorrhizal.crm.network.ApiClient
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Test

class MapRepositoryImplTest {

    private val apiClient = mockk<ApiClient>()
    private val repository = MapRepositoryImpl(apiClient)

    @Test
    fun `getMapConfig passes through to the api client`() = runTest {
        coEvery { apiClient.getMapConfig() } returns Result.success(MapConfig("https://tiles.example/s"))

        assertEquals("https://tiles.example/s", repository.getMapConfig().getOrThrow().tileStyleUrl)
    }

    @Test
    fun `getContactMap passes through to the api client`() = runTest {
        val response = ContactMapResponse(truncated = true)
        coEvery { apiClient.getContactMap() } returns Result.success(response)

        assertSame(response, repository.getContactMap().getOrThrow())
    }

    @Test
    fun `geocodeAddressDraft passes the contact id and draft through`() = runTest {
        val draft = GeocodeDraftRequest(street = "1 Main St", city = "Springfield")
        coEvery { apiClient.geocodeAddressDraft(7, draft) } returns
            Result.success(GeocodeDraftResponse(coordinates = "geo:1,2"))

        assertEquals("geo:1,2", repository.geocodeAddressDraft(7, draft).getOrThrow().coordinates)
        coVerify(exactly = 1) { apiClient.geocodeAddressDraft(7, draft) }
    }

    @Test
    fun `failures propagate unchanged`() = runTest {
        val boom = RuntimeException("boom")
        coEvery { apiClient.getMapConfig() } returns Result.failure(boom)

        assertTrue(repository.getMapConfig().exceptionOrNull() === boom)
    }
}
