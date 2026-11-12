package com.mycorrhizal.crm.network

import com.mycorrhizal.crm.model.MoshiProvider
import com.mycorrhizal.crm.model.network.OccasionObligationInput
import kotlinx.coroutines.runBlocking
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/** ADR 0024 (issue #2521): the occasion-obligation endpoints, request shapes and response unwrapping. */
class ApiClientOccasionObligationsTest {

    private lateinit var server: MockWebServer
    private lateinit var client: ApiClient

    @Before
    fun setup() {
        server = MockWebServer()
        server.start()
        val okHttp = OkHttpClient.Builder()
            .addInterceptor(
                BaseUrlInterceptor(BaseUrlProvider { server.url("/").toString().trimEnd('/') })
            )
            .build()
        client = ApiClient(okHttp, MoshiProvider.get())
    }

    @After
    fun teardown() {
        server.shutdown()
    }

    private fun enqueue(body: String, code: Int = 200) {
        server.enqueue(MockResponse().setResponseCode(code).setBody(body))
    }

    private val input = OccasionObligationInput(
        entityId = "e1",
        kind = "card",
        label = "Christmas card",
        anchorMonth = 12,
        anchorDay = 25,
        leadTimeDays = 14
    )

    @Test
    fun `list sends the entity filter, cursor and limit`() = runBlocking {
        enqueue(
            """{"occasion_obligations":[{"id":"o1","kind":"card","label":"x"}],"total":1,"next_cursor":"","limit":100}"""
        )
        val page = client.listOccasionObligations(
            entityId = "e1",
            cursor = "c0",
            limit = 100
        ).getOrThrow()
        val request = server.takeRequest()
        assertEquals("GET", request.method)
        assertEquals("/api/v1/occasion-obligations", request.requestUrl?.encodedPath)
        assertEquals("e1", request.requestUrl?.queryParameter("entity_id"))
        assertEquals("c0", request.requestUrl?.queryParameter("cursor"))
        assertEquals("100", request.requestUrl?.queryParameter("limit"))
        assertEquals("o1", page.occasionObligations.single().id)
    }

    @Test
    fun `create posts the input and unwraps the wrapped obligation`() = runBlocking {
        enqueue(
            """{"message":"ok","occasion_obligation":{"id":"n1","label":"Christmas card"}}""",
            201
        )
        val created = client.createOccasionObligation(input).getOrThrow()
        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/api/v1/occasion-obligations", request.requestUrl?.encodedPath)
        val body = request.body.readUtf8()
        assertTrue(body, body.contains(""""anchor_month":12"""))
        assertTrue(body, body.contains(""""lead_time_days":14"""))
        assertEquals("n1", created.id)
    }

    @Test
    fun `update puts to the id and reads the raw obligation`() = runBlocking {
        enqueue("""{"id":"o9","label":"Renamed"}""")
        val updated = client.updateOccasionObligation("o9", input).getOrThrow()
        val request = server.takeRequest()
        assertEquals("PUT", request.method)
        assertEquals("/api/v1/occasion-obligations/o9", request.requestUrl?.encodedPath)
        assertEquals("Renamed", updated.label)
    }

    @Test
    fun `delete hits the id and a 404 surfaces as a failure with the reason`() = runBlocking {
        enqueue("""{"message":"deleted"}""")
        assertTrue(client.deleteOccasionObligation("o9").isSuccess)
        val request = server.takeRequest()
        assertEquals("DELETE", request.method)
        assertEquals("/api/v1/occasion-obligations/o9", request.requestUrl?.encodedPath)

        enqueue(
            """{"error":"not found","code":"NOT_FOUND","reason":"occasion_obligation_not_found"}""",
            404
        )
        val failed = client.deleteOccasionObligation("gone")
        assertTrue(failed.isFailure)
        assertNull(failed.getOrNull())
    }
}
