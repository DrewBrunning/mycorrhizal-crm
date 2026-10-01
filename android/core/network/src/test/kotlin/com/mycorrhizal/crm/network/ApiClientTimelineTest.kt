package com.mycorrhizal.crm.network

import com.mycorrhizal.crm.model.network.TimelineBuckets
import com.mycorrhizal.crm.model.network.TimelineTypes
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

/** Issue #1401: `GET /api/v1/contacts/{id}/timeline` client (the T66 endpoint behind "View all"). */
class ApiClientTimelineTest {

    private lateinit var server: MockWebServer
    private lateinit var client: ApiClient

    @Before
    fun setup() {
        server = MockWebServer()
        server.start()
        val okHttp = OkHttpClient.Builder()
            .addInterceptor(
                BaseUrlInterceptor(BaseUrlProvider { server.url("/").toString().trimEnd('/') }),
            )
            .build()
        val moshi = Moshi.Builder().add(KotlinJsonAdapterFactory()).build()
        client = ApiClient(okHttp, moshi)
    }

    @After
    fun teardown() {
        server.shutdown()
    }

    private fun enqueueEmpty() {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"items": [], "next_cursor": "", "limit": 25}"""))
    }

    @Test
    fun `no filters sends only the limit`() = runBlocking {
        enqueueEmpty()

        val result = client.getContactTimeline(7)

        assertTrue(result.isSuccess)
        assertEquals("/api/v1/contacts/7/timeline?limit=25", server.takeRequest().path)
    }

    @Test
    fun `a type subset a bucket and a cursor become query params`() = runBlocking {
        enqueueEmpty()

        // Given out of canonical order: the client normalizes to the backend's canonical order.
        client.getContactTimeline(
            7,
            types = setOf(TimelineTypes.GIFT, TimelineTypes.NOTE),
            bucket = TimelineBuckets.LAST_30_DAYS,
            cursor = "abc123",
            limit = 10,
        )

        assertEquals(
            "/api/v1/contacts/7/timeline?limit=10&type=note%2Cgift&bucket=last_30_days&cursor=abc123",
            server.takeRequest().path,
        )
    }

    @Test
    fun `all six types and the all bucket are the same query as no filter`() = runBlocking {
        enqueueEmpty()

        client.getContactTimeline(7, types = TimelineTypes.ALL, bucket = TimelineBuckets.ALL, cursor = "")

        assertEquals("/api/v1/contacts/7/timeline?limit=25", server.takeRequest().path)
    }

    @Test
    fun `parses every one of the six event types into its typed entity`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """
                {
                  "items": [
                    {"type": "note", "id": "11", "date": "2026-09-01T10:00:00Z",
                     "data": {"ID": 11, "content": "Loves climbing", "contact_id": 7}},
                    {"type": "activity", "id": "12", "date": "2026-08-30T10:00:00Z",
                     "data": {"ID": 12, "title": "Coffee", "type": "visit"}},
                    {"type": "completion", "id": "13", "date": "2026-08-29T10:00:00Z",
                     "data": {"ID": 13, "contact_id": 7, "message": "Call Dana", "completed_at": "2026-08-29T10:00:00Z"}},
                    {"type": "life_event", "id": "le-1", "date": "2026-08-20T00:00:00Z",
                     "data": {"id": "le-1", "entity_id": "u7", "type": "moved", "description": "Moved to Oslo"}},
                    {"type": "external_activity", "id": "ea-1", "date": "2026-08-10T10:00:00Z",
                     "data": {"id": "ea-1", "entity_id": "u7", "source_system": "immich",
                              "type": "photo-appearance", "payload": {"person_name": "Alice"}}},
                    {"type": "gift", "id": "g-1", "date": "2026-08-01T00:00:00Z",
                     "data": {"id": "g-1", "entity_id": "u7", "status": "given", "description": "Book"}}
                  ],
                  "next_cursor": "next-page-token",
                  "limit": 25
                }
                """.trimIndent(),
            ),
        )

        val page = client.getContactTimeline(7).getOrThrow()

        assertEquals(6, page.items.size)
        assertEquals("next-page-token", page.nextCursor)
        assertEquals(25, page.limit)
        assertEquals(11, page.items[0].note?.id)
        assertEquals("Loves climbing", page.items[0].note?.content)
        assertEquals("Coffee", page.items[1].activity?.title)
        assertEquals(13, page.items[2].completion?.id)
        assertEquals("Moved to Oslo", page.items[3].lifeEvent?.description)
        assertEquals("Alice", page.items[4].externalActivity?.payload?.get("person_name"))
        assertEquals("Book", page.items[5].gift?.description)
        assertEquals("2026-08-01T00:00:00Z", page.items[5].date)
    }

    @Test
    fun `an empty next_cursor means the last page and unknown event types are skipped`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """
                {"items": [
                   {"type": "hologram", "id": "x", "date": "2026-09-01T10:00:00Z", "data": {"id": "x"}},
                   {"type": "note", "id": "1", "date": "2026-09-01T10:00:00Z", "data": {"ID": 1, "content": "kept"}}
                 ], "next_cursor": "", "limit": 25}
                """.trimIndent(),
            ),
        )

        val page = client.getContactTimeline(7).getOrThrow()

        assertEquals(1, page.items.size)
        assertEquals("kept", page.items[0].note?.content)
        assertNull(page.nextCursor)
    }

    @Test
    fun `a null items array parses as an empty page`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"items": null, "next_cursor": "", "limit": 25}"""))

        val page = client.getContactTimeline(7).getOrThrow()

        assertTrue(page.items.isEmpty())
    }

    @Test
    fun `a 400 for a bad filter maps to Client 400`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(400)
                .setBody("""{"error":{"code":"invalid_input","message":"unknown timeline type"}}"""),
        )

        val result = client.getContactTimeline(7, types = setOf("bogus"))

        assertTrue(result.isFailure)
        assertEquals(400, (result.exceptionOrNull() as ApiError.Client).code)
    }

    @Test
    fun `a 404 maps to Client 404`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(404).setBody("""{"error":{"code":"not_found","message":"Contact not found"}}"""))

        val result = client.getContactTimeline(999)

        assertEquals(404, (result.exceptionOrNull() as ApiError.Client).code)
    }
}
