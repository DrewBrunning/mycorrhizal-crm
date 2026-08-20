package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.model.network.ExternalIdentitiesPage
import com.mycorrhizal.crm.model.network.ExternalIdentity
import com.mycorrhizal.crm.model.network.ImmichConfigInput
import com.mycorrhizal.crm.model.network.ImmichConfigResponse
import com.mycorrhizal.crm.model.network.ImmichConnectionTestResult
import com.mycorrhizal.crm.model.network.ImmichPerson
import com.mycorrhizal.crm.model.network.ImmichPersonSummary
import com.mycorrhizal.crm.model.network.NextcloudConfigResponse
import com.mycorrhizal.crm.model.network.PaperlessConfigResponse
import com.mycorrhizal.crm.model.network.PaperlessDocument
import com.mycorrhizal.crm.model.network.SeafileConfigResponse
import com.mycorrhizal.crm.model.network.SeafileItem
import com.mycorrhizal.crm.model.network.SeafileLibrary
import com.mycorrhizal.crm.model.network.SeafileLinkRequest
import com.mycorrhizal.crm.model.network.WebDAVItem
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

    @Test
    fun `getConfig forwards to the client`() = runTest {
        coEvery { apiClient.getImmichConfig() } returns Result.success(ImmichConfigResponse(hasApiKey = true))

        val result = repository.getConfig()

        assertTrue(result.getOrThrow().hasApiKey)
    }

    @Test
    fun `saveConfig forwards the input`() = runTest {
        val input = ImmichConfigInput(baseUrl = "https://immich.example", apiKey = "secret")
        coEvery { apiClient.saveImmichConfig(input) } returns Result.success(ImmichConfigResponse(hasApiKey = true))

        val result = repository.saveConfig(input)

        assertTrue(result.isSuccess)
        io.mockk.coVerify(exactly = 1) { apiClient.saveImmichConfig(input) }
    }

    @Test
    fun `deleteConfig forwards to the client`() = runTest {
        coEvery { apiClient.deleteImmichConfig() } returns Result.success(Unit)

        val result = repository.deleteConfig()

        assertTrue(result.isSuccess)
    }

    @Test
    fun `testConnection forwards to the client`() = runTest {
        coEvery { apiClient.testImmichConnection() } returns Result.success(
            ImmichConnectionTestResult(ok = false, message = "invalid key"),
        )

        val result = repository.testConnection()

        assertEquals(false, result.getOrThrow().ok)
    }
}

class PaperlessRepositoryImplTest {

    private val apiClient = mockk<ApiClient>()
    private val repository = PaperlessRepositoryImpl(apiClient)

    @Test
    fun `isConfigured maps has_api_token`() = runTest {
        coEvery { apiClient.getPaperlessConfig() } returns Result.success(PaperlessConfigResponse(hasApiToken = true))

        val result = repository.isConfigured()

        assertEquals(true, result.getOrThrow())
    }

    @Test
    fun `searchDocuments forwards the query`() = runTest {
        coEvery { apiClient.searchPaperlessDocuments("lease") } returns Result.success(
            listOf(PaperlessDocument(id = 1, title = "Lease")),
        )

        val result = repository.searchDocuments("lease")

        assertEquals(1, result.getOrThrow().size)
        io.mockk.coVerify(exactly = 1) { apiClient.searchPaperlessDocuments("lease") }
    }

    @Test
    fun `linkDocument forwards the document id`() = runTest {
        coEvery { apiClient.linkPaperlessContact("u5", "42") } returns Result.success(Unit)

        val result = repository.linkDocument("u5", "42")

        assertTrue(result.isSuccess)
        io.mockk.coVerify(exactly = 1) { apiClient.linkPaperlessContact("u5", "42") }
    }
}

class SeafileRepositoryImplTest {

    private val apiClient = mockk<ApiClient>()
    private val repository = SeafileRepositoryImpl(apiClient)

    @Test
    fun `isConfigured maps has_api_token`() = runTest {
        coEvery { apiClient.getSeafileConfig() } returns Result.success(SeafileConfigResponse(hasApiToken = true))

        val result = repository.isConfigured()

        assertEquals(true, result.getOrThrow())
    }

    @Test
    fun `listLibraries forwards to the client`() = runTest {
        coEvery { apiClient.listSeafileLibraries() } returns Result.success(listOf(SeafileLibrary(id = "r1", name = "Docs")))

        val result = repository.listLibraries()

        assertEquals(1, result.getOrThrow().size)
    }

    @Test
    fun `listDir forwards repoId and path`() = runTest {
        coEvery { apiClient.listSeafileDir("r1", "/docs") } returns Result.success(
            listOf(SeafileItem(id = "f1", name = "doc.pdf", type = "file")),
        )

        val result = repository.listDir("r1", "/docs")

        assertEquals(1, result.getOrThrow().size)
        io.mockk.coVerify(exactly = 1) { apiClient.listSeafileDir("r1", "/docs") }
    }

    @Test
    fun `linkItem forwards the request`() = runTest {
        val request = SeafileLinkRequest(repoId = "r1", path = "/docs/doc.pdf", name = "doc.pdf", type = "file")
        coEvery { apiClient.linkSeafileContact("u5", request) } returns Result.success(Unit)

        val result = repository.linkItem("u5", request)

        assertTrue(result.isSuccess)
        io.mockk.coVerify(exactly = 1) { apiClient.linkSeafileContact("u5", request) }
    }
}

class NextcloudRepositoryImplTest {

    private val apiClient = mockk<ApiClient>()
    private val repository = NextcloudRepositoryImpl(apiClient)

    @Test
    fun `isConfigured maps has_app_password`() = runTest {
        coEvery { apiClient.getNextcloudConfig() } returns Result.success(
            NextcloudConfigResponse(hasAppPassword = true),
        )

        val result = repository.isConfigured()

        assertEquals(true, result.getOrThrow())
    }

    @Test
    fun `listDir defaults to the dav root`() = runTest {
        coEvery { apiClient.listNextcloudDir("/") } returns Result.success(
            listOf(WebDAVItem(name = "Photos", path = "/Photos", type = "dir")),
        )

        val result = repository.listDir()

        assertEquals(1, result.getOrThrow().size)
        io.mockk.coVerify(exactly = 1) { apiClient.listNextcloudDir("/") }
    }

    @Test
    fun `linkItem maps the WebDAV item into the link request`() = runTest {
        val item = WebDAVItem(name = "img.jpg", path = "/Photos/img.jpg", type = "file", size = 2048)
        coEvery {
            apiClient.linkNextcloudContact(
                "u5",
                com.mycorrhizal.crm.model.network.NextcloudLinkRequest(
                    path = "/Photos/img.jpg",
                    name = "img.jpg",
                    type = "file",
                    size = 2048,
                ),
            )
        } returns Result.success(Unit)

        val result = repository.linkItem("u5", item)

        assertTrue(result.isSuccess)
    }
}
