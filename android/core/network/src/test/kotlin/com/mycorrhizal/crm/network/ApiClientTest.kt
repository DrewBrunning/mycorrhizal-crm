package com.mycorrhizal.crm.network

import com.mycorrhizal.crm.model.network.ActivityInput
import com.mycorrhizal.crm.model.network.CRMEnvelope
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordInput
import com.mycorrhizal.crm.model.network.LoginResponse
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.model.network.NoteInput
import com.mycorrhizal.crm.model.network.Reminder
import com.mycorrhizal.crm.model.network.ReminderRecurrence
import com.squareup.moshi.Moshi
import com.squareup.moshi.kotlin.reflect.KotlinJsonAdapterFactory
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import kotlinx.coroutines.runBlocking

class ApiClientTest {

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
        val moshi = Moshi.Builder()
            .add(KotlinJsonAdapterFactory())
            .build()
        client = ApiClient(okHttp, moshi)
    }

    @After
    fun teardown() {
        server.shutdown()
    }

    @Test
    fun `login captures the auth_token cookie and parses preferences`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Set-Cookie", "auth_token=eyJhbGciOiJIUzI1NiJ9.abc; Path=/; HttpOnly")
                .setBody("""{"language":"en","date_format":"eu"}"""),
        )

        val result = client.login("alice", "secret")

        assertTrue(result.isSuccess)
        val login = result.getOrThrow()
        assertEquals("eyJhbGciOiJIUzI1NiJ9.abc", login.token)
        assertEquals("en", login.language)
        assertEquals("eu", login.dateFormat)

        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/api/v1/login", request.path)
    }

    @Test
    fun `login returns null token when no auth cookie is set`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody("""{"language":"en","date_format":"eu"}"""),
        )

        val result = client.login("alice", "secret")

        assertTrue(result.isSuccess)
        assertEquals(null, result.getOrThrow().token)
    }

    @Test
    fun `login with bad credentials maps to Client 401`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(401)
                .setBody("""{"error":{"code":"invalid_credentials","message":"Invalid credentials"}}"""),
        )

        val result = client.login("alice", "wrong")

        assertTrue(result.isFailure)
        val error = result.exceptionOrNull() as ApiError
        assertTrue(error is ApiError.Client)
        assertEquals(401, (error as ApiError.Client).code)
    }

    @Test
    fun `list contacts parses the cursor page`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """
                    {
                      "contacts": [
                        {"id": 1, "uid": "u1", "fn": "Alice Smith", "firstname": "Alice", "lastname": "Smith", "primary_email": "a@x.com"},
                        {"id": 2, "uid": "u2", "fn": "Bob Jones"}
                      ],
                      "next_cursor": "cursor-2",
                      "limit": 50,
                      "sync": {"mode": "incremental", "incremental": ["contacts"]}
                    }
                    """.trimIndent(),
                ),
        )

        val result = client.listContacts()

        assertTrue(result.isSuccess)
        val page = result.getOrThrow()
        assertEquals(2, page.contacts.size)
        assertEquals("Alice Smith", page.contacts[0].fn)
        assertEquals("cursor-2", page.nextCursor)
        assertEquals("incremental", page.sync?.mode)
    }

    @Test
    fun `list contacts sends query parameters`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody("""{"contacts":[],"next_cursor":""}"""),
        )

        client.listContacts(cursor = "c1", limit = 25, search = "ali")

        val request = server.takeRequest()
        assertEquals("/api/v1/contacts?cursor=c1&limit=25&search=ali", request.path)
    }

    @Test
    fun `get contact parses the full record`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """
                    {
                      "id": 7,
                      "uid": "u7",
                      "etag": "abc",
                      "card": {
                        "name": {"full": "Carol King", "components": [{"kind": "given", "value": "Carol"}]},
                        "emails": [{"address": "carol@x.com", "contexts": ["work"]}],
                        "phones": [{"number": "+1-555-0100", "features": ["cell"]}]
                      },
                      "crm": {"circles": ["friends"]},
                      "archived": false
                    }
                    """.trimIndent(),
                ),
        )

        val result = client.getContact(7)

        assertTrue(result.isSuccess)
        val record = result.getOrThrow()
        assertEquals(7, record.id)
        assertEquals("Carol King", record.card?.name?.full)
        assertEquals("carol@x.com", record.card?.emails?.firstOrNull()?.address)
        assertEquals(listOf("friends"), record.crm?.circles)

        val request = server.takeRequest()
        assertEquals("/api/v1/contacts/7", request.path)
    }

    @Test
    fun `current user parses profile`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """{"id": 3, "username": "alice", "email": "a@x.com", "is_admin": true, "language": "de"}""",
                ),
        )

        val result = client.currentUser()

        assertTrue(result.isSuccess)
        val profile = result.getOrThrow()
        assertEquals(3, profile.id)
        assertEquals("alice", profile.username)
        assertTrue(profile.isAdmin)
        assertEquals("de", profile.language)
    }

    @Test
    fun `404 maps to Client error`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(404)
                .setBody("""{"error":{"code":"not_found","message":"Contact not found"}}"""),
        )

        val result = client.getContact(999)

        assertTrue(result.isFailure)
        val error = result.exceptionOrNull() as ApiError
        assertTrue(error is ApiError.Client)
        assertEquals(404, (error as ApiError.Client).code)
        assertEquals("Contact not found", error.body)
    }

    @Test
    fun `malformed json maps to Parse error`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody("""{"contacts": [}"""),
        )

        val result = client.listContacts()

        assertTrue(result.isFailure)
        val error = result.exceptionOrNull()
        assertNotNull(error)
    }

    @Test
    fun `create contact sends a POST body and unwraps the wrapped response`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(201)
                .setBody(
                    """
                    {
                      "message": "Contact created successfully",
                      "contact": {
                        "id": 9,
                        "uid": "u9",
                        "card": {"name": {"full": "Carol King"}},
                        "crm": {"circles": ["friends"]}
                      }
                    }
                    """.trimIndent(),
                ),
        )
        val input = ContactRecordInput(
            card = Card(name = Name(full = "Carol King")),
            crm = CRMEnvelope(circles = listOf("friends")),
        )

        val result = client.createContact(input)

        assertTrue(result.isSuccess)
        // §2.6: the POST wrapper is unwrapped; callers get the bare record.
        assertEquals(9, result.getOrThrow().id)
        assertEquals("Carol King", result.getOrThrow().card?.name?.full)

        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/api/v1/contacts", request.path)
        // Body carries the neutral Card; check for the given-name component.
        assertTrue(request.body.readUtf8().contains("Carol"))
    }

    @Test
    fun `create contact with a 400 validation error maps to Client error`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(400)
                .setBody(
                    """{"error":{"code":"validation_failed","message":"at least one name component (kind=given) or name.full is required"}}""",
                ),
        )

        val result = client.createContact(ContactRecordInput(card = Card()))

        assertTrue(result.isFailure)
        val error = result.exceptionOrNull() as ApiError
        assertTrue(error is ApiError.Client)
        assertEquals(400, (error as ApiError.Client).code)
    }

    @Test
    fun `update contact sends a PUT to the contact path`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """
                    {
                      "id": 9,
                      "uid": "u9",
                      "card": {"name": {"full": "Carol King Renamed"}}
                    }
                    """.trimIndent(),
                ),
        )
        val input = ContactRecordInput(card = Card(name = Name(full = "Carol King Renamed")))

        val result = client.updateContact(9, input)

        assertTrue(result.isSuccess)
        assertEquals("Carol King Renamed", result.getOrThrow().card?.name?.full)

        val request = server.takeRequest()
        assertEquals("PUT", request.method)
        assertEquals("/api/v1/contacts/9", request.path)
    }

    @Test
    fun `list contact activities parses the bare activities array`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """
                    {
                      "activities": [
                        {"ID": 1, "title": "Coffee with Dana", "type": "visit", "date": "2026-08-01T10:00:00Z"},
                        {"ID": 2, "title": "Phone call", "type": "call", "date": "2026-08-02T11:00:00Z"}
                      ]
                    }
                    """.trimIndent(),
                ),
        )

        val result = client.listContactActivities(5)

        assertTrue(result.isSuccess)
        val activities = result.getOrThrow().activities
        assertEquals(2, activities.size)
        assertEquals(1, activities[0].id)
        assertEquals("Coffee with Dana", activities[0].title)
        assertEquals("call", activities[1].type)

        val request = server.takeRequest()
        assertEquals("/api/v1/contacts/5/activities", request.path)
    }

    @Test
    fun `create activity posts and unwraps the wrapped response`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """
                    {
                      "message": "Activity created successfully",
                      "activity": {"ID": 7, "title": "Lunch", "type": "meal"}
                    }
                    """.trimIndent(),
                ),
        )

        val result = client.createActivity(ActivityInput(title = "Lunch", type = "meal", contactIds = listOf(5)))

        assertTrue(result.isSuccess)
        assertEquals(7, result.getOrThrow().id)
        assertEquals("Lunch", result.getOrThrow().title)

        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/api/v1/activities", request.path)
        assertTrue(request.body.readUtf8().contains("5"))
    }

    @Test
    fun `update activity sends a PUT and parses the raw activity`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """{"ID": 7, "title": "Lunch and coffee", "type": "meal"}""",
                ),
        )

        val result = client.updateActivity(7, ActivityInput(title = "Lunch and coffee"))

        assertTrue(result.isSuccess)
        assertEquals("Lunch and coffee", result.getOrThrow().title)

        val request = server.takeRequest()
        assertEquals("PUT", request.method)
        assertEquals("/api/v1/activities/7", request.path)
    }

    @Test
    fun `get activity parses a single activity with participants`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """
                    {
                      "ID": 7,
                      "title": "Lunch",
                      "type": "meal",
                      "contacts": [{"ID": 5, "firstname": "Dana"}, {"ID": 9, "firstname": "Carol"}],
                      "external_ref": "cal-event-42"
                    }
                    """.trimIndent(),
                ),
        )

        val result = client.getActivity(7)

        assertTrue(result.isSuccess)
        val activity = result.getOrThrow()
        assertEquals(7, activity.id)
        assertEquals(listOf(5, 9), activity.contacts?.mapNotNull { it.id.takeIf { id -> id != 0 } })
        assertEquals("cal-event-42", activity.externalRef)

        val request = server.takeRequest()
        assertEquals("/api/v1/activities/7", request.path)
    }

    @Test
    fun `list activities parses the cursor page`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """
                    {
                      "activities": [{"ID": 1, "title": "Visit"}],
                      "next_cursor": "cursor-9",
                      "limit": 25
                    }
                    """.trimIndent(),
                ),
        )

        val result = client.listActivities(cursor = "cursor-9", limit = 25)

        assertTrue(result.isSuccess)
        val page = result.getOrThrow()
        assertEquals(1, page.activities.size)
        assertEquals("cursor-9", page.nextCursor)
    }

    @Test
    fun `list contact notes parses the wrapped notes array`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """
                    {
                      "notes": [
                        {"ID": 3, "content": "Loves climbing", "date": "2026-08-01T10:00:00Z"},
                        {"ID": 4, "content": "Met at conference", "date": "2026-08-02T11:00:00Z"}
                      ]
                    }
                    """.trimIndent(),
                ),
        )

        val result = client.listContactNotes(5)

        assertTrue(result.isSuccess)
        val notes = result.getOrThrow().notes
        assertEquals(2, notes.size)
        assertEquals(3, notes[0].id)
        assertEquals("Loves climbing", notes[0].content)

        val request = server.takeRequest()
        assertEquals("/api/v1/contacts/5/notes", request.path)
    }

    @Test
    fun `create note posts to the contact path and unwraps`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """
                    {
                      "message": "Note created successfully",
                      "note": {"ID": 3, "content": "Loves climbing"}
                    }
                    """.trimIndent(),
                ),
        )

        val result = client.createNote(5, NoteInput(content = "Loves climbing"))

        assertTrue(result.isSuccess)
        assertEquals(3, result.getOrThrow().id)
        assertEquals("Loves climbing", result.getOrThrow().content)

        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/api/v1/contacts/5/notes", request.path)
    }

    @Test
    fun `update note sends a PUT and unwraps the wrapped response`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """{"message": "Note updated successfully", "note": {"ID": 3, "content": "Loves rock climbing"}}""",
                ),
        )

        val result = client.updateNote(3, NoteInput(content = "Loves rock climbing"))

        assertTrue(result.isSuccess)
        assertEquals(3, result.getOrThrow().id)
        assertEquals("Loves rock climbing", result.getOrThrow().content)

        val request = server.takeRequest()
        assertEquals("PUT", request.method)
        assertEquals("/api/v1/notes/3", request.path)
    }

    @Test
    fun `get note parses a single note`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """{"ID": 3, "content": "Loves climbing", "date": "2026-08-01T10:00:00Z"}""",
                ),
        )

        val result = client.getNote(3)

        assertTrue(result.isSuccess)
        assertEquals("Loves climbing", result.getOrThrow().content)
    }

    @Test
    fun `list contact reminders parses the wrapped reminders array`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """
                    {
                      "reminders": [
                        {"ID": 1, "message": "Call Dana", "remind_at": "2026-08-10T14:00:00Z", "recurrence": "weekly"},
                        {"ID": 2, "message": "Birthday gift", "remind_at": "2026-08-15T09:00:00Z", "recurrence": "yearly"}
                      ]
                    }
                    """.trimIndent(),
                ),
        )

        val result = client.listContactReminders(5)

        assertTrue(result.isSuccess)
        val reminders = result.getOrThrow().reminders
        assertEquals(2, reminders.size)
        assertEquals("Call Dana", reminders[0].message)
        assertEquals("weekly", reminders[0].recurrence)

        val request = server.takeRequest()
        assertEquals("/api/v1/contacts/5/reminders", request.path)
    }

    @Test
    fun `create reminder posts and unwraps the wrapped response`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """
                    {
                      "message": "Reminder created successfully",
                      "reminder": {"ID": 1, "message": "Call Dana", "recurrence": "once"}
                    }
                    """.trimIndent(),
                ),
        )
        val reminder = Reminder(
            message = "Call Dana",
            remindAt = "2026-08-10T14:00:00Z",
            recurrence = ReminderRecurrence.ONCE,
            contactId = 5,
        )

        val result = client.createReminder(5, reminder)

        assertTrue(result.isSuccess)
        assertEquals(1, result.getOrThrow().id)

        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/api/v1/contacts/5/reminders", request.path)
    }

    @Test
    fun `complete reminder parses the complete response`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """{"message": "Reminder completed", "reminder": {"ID": 1, "message": "Call Dana", "completed": false}}""",
                ),
        )

        val result = client.completeReminder(1)

        assertTrue(result.isSuccess)
        assertEquals(1, result.getOrThrow().reminder?.id)

        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/api/v1/reminders/1/complete", request.path)
    }

    @Test
    fun `update reminder sends a PUT and unwraps the wrapped response`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """{"message": "Reminder updated successfully", "reminder": {"ID": 1, "message": "Call Dana weekly", "recurrence": "weekly"}}""",
                ),
        )

        val result = client.updateReminder(
            1,
            Reminder(message = "Call Dana weekly", recurrence = ReminderRecurrence.WEEKLY),
        )

        assertTrue(result.isSuccess)
        assertEquals(1, result.getOrThrow().id)
        assertEquals("Call Dana weekly", result.getOrThrow().message)

        val request = server.takeRequest()
        assertEquals("PUT", request.method)
        assertEquals("/api/v1/reminders/1", request.path)
    }

    @Test
    fun `get reminder parses a single reminder`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """{"ID": 1, "message": "Call Dana", "recurrence": "weekly"}""",
                ),
        )

        val result = client.getReminder(1)

        assertTrue(result.isSuccess)
        assertEquals("Call Dana", result.getOrThrow().message)
    }
}
