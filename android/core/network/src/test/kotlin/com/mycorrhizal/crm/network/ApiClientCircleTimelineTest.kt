package com.mycorrhizal.crm.network

import com.mycorrhizal.crm.model.network.CircleInput
import com.squareup.moshi.Moshi
import com.squareup.moshi.kotlin.reflect.KotlinJsonAdapterFactory
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

/**
 * Issues #1995/#2361 Android parity (#2522): circle description + colour on the wire and
 * `GET /api/v1/circles/{id}/timeline`.
 */
class ApiClientCircleTimelineTest {

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
        val moshi = Moshi.Builder().add(KotlinJsonAdapterFactory()).build()
        client = ApiClient(okHttp, moshi)
    }

    @After
    fun teardown() {
        server.shutdown()
    }

    @Test
    fun `timeline request sends the default limit of 25 and no cursor`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(200)
                .setBody("""{"items": [], "next_cursor": "", "limit": 25, "member_count": 0}""")
        )

        val result = client.getCircleTimeline("c-1")

        assertTrue(result.isSuccess)
        assertEquals("/api/v1/circles/c-1/timeline?limit=25", server.takeRequest().path)
    }

    @Test
    fun `timeline request forwards the cursor and an explicit limit`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(200)
                .setBody("""{"items": [], "next_cursor": "", "limit": 5, "member_count": 0}""")
        )

        client.getCircleTimeline("c-1", cursor = "abc123", limit = 5)

        assertEquals(
            "/api/v1/circles/c-1/timeline?limit=5&cursor=abc123",
            server.takeRequest().path
        )
    }

    @Test
    fun `timeline parses items with their members, the cursor and member_count`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """
                {
                  "items": [
                    {"type": "note", "id": "11", "date": "2026-09-01T10:00:00Z",
                     "data": {"ID": 11, "content": "Loves climbing"},
                     "members": [{"id": 7, "firstname": "Dana", "lastname": "Lee"}]},
                    {"type": "activity", "id": "12", "date": "2026-08-30T10:00:00Z",
                     "data": {"ID": 12, "title": "Coffee"},
                     "members": [{"id": 7, "firstname": "Dana", "lastname": "Lee"},
                                 {"id": 8, "firstname": "Sam", "lastname": ""}]}
                  ],
                  "next_cursor": "next-1", "limit": 25, "member_count": 3
                }
                """.trimIndent()
            )
        )

        val page = client.getCircleTimeline("c-1").getOrThrow()

        assertEquals(3, page.memberCount)
        assertEquals("next-1", page.nextCursor)
        assertEquals(listOf("note", "activity"), page.items.map { it.type })
        assertEquals("Loves climbing", page.items[0].data?.get("content"))
        assertEquals(listOf(7), page.items[0].members.map { it.id })
        assertEquals(listOf("Dana Lee", "Sam"), page.items[1].members.map { it.displayName() })
    }

    @Test
    fun `the last page maps an empty next_cursor to null`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(200)
                .setBody("""{"items": [], "next_cursor": "", "limit": 25, "member_count": 2}""")
        )

        val page = client.getCircleTimeline("c-1").getOrThrow()

        assertNull(page.nextCursor)
        assertTrue(page.items.isEmpty())
        assertEquals(2, page.memberCount)
    }

    @Test
    fun `a 404 surfaces as a failed result`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(404)
                .setBody("""{"error":{"code":"not_found","message":"Circle not found"}}""")
        )

        val result = client.getCircleTimeline("gone")

        assertTrue(result.isFailure)
        assertTrue(result.exceptionOrNull() is ApiError.Client)
    }

    @Test
    fun `updateCircle sends description and colour and parses them back`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"id":"c-1","name":"Family","description":"Close ones","color":"teal"}"""
            )
        )

        val circle = client.updateCircle(
            "c-1",
            CircleInput(name = "Family", description = "Close ones", color = "teal")
        ).getOrThrow()

        val body = server.takeRequest().body.readUtf8()
        assertTrue(body, body.contains("\"description\":\"Close ones\""))
        assertTrue(body, body.contains("\"color\":\"teal\""))
        assertEquals("Close ones", circle.description)
        assertEquals("teal", circle.color)
    }

    @Test
    fun `an empty colour is sent to clear it while an omitted field is not sent`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"id":"c-1","name":"A"}"""))
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"id":"c-1","name":"A"}"""))

        client.updateCircle("c-1", CircleInput(name = "A", description = "", color = ""))
        client.updateCircle("c-1", CircleInput(name = "A"))

        val cleared = server.takeRequest().body.readUtf8()
        assertTrue(cleared, cleared.contains("\"color\":\"\""))
        assertTrue(cleared, cleared.contains("\"description\":\"\""))
        val rename = server.takeRequest().body.readUtf8()
        assertEquals("{\"name\":\"A\"}", rename)
    }

    @Test
    fun `a circle without metadata decodes to empty description and colour`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"id":"c-1","name":"A"}"""))

        val circle = client.updateCircle("c-1", CircleInput(name = "A")).getOrThrow()

        assertEquals("", circle.description)
        assertEquals("", circle.color)
    }
}
