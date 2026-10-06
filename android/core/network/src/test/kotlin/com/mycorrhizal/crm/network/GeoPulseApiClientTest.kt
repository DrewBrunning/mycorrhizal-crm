package com.mycorrhizal.crm.network

import com.mycorrhizal.crm.model.MoshiProvider
import com.mycorrhizal.crm.model.network.GeoPulseConfigInput
import kotlinx.coroutines.runBlocking
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * Issue #160: wire contract of the GeoPulse endpoints against the real (codegen) Moshi adapters.
 * Bodies mirror the `GeoPulse*` schemas in backend/openapi.yaml (the spec carries no response
 * `example:` for these yet, so there is no generated testdata/contract-fixtures file to read).
 */
class GeoPulseApiClientTest {

    private lateinit var server: MockWebServer
    private lateinit var client: ApiClient

    @Before
    fun setup() {
        server = MockWebServer()
        server.start()
        val okHttp = OkHttpClient.Builder()
            .addInterceptor(BaseUrlInterceptor(BaseUrlProvider { server.url("/").toString().trimEnd('/') }))
            .build()
        client = ApiClient(okHttp, MoshiProvider.get())
    }

    @After
    fun teardown() {
        server.shutdown()
    }

    @Test
    fun `getGeoPulseConfig parses has_api_key and never expects a token`() = runBlocking {
        server.enqueue(MockResponse().setBody("""{"base_url":"https://geo.example.com","has_api_key":true}"""))

        val config = client.getGeoPulseConfig().getOrThrow()

        assertEquals("https://geo.example.com", config.baseUrl)
        assertTrue(config.hasApiKey)
        assertEquals("/api/v1/geopulse/config", server.takeRequest().path)
    }

    @Test
    fun `saveGeoPulseConfig PUTs snake_case base_url and api_key`() = runBlocking {
        server.enqueue(MockResponse().setBody("""{"base_url":"https://geo.example.com","has_api_key":true}"""))

        client.saveGeoPulseConfig(GeoPulseConfigInput("https://geo.example.com", "tok")).getOrThrow()

        val req = server.takeRequest()
        assertEquals("PUT", req.method)
        assertEquals("/api/v1/geopulse/config", req.path)
        val body = req.body.readUtf8()
        assertTrue(body, body.contains("\"base_url\":\"https://geo.example.com\""))
        assertTrue(body, body.contains("\"api_key\":\"tok\""))
    }

    @Test
    fun `a 400 on api_key surfaces the server message`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(400)
                .setBody("""{"error":"re-enter the API token when changing the GeoPulse server"}"""),
        )

        val result = client.saveGeoPulseConfig(GeoPulseConfigInput("https://other.example.com"))

        assertTrue(result.isFailure)
        assertTrue(result.exceptionOrNull() is ApiError.Client)
        assertEquals(400, (result.exceptionOrNull() as ApiError.Client).code)
    }

    @Test
    fun `deleteGeoPulseConfig and testGeoPulseConnection use the right verbs`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"message":"ok"}"""))
        server.enqueue(MockResponse().setBody("""{"ok":false,"stage":"auth","message":"API token rejected"}"""))

        assertTrue(client.deleteGeoPulseConfig().isSuccess)
        val test = client.testGeoPulseConnection().getOrThrow()

        assertEquals("DELETE", server.takeRequest().method)
        val req = server.takeRequest()
        assertEquals("POST", req.method)
        assertEquals("/api/v1/geopulse/test-connection", req.path)
        assertFalse(test.ok)
        assertEquals("auth", test.stage)
        assertEquals("API token rejected", test.message)
    }

    @Test
    fun `getGeoPulseSuggestions sends date and timezone and parses a stay`() = runBlocking {
        server.enqueue(
            MockResponse().setBody(
                """
                {"date":"2026-03-10","suggestions":[
                  {"stay_id":42,"external_ref":"geopulse:stay:42","location":"Cafe Roma","city":"Rome",
                   "country":"Italy","latitude":41.9,"longitude":12.5,"timestamp":"2026-03-10T14:30:00Z",
                   "duration_seconds":5400,
                   "photos":[{"id":"p1","file_name":"IMG_1.jpg","taken_at":"2026-03-10T14:40:00Z"}],
                   "photos_unavailable":false,"existing_activity_id":7},
                  {"stay_id":43,"external_ref":"geopulse:stay:43","location":"Park","city":"","country":"",
                   "latitude":0,"longitude":0,"timestamp":"2026-03-10T16:00:00Z","duration_seconds":60,
                   "photos":[],"photos_unavailable":true}
                ]}
                """.trimIndent(),
            ),
        )

        val resp = client.getGeoPulseSuggestions("2026-03-10", "Europe/Rome").getOrThrow()

        val path = server.takeRequest().path!!
        assertTrue(path, path.startsWith("/api/v1/geopulse/suggestions?"))
        assertTrue(path, path.contains("date=2026-03-10"))
        assertTrue(path, path.contains("timezone=Europe%2FRome"))
        assertEquals(2, resp.suggestions.size)
        val first = resp.suggestions[0]
        assertEquals("geopulse:stay:42", first.externalRef)
        assertEquals(5400L, first.durationSeconds)
        assertEquals("IMG_1.jpg", first.photos.single().fileName)
        assertEquals(7, first.existingActivityId)
        val second = resp.suggestions[1]
        assertTrue(second.photosUnavailable)
        assertNull(second.existingActivityId)
    }

    @Test
    fun `timezone is omitted when null`() = runBlocking {
        server.enqueue(MockResponse().setBody("""{"date":"2026-03-10","suggestions":[]}"""))

        client.getGeoPulseSuggestions("2026-03-10").getOrThrow()

        assertFalse(server.takeRequest().path!!.contains("timezone"))
    }

    @Test
    fun `503 external-service error surfaces the server's actionable message`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(503).setBody(
                """{"error":{"code":"EXTERNAL_SERVICE_ERROR","message":"GeoPulse service error: Could not reach GeoPulse. Is the instance up?"}}""",
            ),
        )

        val result = client.getGeoPulseSuggestions("2026-03-10", "UTC")

        assertEquals(
            "GeoPulse service error: Could not reach GeoPulse. Is the instance up?",
            (result.exceptionOrNull() as ApiError).displayMessage,
        )
    }

    @Test
    fun `500 internal and database errors stay generic`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(500)
                .setBody("""{"error":{"code":"INTERNAL_ERROR","message":"sql: connection refused at 10.0.0.5"}}"""),
        )
        server.enqueue(
            MockResponse().setResponseCode(503)
                .setBody("""{"error":{"code":"DATABASE_ERROR","message":"database is locked"}}"""),
        )

        assertEquals("Server error (500)", (client.getGeoPulseConfig().exceptionOrNull() as ApiError).displayMessage)
        assertEquals("Server error (503)", (client.getGeoPulseConfig().exceptionOrNull() as ApiError).displayMessage)
    }
}
