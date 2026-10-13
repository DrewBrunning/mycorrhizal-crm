package com.mycorrhizal.crm.network

import com.mycorrhizal.crm.model.network.ActivityInput
import com.mycorrhizal.crm.model.network.ContactRecordInput
import com.mycorrhizal.crm.model.network.LifeEventInput
import com.mycorrhizal.crm.model.network.NoteInput
import com.mycorrhizal.crm.model.network.Reminder
import com.squareup.moshi.Moshi
import com.squareup.moshi.kotlin.reflect.KotlinJsonAdapterFactory
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
 * Issue #1665 / ADR 0008: PUT and DELETE on the five revision-bearing entities carry
 * `If-Match: "<revision>"` when the caller has a revision, omit it when it does not, and a
 * 412 comes back as the typed [ApiError.PreconditionFailed] (never the generic 4xx).
 */
class ApiClientIfMatchTest {

    private lateinit var server: MockWebServer
    private lateinit var client: ApiClient

    @Before
    fun setup() {
        server = MockWebServer()
        server.start()
        val okHttp = OkHttpClient.Builder()
            .addInterceptor(BaseUrlInterceptor(BaseUrlProvider { server.url("/").toString().trimEnd('/') }))
            .build()
        client = ApiClient(okHttp, Moshi.Builder().add(KotlinJsonAdapterFactory()).build())
    }

    @After
    fun teardown() {
        server.shutdown()
    }

    /** One body that satisfies every update mapper (raw DTO, `{note}` and `{reminder}` wrappers). */
    private val okBody = """{"ID":1,"revision":9,"note":{"ID":1,"revision":9},"reminder":{"ID":1,"revision":9}}"""

    private data class Write(val name: String, val method: String, val path: String, val call: suspend (ApiClient, Int?) -> Result<*>)

    private val writes = listOf(
        Write("updateContact", "PUT", "/api/v1/contacts/1") { c, r -> c.updateContact(1, ContactRecordInput(), r) },
        Write("deleteContact", "DELETE", "/api/v1/contacts/1") { c, r -> c.deleteContact(1, r) },
        Write("updateNote", "PUT", "/api/v1/notes/1") { c, r -> c.updateNote(1, NoteInput(content = "x"), r) },
        Write("deleteNote", "DELETE", "/api/v1/notes/1") { c, r -> c.deleteNote(1, r) },
        Write("updateActivity", "PUT", "/api/v1/activities/1") { c, r -> c.updateActivity(1, ActivityInput(title = "x"), r) },
        Write("deleteActivity", "DELETE", "/api/v1/activities/1") { c, r -> c.deleteActivity(1, r) },
        Write("updateLifeEvent", "PUT", "/api/v1/life-events/le1") { c, r -> c.updateLifeEvent("le1", LifeEventInput(entityId = "e"), r) },
        Write("deleteLifeEvent", "DELETE", "/api/v1/life-events/le1") { c, r -> c.deleteLifeEvent("le1", r) },
        Write("updateReminder", "PUT", "/api/v1/reminders/1") { c, r -> c.updateReminder(1, Reminder(message = "x"), r) },
        Write("deleteReminder", "DELETE", "/api/v1/reminders/1") { c, r -> c.deleteReminder(1, r) },
    )

    @Test
    fun `every update and delete sends the revision as a quoted If-Match`() = runBlocking {
        for (w in writes) {
            server.enqueue(MockResponse().setResponseCode(200).setBody(okBody))
            w.call(client, 7)
            val request = server.takeRequest()
            assertEquals(w.name, w.method, request.method)
            assertEquals(w.name, w.path, request.path)
            assertEquals(w.name, "\"7\"", request.getHeader("If-Match"))
        }
    }

    @Test
    fun `a null revision omits the If-Match header`() = runBlocking {
        for (w in writes) {
            server.enqueue(MockResponse().setResponseCode(200).setBody(okBody))
            w.call(client, null)
            assertNull(w.name, server.takeRequest().getHeader("If-Match"))
        }
    }

    @Test
    fun `a 412 maps to PreconditionFailed carrying the server message`() = runBlocking {
        for (w in writes) {
            server.enqueue(
                MockResponse().setResponseCode(412).setBody(
                    """{"error":{"code":"PRECONDITION_FAILED","message":"stale revision","details":{"expected_revision":8}}}""",
                ),
            )
            val error = w.call(client, 7).exceptionOrNull()
            assertTrue(w.name, error is ApiError.PreconditionFailed)
            assertEquals(w.name, "stale revision", (error as ApiError.PreconditionFailed).body)
            server.takeRequest()
        }
    }

    @Test
    fun `other 4xx statuses stay a generic client error`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(409).setBody("""{"error":{"code":"CONFLICT","message":"dup"}}"""))
        val error = client.deleteNote(1, 7).exceptionOrNull()
        assertTrue(error is ApiError.Client)
        assertEquals(409, (error as ApiError.Client).code)
    }

    @Test
    fun `reminder revision rides in the header and never the request body`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(200).setBody(okBody))
        client.updateReminder(1, Reminder(message = "x", revision = 7), 7)
        val request = server.takeRequest()
        assertEquals("\"7\"", request.getHeader("If-Match"))
        assertFalse(request.body.readUtf8().contains("revision"))
    }

    @Test
    fun `revision parses off the response DTOs`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"id":1,"revision":9}"""))
        assertEquals(9, client.updateContact(1, ContactRecordInput()).getOrThrow().revision)
        server.takeRequest()
        server.enqueue(MockResponse().setResponseCode(200).setBody(okBody))
        assertEquals(9, client.updateNote(1, NoteInput(content = "x")).getOrThrow().revision)
        server.takeRequest()
        server.enqueue(MockResponse().setResponseCode(200).setBody(okBody))
        assertEquals(9, client.updateActivity(1, ActivityInput(title = "x")).getOrThrow().revision)
        server.takeRequest()
        server.enqueue(MockResponse().setResponseCode(200).setBody(okBody))
        assertEquals(9, client.updateReminder(1, Reminder(message = "x")).getOrThrow().revision)
        server.takeRequest()
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"id":"le1","revision":9}"""))
        assertEquals(9, client.updateLifeEvent("le1", LifeEventInput(entityId = "e")).getOrThrow().revision)
    }
}
