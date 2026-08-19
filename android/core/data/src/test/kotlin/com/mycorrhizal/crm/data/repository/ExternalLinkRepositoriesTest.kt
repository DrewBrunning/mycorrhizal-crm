package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.model.network.ExternalIdentitiesPage
import com.mycorrhizal.crm.model.network.ExternalIdentity
import com.mycorrhizal.crm.model.network.ImmichConfigResponse
import com.mycorrhizal.crm.model.network.ImmichPerson
import com.mycorrhizal.crm.model.network.ImmichPersonSummary
import com.mycorrhizal.crm.network.ApiClient
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class ExternalIdentityRepositoryImplTest {

    private val apiClient = mockk<ApiClient>()
    private val repository = ExternalIdentityRepositoryImpl(apiClient)

    @Test
    fun `listForContact maps the page to its identities and forwards the uid`() = runTest {
        val identity = ExternalIdentity(id = "i1", entityId = "u5", system = "paperless", externalId = "doc-1")
        coEvery { apiClient.listExternalIdentities("u5", any()) } returns Result.success(
            ExternalIdentitiesPage(externalIdentities = listOf(identity), total = 1),
        )

        val result = repository.listForContact("u5")

        assertTrue(result.isSuccess)
        assertEquals(listOf(identity), result.getOrThrow())
    }

    @Test
    fun `delete forwards to the client`() = runTest {
        coEvery { apiClient.deleteExternalIdentity("i1") } returns Result.success(Unit)

        val result = repository.delete("i1")

        assertTrue(result.isSuccess)
        io.mockk.coVerify(exactly = 1) { apiClient.deleteExternalIdentity("i1") }
    }
}

class ImmichRepositoryImplTest {

    private val apiClient = mockk<ApiClient>()
    private val repository = ImmichRepositoryImpl(apiClient)

    @Test
    fun `isConfigured maps has_api_key`() = runTest {
        coEvery { apiClient.getImmichConfig() } returns Result.success(ImmichConfigResponse(hasApiKey = true))

        val result = repository.isConfigured()

        assertTrue(result.isSuccess)
        assertEquals(true, result.getOrThrow())
    }

    @Test
    fun `isConfigured is false when the connection has no key`() = runTest {
        coEvery { apiClient.getImmichConfig() } returns Result.success(ImmichConfigResponse())

        val result = repository.isConfigured()

        assertEquals(false, result.getOrThrow())
    }

    @Test
    fun `listPeople unwraps the response`() = runTest {
        coEvery { apiClient.listImmichPeople() } returns Result.success(
            listOf(ImmichPerson(id = "p1", name = "Alice")),
        )

        val result = repository.listPeople()

        assertEquals(1, result.getOrThrow().size)
        assertEquals("Alice", result.getOrThrow().first().name)
    }

    @Test
    fun `linkPerson maps the person into the request`() = runTest {
        coEvery { apiClient.linkImmichContact("u5", "p1", "Alice") } returns Result.success(Unit)

        val result = repository.linkPerson("u5", ImmichPerson(id = "p1", name = "Alice"))

        assertTrue(result.isSuccess)
        io.mockk.coVerify(exactly = 1) { apiClient.linkImmichContact("u5", "p1", "Alice") }
    }

    @Test
    fun `getContactSummary unwraps the nullable summary`() = runTest {
        coEvery { apiClient.getImmichContactSummary("u5") } returns Result.success(
            ImmichPersonSummary(personName = "Alice", photoCount = 3),
        )

        val result = repository.getContactSummary("u5")

        assertEquals("Alice", result.getOrThrow()?.personName)
    }

    @Test
    fun `listContactAssets unwraps the response`() = runTest {
        coEvery { apiClient.listImmichContactAssets("u5") } returns Result.success(
            listOf(com.mycorrhizal.crm.model.network.ImmichAssetSummary(id = "a1")),
        )

        val result = repository.listContactAssets("u5")

        assertEquals(1, result.getOrThrow().size)
    }

    @Test
    fun `getAssetImageBytes forwards the call`() = runTest {
        val bytes = ByteArray(64)
        coEvery { apiClient.getImmichAssetImageBytes("u5", "a1") } returns Result.success(bytes)

        val result = repository.getAssetImageBytes("u5", "a1")

        assertEquals(bytes.contentToString(), result.getOrThrow().contentToString())
    }
}
