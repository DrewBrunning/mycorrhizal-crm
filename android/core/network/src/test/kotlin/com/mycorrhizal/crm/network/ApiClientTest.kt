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
    fun `list contacts sends the circle filter and archived toggle on the query string`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody("""{"contacts":[],"next_cursor":""}"""),
        )

        client.listContacts(includeArchived = true, circle = "Book club")

        val request = server.takeRequest()
        assertEquals("/api/v1/contacts?include_archived=true&circle=Book%20club", request.path)
    }

    @Test
    fun `list contacts omits a blank circle filter`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody("""{"contacts":[],"next_cursor":""}"""),
        )

        client.listContacts(circle = "   ")

        val request = server.takeRequest()
        assertEquals("/api/v1/contacts", request.path)
    }

    @Test
    fun `list contacts with vcardUids sends repeatable vcard_uid params and skips pagination params`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody("""{"contacts":[],"next_cursor":""}"""),
        )

        client.listContacts(cursor = "c1", limit = 25, search = "ali", vcardUids = listOf("uid-1", "uid-2"))

        val request = server.takeRequest()
        assertEquals("/api/v1/contacts?vcard_uid=uid-1&vcard_uid=uid-2", request.path)
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
    fun `get briefing parses the full prep-view composite`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """
                    {
                      "contact_id": 7,
                      "uid": "u7",
                      "name": "Carol King",
                      "kind": "human",
                      "photo_thumbnail": "data:image/png;base64,abc",
                      "last_activity": {
                        "id": 41,
                        "title": "Coffee",
                        "type": "visit",
                        "description": "Catch-up",
                        "date": "2026-08-10T14:00:00Z"
                      },
                      "recent_notes": [
                        {"ID": 9, "content": "Talks about her garden"}
                      ],
                      "cadence": {
                        "policy": {
                          "id": "pol-1",
                          "entity_id": "u7",
                          "target_interval_days": 30,
                          "qualifying_types": ["visit", "meal"]
                        },
                        "health": {
                          "has_qualifying_interaction": true,
                          "last_interaction": "2026-08-10T14:00:00Z",
                          "next_due": "2026-09-09T14:00:00Z",
                          "overdue_by": 2
                        }
                      },
                      "open_agenda_items": [
                        {"id": "ag-1", "content": "Ask about the surgery"}
                      ],
                      "relationships": [
                        {
                          "edge": {
                            "id": "edge-1",
                            "source_id": "u7",
                            "target_id": "u8",
                            "type": "spouse_of",
                            "directional": false,
                            "status": "confirmed"
                          },
                          "other_party_contact_id": 8,
                          "other_party_name": "Bob Marley",
                          "other_party_uid": "u8",
                          "display_token": "spouse_of"
                        }
                      ],
                      "life_events": [
                        {"id": "le-1", "entity_id": "u7", "type": "graduated", "description": "MSc"}
                      ],
                      "upcoming_reminders": [
                        {"ID": 3, "message": "Send card", "remind_at": "2026-08-20T09:00:00Z"}
                      ],
                      "upcoming_dates": [
                        {"label": "birthday", "date": "--12-25", "days_until": 5}
                      ]
                    }
                    """.trimIndent(),
                ),
        )

        val result = client.getBriefing(7)

        assertTrue(result.isSuccess)
        val briefing = result.getOrThrow()
        assertEquals(7, briefing.contactId)
        assertEquals("Carol King", briefing.name)
        assertEquals("human", briefing.kind)
        assertEquals("data:image/png;base64,abc", briefing.photoThumbnail)

        assertEquals("Coffee", briefing.lastActivity?.title)
        assertEquals("visit", briefing.lastActivity?.type)
        assertEquals(1, briefing.recentNotes.size)
        assertEquals("Talks about her garden", briefing.recentNotes.first().content)

        assertEquals(30, briefing.cadence?.policy?.targetIntervalDays)
        assertEquals(listOf("visit", "meal"), briefing.cadence?.policy?.qualifyingTypes)
        assertTrue(briefing.cadence?.health?.hasQualifyingInteraction ?: false)
        assertEquals(2, briefing.cadence?.health?.overdueBy)

        assertEquals(1, briefing.openAgendaItems.size)
        assertEquals("Ask about the surgery", briefing.openAgendaItems.first().content)

        assertEquals(1, briefing.relationships.size)
        val rel = briefing.relationships.first()
        assertEquals("Bob Marley", rel.otherPartyName)
        assertEquals(8, rel.otherPartyContactId)
        assertEquals("spouse_of", rel.displayToken)
        assertEquals("spouse_of", rel.edge?.type)
        assertEquals("u8", rel.edge?.targetId)

        assertEquals(1, briefing.lifeEvents.size)
        assertEquals("graduated", briefing.lifeEvents.first().type)

        assertEquals(1, briefing.upcomingReminders.size)
        assertEquals("Send card", briefing.upcomingReminders.first().message)

        assertEquals(1, briefing.upcomingDates.size)
        assertEquals("birthday", briefing.upcomingDates.first().label)
        assertEquals(5, briefing.upcomingDates.first().daysUntil)

        val request = server.takeRequest()
        assertEquals("/api/v1/contacts/7/briefing", request.path)
    }

    @Test
    fun `get briefing tolerates every collection key being absent`() = runBlocking {
        // The exact web crash: a contact with no history once returned every
        // collection block ABSENT (not `[]`), and PrepViewPage dereferenced
        // `.length` on them — white-screening the page into the ErrorBoundary.
        // Decoding into the Kotlin model makes absent vs [] indistinguishable,
        // so assert against the raw JSON with the keys omitted, exactly the
        // trap /CLAUDE.md frontend trap #8 describes.
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """
                    {
                      "contact_id": 1,
                      "uid": "u1",
                      "name": "Empty"
                    }
                    """.trimIndent(),
                ),
        )

        val result = client.getBriefing(1)

        assertTrue(result.isSuccess)
        val briefing = result.getOrThrow()
        assertEquals("Empty", briefing.name)
        assertTrue(briefing.recentNotes.isEmpty())
        assertTrue(briefing.openAgendaItems.isEmpty())
        assertTrue(briefing.relationships.isEmpty())
        assertTrue(briefing.lifeEvents.isEmpty())
        assertTrue(briefing.upcomingReminders.isEmpty())
        assertTrue(briefing.upcomingDates.isEmpty())
        assertEquals(null, briefing.cadence)
        assertEquals(null, briefing.lastActivity)
    }

    @Test
    fun `get briefing normalizes an explicit JSON null collection to an empty list`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """
                    {
                      "contact_id": 2,
                      "uid": "u2",
                      "name": "Nulls",
                      "recent_notes": null,
                      "relationships": null,
                      "upcoming_dates": null
                    }
                    """.trimIndent(),
                ),
        )

        val result = client.getBriefing(2)

        assertTrue(result.isSuccess)
        val briefing = result.getOrThrow()
        assertTrue(briefing.recentNotes.isEmpty())
        assertTrue(briefing.relationships.isEmpty())
        assertTrue(briefing.upcomingDates.isEmpty())
    }

    @Test
    fun `get briefing failure maps to the backend error`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(404)
                .setBody("""{"error":{"code":"not_found","message":"Contact not found"}}"""),
        )

        val result = client.getBriefing(999)

        assertTrue(result.isFailure)
        val error = result.exceptionOrNull() as ApiError
        assertTrue(error is ApiError.Client)
        assertEquals(404, (error as ApiError.Client).code)
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
    fun `list contact activities sends search date and cursor params and parses next_cursor`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """{"activities":[{"ID":1,"title":"Coffee with Dana"}],"next_cursor":"cursor-2","limit":25}""",
                ),
        )

        val result = client.listContactActivities(5, cursor = "c1", limit = 25, search = "coffee", fromDate = "2026-08-01", toDate = "2026-08-10")

        assertTrue(result.isSuccess)
        assertEquals("cursor-2", result.getOrThrow().nextCursor)

        val request = server.takeRequest()
        assertEquals(
            "/api/v1/contacts/5/activities?cursor=c1&limit=25&search=coffee&fromDate=2026-08-01&toDate=2026-08-10",
            request.path,
        )
    }

    @Test
    fun `delete activity sends a DELETE and succeeds`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody("""{"message":"Activity deleted"}"""),
        )

        val result = client.deleteActivity(7)

        assertTrue(result.isSuccess)
        val request = server.takeRequest()
        assertEquals("DELETE", request.method)
        assertEquals("/api/v1/activities/7", request.path)
    }

    @Test
    fun `delete activity surfaces a 404 as a failure`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(404).setBody("""{"error":"Activity not found"}"""),
        )

        val result = client.deleteActivity(999)

        assertTrue(result.isFailure)
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
    fun `list activities with includeContacts appends the include query param`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(200)
                .setBody("""{"activities": [], "next_cursor": "", "limit": 25}"""),
        )

        client.listActivities(includeContacts = true)

        val request = server.takeRequest()
        assertEquals("/api/v1/activities?include=contacts", request.path)
    }

    // Trap-#8 (`/CLAUDE.md`): GetActivities marshals a nil Go slice as JSON `null`, not `[]`, for
    // zero activities — a different failure mode than the absent-key one below. Both must parse.
    @Test
    fun `list activities normalizes an explicit JSON null to an empty list`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(200)
                .setBody("""{"activities": null, "next_cursor": "", "limit": 25}"""),
        )

        val result = client.listActivities()

        assertTrue(result.isSuccess)
        assertEquals(0, result.getOrThrow().activities.size)
    }

    @Test
    fun `list activities tolerates the collection key being fully absent`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"next_cursor": "", "limit": 25}"""))

        val result = client.listActivities()

        assertTrue(result.isSuccess)
        assertEquals(0, result.getOrThrow().activities.size)
    }

    // M9: the Notes drawer inbox — GET /notes, the N4 unfiled-notes queue (ticket's test case 3).
    @Test
    fun `list notes parses the cursor page and its total`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """
                {
                  "notes": [{"ID": 3, "content": "Buy milk", "date": "2026-08-01T10:00:00Z"}],
                  "next_cursor": "cursor-2",
                  "limit": 25,
                  "total": 9
                }
                """.trimIndent(),
            ),
        )

        val result = client.listNotes(cursor = "cursor-1", limit = 25)

        assertTrue(result.isSuccess)
        val page = result.getOrThrow()
        assertEquals(1, page.notes.size)
        assertEquals("Buy milk", page.notes[0].content)
        assertEquals("cursor-2", page.nextCursor)
        assertEquals(9, page.total)

        val request = server.takeRequest()
        assertEquals("/api/v1/notes?cursor=cursor-1&limit=25", request.path)
    }

    // Trap-#8 (`/CLAUDE.md`): GetUnassignedNotes marshals a nil Go slice as JSON `null`, not
    // `[]`, for zero unfiled notes — a different failure mode than the absent-key one below.
    @Test
    fun `list notes normalizes an explicit JSON null to an empty list`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(200)
                .setBody("""{"notes": null, "next_cursor": "", "limit": 25, "total": 0}"""),
        )

        val result = client.listNotes()

        assertTrue(result.isSuccess)
        assertEquals(0, result.getOrThrow().notes.size)
    }

    @Test
    fun `list notes tolerates the collection key being fully absent`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"total": 0}"""))

        val result = client.listNotes()

        assertTrue(result.isSuccess)
        assertEquals(0, result.getOrThrow().notes.size)
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
    fun `list contact notes sends search date and cursor params and parses next_cursor`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """{"notes":[{"ID":3,"content":"Loves climbing"}],"next_cursor":"cursor-2","limit":25}""",
                ),
        )

        val result = client.listContactNotes(5, cursor = "c1", limit = 25, search = "climb", fromDate = "2026-08-01", toDate = "2026-08-10")

        assertTrue(result.isSuccess)
        assertEquals("cursor-2", result.getOrThrow().nextCursor)

        val request = server.takeRequest()
        assertEquals(
            "/api/v1/contacts/5/notes?cursor=c1&limit=25&search=climb&fromDate=2026-08-01&toDate=2026-08-10",
            request.path,
        )
    }

    @Test
    fun `list contact notes omits blank filters`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody("""{"notes":[],"next_cursor":""}"""),
        )

        client.listContactNotes(5, search = "   ")

        val request = server.takeRequest()
        assertEquals("/api/v1/contacts/5/notes", request.path)
    }

    @Test
    fun `delete note sends a DELETE and succeeds`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody("""{"message":"Note deleted"}"""),
        )

        val result = client.deleteNote(3)

        assertTrue(result.isSuccess)
        val request = server.takeRequest()
        assertEquals("DELETE", request.method)
        assertEquals("/api/v1/notes/3", request.path)
    }

    @Test
    fun `delete note surfaces a 404 as a failure`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(404).setBody("""{"error":"Note not found"}"""),
        )

        val result = client.deleteNote(999)

        assertTrue(result.isFailure)
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
    fun `create unassigned note posts to the notes path`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """{"message": "Note created successfully", "note": {"ID": 3, "content": "Loves climbing"}}""",
                ),
        )

        val result = client.createUnassignedNote(NoteInput(content = "Loves climbing"))

        assertTrue(result.isSuccess)
        assertEquals(3, result.getOrThrow().id)

        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/api/v1/notes", request.path)
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
    fun `complete reminder with skip true appends the skip query param`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody("""{"message": "Reminder skipped", "reminder": {"ID": 1, "message": "Call Dana", "completed": false}}"""),
        )

        val result = client.completeReminder(1, skip = true)

        assertTrue(result.isSuccess)
        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/api/v1/reminders/1/complete?skip=true", request.path)
    }

    @Test
    fun `get dashboard parses all four widgets from one composite response`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """
                    {
                      "birthdays": [
                        {"type": "contact", "name": "Alice", "birthday": "--12-25", "photo_thumbnail": "data:image/png;base64,abc", "contact_id": 1}
                      ],
                      "random_contacts": [
                        {"id": 3, "uid": "u3", "firstname": "Bob", "lastname": "Smith", "nickname": "Bobby", "circles": ["family"], "photo_thumbnail": "data:image/png;base64,def"}
                      ],
                      "upcoming_reminders": [
                        {"ID": 7, "message": "Call Dana", "remind_at": "2026-08-15T09:00:00Z", "recurrence": "weekly", "by_mail": true, "contact_id": 3, "contact_name": "Bobby Smith"}
                      ],
                      "overdue": [
                        {"policy": {"id": "cadence-1", "entity_id": "u3", "target_interval_days": 30}, "health": {"has_qualifying_interaction": true, "next_due": "2026-08-01T00:00:00Z", "overdue_by": 13}, "contact_id": 3, "contact_name": "Bobby Smith", "photo_thumbnail": "data:image/png;base64,ghi"}
                      ]
                    }
                    """.trimIndent(),
                ),
        )

        val result = client.getDashboard()

        assertTrue(result.isSuccess)
        val dashboard = result.getOrThrow()
        // Birthdays.
        assertEquals(1, dashboard.birthdays.size)
        assertEquals("Alice", dashboard.birthdays[0].name)
        assertEquals(1L, dashboard.birthdays[0].contactId)
        // Random contacts.
        assertEquals(1, dashboard.randomContacts.size)
        assertEquals("Bobby", dashboard.randomContacts[0].nickname)
        assertEquals(listOf("family"), dashboard.randomContacts[0].circles)
        // Upcoming reminders — the M3 embedded contact name must survive parsing.
        assertEquals(1, dashboard.upcomingReminders.size)
        assertEquals(7, dashboard.upcomingReminders[0].id)
        assertEquals("Call Dana", dashboard.upcomingReminders[0].message)
        assertEquals("Bobby Smith", dashboard.upcomingReminders[0].contactName)
        assertEquals("weekly", dashboard.upcomingReminders[0].recurrence)
        assertEquals(true, dashboard.upcomingReminders[0].byMail)
        assertEquals(3, dashboard.upcomingReminders[0].contactId)
        // Overdue cadences.
        assertEquals(1, dashboard.overdue.size)
        assertEquals("cadence-1", dashboard.overdue[0].policy?.id)
        assertEquals(13, dashboard.overdue[0].health?.overdueBy)

        val request = server.takeRequest()
        assertEquals("GET", request.method)
        assertEquals("/api/v1/dashboard", request.path)
    }

    @Test
    fun `get dashboard normalizes empty arrays to empty lists`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """
                    {
                      "birthdays": [],
                      "random_contacts": [],
                      "upcoming_reminders": [],
                      "overdue": []
                    }
                    """.trimIndent(),
                ),
        )

        val result = client.getDashboard()

        assertTrue(result.isSuccess)
        val dashboard = result.getOrThrow()
        assertTrue(dashboard.birthdays.isEmpty())
        assertTrue(dashboard.randomContacts.isEmpty())
        assertTrue(dashboard.upcomingReminders.isEmpty())
        assertTrue(dashboard.overdue.isEmpty())
    }

    @Test
    fun `get dashboard tolerates every collection key being absent`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody("""{}"""),
        )

        val result = client.getDashboard()

        assertTrue(result.isSuccess)
        val dashboard = result.getOrThrow()
        assertTrue(dashboard.birthdays.isEmpty())
        assertTrue(dashboard.randomContacts.isEmpty())
        assertTrue(dashboard.upcomingReminders.isEmpty())
        assertTrue(dashboard.overdue.isEmpty())
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

    @Test
    fun `delete reminder sends a DELETE and succeeds`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody("""{"message":"Reminder deleted"}"""),
        )

        val result = client.deleteReminder(7)

        assertTrue(result.isSuccess)
        val request = server.takeRequest()
        assertEquals("DELETE", request.method)
        assertEquals("/api/v1/reminders/7", request.path)
    }

    @Test
    fun `delete reminder surfaces a 404 as a failure`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(404).setBody("""{"error":"Reminder not found"}"""),
        )

        val result = client.deleteReminder(999)

        assertTrue(result.isFailure)
    }

    @Test
    fun `list contact reminder completions parses the wrapped completions array`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """
                {
                  "completions": [
                    {"ID": 1, "contact_id": 5, "message": "Call Dana", "completed_at": "2026-08-12T10:00:00Z"},
                    {"ID": 2, "reminder_id": 3, "contact_id": 5, "message": "Gift", "completed_at": "2026-08-11T09:00:00Z"}
                  ]
                }
                """.trimIndent(),
            ),
        )

        val result = client.listContactReminderCompletions(5)

        assertTrue(result.isSuccess)
        val completions = result.getOrThrow().completions
        assertEquals(2, completions.size)
        assertEquals("Call Dana", completions[0].message)
        assertEquals(1, completions[0].id)
        assertEquals(3, completions[1].reminderId)

        val request = server.takeRequest()
        assertEquals("GET", request.method)
        assertEquals("/api/v1/contacts/5/reminder-completions", request.path)
    }

    @Test
    fun `list contact reminder completions normalizes an empty completions array`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody("""{"completions": []}"""),
        )

        val result = client.listContactReminderCompletions(5)

        assertTrue(result.isSuccess)
        assertTrue(result.getOrThrow().completions.isEmpty())
    }

    @Test
    fun `list contact reminder completions normalizes an explicit JSON null to an empty list`() = runBlocking {
        // The backend's GetCompletionsForContact uses a nil Go slice when a contact has no
        // completions, which gin.H serializes as JSON null, not []. Must not crash (the
        // /CLAUDE.md trap #8 family — a collection field that's absent OR null).
        server.enqueue(
            MockResponse().setResponseCode(200).setBody("""{"completions": null}"""),
        )

        val result = client.listContactReminderCompletions(5)

        assertTrue(result.isSuccess)
        assertTrue(result.getOrThrow().completions.isEmpty())
    }

    @Test
    fun `delete reminder completion sends a DELETE and succeeds`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody("""{"message":"Reminder completion deleted"}"""),
        )

        val result = client.deleteReminderCompletion(7)

        assertTrue(result.isSuccess)
        val request = server.takeRequest()
        assertEquals("DELETE", request.method)
        assertEquals("/api/v1/reminder-completions/7", request.path)
    }

    @Test
    fun `delete reminder completion surfaces a 404 as a failure`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(404).setBody("""{"error":"Reminder completion not found"}"""),
        )

        val result = client.deleteReminderCompletion(999)

        assertTrue(result.isFailure)
    }

    @Test
    fun `search parses notes and activities with snippets and resolved_relation`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """
                    {
                      "query": "mom",
                      "resolved_relation": "parent_of",
                      "contacts": [{"id": 99, "fn": "Should Be Ignored"}],
                      "notes": [
                        {"ID": 1, "content": "called mom", "date": "2026-01-01", "contact_id": 5, "contact_name": "Dana White", "snippet": "called <b>mom</b>"}
                      ],
                      "activities": [
                        {"ID": 2, "title": "Coffee with mom", "date": "2026-01-02", "snippet": "Coffee with <b>mom</b>"}
                      ]
                    }
                    """.trimIndent(),
                ),
        )

        val result = client.search("mom", limit = 5)

        assertTrue(result.isSuccess)
        val search = result.getOrThrow()
        assertEquals("parent_of", search.resolvedRelation)
        assertEquals(1, search.notes.size)
        assertEquals("Dana White", search.notes[0].contactName)
        assertEquals("called <b>mom</b>", search.notes[0].snippet)
        assertEquals(1, search.activities.size)
        assertEquals("Coffee with mom", search.activities[0].title)

        val request = server.takeRequest()
        assertEquals("GET", request.method)
        assertEquals("/api/v1/search?q=mom&limit=5", request.path)
    }

    @Test
    fun `search response's contacts group never reaches the parsed model`() = runBlocking {
        // T87: the contact list is the sole authority for contact results; SearchResult has no
        // `contacts` property to leak them into, regardless of what the server sends.
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody("""{"query": "ali", "contacts": [{"id": 1, "fn": "Alice"}], "notes": [], "activities": []}"""),
        )

        val result = client.search("ali")

        assertTrue(result.isSuccess)
        assertEquals(0, result.getOrThrow().notes.size)
        assertEquals(0, result.getOrThrow().activities.size)
    }

    @Test
    fun `search omits optional params when not provided`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"notes": [], "activities": []}"""))

        client.search("al")

        val request = server.takeRequest()
        assertEquals("/api/v1/search?q=al", request.path)
    }

    @Test
    fun `listFieldDefinitions parses the definitions list`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """
                {
                  "field_definitions": [
                    {"id": "d1", "label": "Coffee order", "key": "coffee_order", "target": "contact",
                     "type": "string", "projection": "internal-only", "sensitivity": "normal",
                     "created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z"}
                  ],
                  "total": 1, "next_cursor": "", "limit": 100
                }
                """.trimIndent(),
            ),
        )

        val result = client.listFieldDefinitions()

        assertTrue(result.isSuccess)
        val defs = result.getOrThrow().definitions
        assertEquals(1, defs.size)
        assertEquals("Coffee order", defs[0].label)
        assertEquals("string", defs[0].type)

        val request = server.takeRequest()
        assertEquals("/api/v1/field-definitions", request.path)
    }

    @Test
    fun `listFieldDefinitions normalizes an explicit JSON null to an empty list`() = runBlocking {
        // T84: gin.H always emits the key, but a nil Go slice serializes as JSON null, not [] —
        // a different failure mode than /CLAUDE.md trap #8's absent-key one. Both must not crash.
        server.enqueue(
            MockResponse().setResponseCode(200)
                .setBody("""{"field_definitions": null, "total": 0, "next_cursor": "", "limit": 100}"""),
        )

        val result = client.listFieldDefinitions()

        assertTrue(result.isSuccess)
        assertEquals(0, result.getOrThrow().definitions.size)
    }

    @Test
    fun `listFieldDefinitions tolerates the collection key being fully absent`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"total": 0}"""))

        val result = client.listFieldDefinitions()

        assertTrue(result.isSuccess)
        assertEquals(0, result.getOrThrow().definitions.size)
    }

    @Test
    fun `listContactFieldValues parses the values list`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"field_values": [{"id": 1, "field_definition_id": "d1", "entity_id": "u1", "value": "Latte"}]}""",
            ),
        )

        val result = client.listContactFieldValues(5)

        assertTrue(result.isSuccess)
        val values = result.getOrThrow().values
        assertEquals(1, values.size)
        assertEquals("Latte", values[0].value)

        val request = server.takeRequest()
        assertEquals("GET", request.method)
        assertEquals("/api/v1/contacts/5/field-values", request.path)
    }

    @Test
    fun `a value written via replaceContactFieldValues round-trips through a subsequent read`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"message": "Field values saved successfully", "field_values": [{"id": 1, "field_definition_id": "d1", "entity_id": "u1", "value": "Latte"}]}""",
            ),
        )
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"field_values": [{"id": 1, "field_definition_id": "d1", "entity_id": "u1", "value": "Latte"}]}""",
            ),
        )

        val writeResult = client.replaceContactFieldValues(
            5,
            com.mycorrhizal.crm.model.network.ContactFieldValuesInput(
                fieldValues = listOf(
                    com.mycorrhizal.crm.model.network.FieldValueInput(fieldDefinitionId = "d1", value = "Latte"),
                ),
            ),
        )
        val readResult = client.listContactFieldValues(5)

        assertTrue(writeResult.isSuccess)
        assertTrue(readResult.isSuccess)
        assertEquals("Latte", readResult.getOrThrow().values.first().value)

        val putRequest = server.takeRequest()
        assertEquals("PUT", putRequest.method)
        assertEquals("/api/v1/contacts/5/field-values", putRequest.path)
        assertTrue(putRequest.body.readUtf8().contains("\"field_definition_id\":\"d1\""))
    }

    @Test
    fun `update relationship edge sends a PUT and parses the raw edge`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """{"id": "e1", "source_id": "u1", "target_id": "u2", "type": "spouse_of", "sensitivity": "private"}""",
                ),
        )

        val result = client.updateRelationshipEdge(
            "e1",
            com.mycorrhizal.crm.model.network.RelationshipEdgeInput(
                sourceId = "u1",
                targetId = "u2",
                type = "spouse_of",
                sensitivity = "private",
            ),
        )

        assertTrue(result.isSuccess)
        assertEquals("spouse_of", result.getOrThrow().type)
        assertEquals("private", result.getOrThrow().sensitivity)

        val request = server.takeRequest()
        assertEquals("PUT", request.method)
        assertEquals("/api/v1/relationship-edges/e1", request.path)
    }

    // --- M24: contact-level actions (delete/archive/unarchive/export) ---

    @Test
    fun `delete contact sends a DELETE to the contact route`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"message":"deleted"}"""))

        val result = client.deleteContact(5)

        assertTrue(result.isSuccess)
        val request = server.takeRequest()
        assertEquals("DELETE", request.method)
        assertEquals("/api/v1/contacts/5", request.path)
    }

    @Test
    fun `delete contact failure maps to the backend error`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(404).setBody("""{"error":{"code":"not_found","message":"Contact not found"}}"""),
        )

        val result = client.deleteContact(5)

        assertTrue(result.isFailure)
        val error = result.exceptionOrNull() as ApiError
        assertTrue(error is ApiError.Client)
        assertEquals(404, (error as ApiError.Client).code)
    }

    @Test
    fun `archive contact posts to the archive route`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody("""{"id":5,"archived":true,"firstname":"Dana","lastname":"White"}"""),
        )

        val result = client.archiveContact(5)

        assertTrue(result.isSuccess)
        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/api/v1/contacts/5/archive", request.path)
    }

    @Test
    fun `unarchive contact posts to the unarchive route`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody("""{"id":5,"archived":false,"firstname":"Dana","lastname":"White"}"""),
        )

        val result = client.unarchiveContact(5)

        assertTrue(result.isSuccess)
        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/api/v1/contacts/5/unarchive", request.path)
    }

    @Test
    fun `export single contact vcard 4 returns the raw file bytes`() = runBlocking {
        val vcf = "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Dana White\r\nUID:u1\r\nEND:VCARD\r\n"
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "text/vcard; charset=utf-8")
                .setBody(vcf),
        )

        val result = client.exportContactVcf("u1")

        assertTrue(result.isSuccess)
        assertEquals(vcf.toByteArray().contentToString(), result.getOrThrow().contentToString())
        val request = server.takeRequest()
        assertEquals("GET", request.method)
        assertEquals("/api/v1/export/vcf?vcard_uid=u1", request.path)
    }

    @Test
    fun `export single contact vcard 3 adds the version param`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "text/vcard; charset=utf-8")
                .setBody("BEGIN:VCARD\r\nVERSION:3.0\r\nEND:VCARD\r\n"),
        )

        val result = client.exportContactVcf("u1", version = 3)

        assertTrue(result.isSuccess)
        val request = server.takeRequest()
        assertEquals("/api/v1/export/vcf?vcard_uid=u1&version=3", request.path)
    }

    @Test
    fun `export failure parses the JSON error instead of returning bytes`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(400).setBody("""{"error":{"code":"validation","message":"bad request"}}"""),
        )

        val result = client.exportContactVcf("missing")

        assertTrue(result.isFailure)
        val error = result.exceptionOrNull() as ApiError
        assertTrue(error is ApiError.Client)
        assertEquals(400, (error as ApiError.Client).code)
    }

    // M9 item 4: confirmVcfImport had zero callers/tests before this ticket wired a VCF import
    // screen to it — same request/response shape as the (also-untested) CSV confirmImport.
    @Test
    fun `confirmVcfImport posts to the vcf confirm path and parses the result`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody("""{"total_processed": 3, "created": 2, "updated": 1, "skipped": 0, "errors": []}"""),
        )

        val result = client.confirmVcfImport(
            com.mycorrhizal.crm.model.network.ImportConfirmRequest(
                sessionId = "session-1",
                actions = listOf(
                    com.mycorrhizal.crm.model.network.RowImportAction(rowIndex = 0, action = "add"),
                    com.mycorrhizal.crm.model.network.RowImportAction(rowIndex = 1, action = "update"),
                ),
            ),
        )

        assertTrue(result.isSuccess)
        assertEquals(2, result.getOrThrow().created)
        assertEquals(1, result.getOrThrow().updated)

        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/api/v1/contacts/import/vcf/confirm", request.path)
        val body = request.body.readUtf8()
        assertTrue(body.contains("session-1"))
        assertTrue(body.contains("\"action\":\"update\""))
    }

    // --- M12: cadence policies (5 new client methods) ---

    @Test
    fun `list cadence policies sends the entity_id filter and parses the wrapped list`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """{"cadence_policies": [
                        {"id": "p1", "entity_id": "u1", "target_interval_days": 30,
                         "qualifying_types": ["call"], "health": {"has_qualifying_interaction": true, "overdue_by": 2}}
                    ], "total": 1, "next_cursor": "", "limit": 25}""",
                ),
        )

        val result = client.listCadencePolicies("u1")

        assertTrue(result.isSuccess)
        val page = result.getOrThrow()
        assertEquals(1, page.cadencePolicies.size)
        assertEquals(30, page.cadencePolicies[0].targetIntervalDays)
        assertEquals(listOf("call"), page.cadencePolicies[0].qualifyingTypes)
        assertEquals(2, page.cadencePolicies[0].health?.overdueBy)

        val request = server.takeRequest()
        assertEquals("/api/v1/cadence-policies?entity_id=u1", request.path)
    }

    @Test
    fun `list cadence policies normalizes an absent policy array to empty`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody("""{"total": 0, "next_cursor": "", "limit": 25}"""),
        )

        val result = client.listCadencePolicies("u1")

        assertTrue(result.isSuccess)
        assertTrue(result.getOrThrow().cadencePolicies.isEmpty())
    }

    @Test
    fun `get cadence policy parses the raw policy with health`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """{"id": "p1", "entity_id": "u1", "target_interval_days": 30,
                        "health": {"has_qualifying_interaction": false, "overdue_by": 0}}""",
                ),
        )

        val result = client.getCadencePolicy("p1")

        assertTrue(result.isSuccess)
        assertEquals("p1", result.getOrThrow().id)
        assertTrue(result.getOrThrow().qualifyingTypes.isEmpty())
        assertTrue(result.getOrThrow().health?.hasQualifyingInteraction == false)

        val request = server.takeRequest()
        assertEquals("/api/v1/cadence-policies/p1", request.path)
    }

    @Test
    fun `create cadence policy posts the input and unwraps the created policy`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(201)
                .setBody(
                    """{"message": "Cadence policy created", "cadence_policy":
                        {"id": "p1", "entity_id": "u1", "target_interval_days": 14, "qualifying_types": []}}""",
                ),
        )

        val result = client.createCadencePolicy(
            com.mycorrhizal.crm.model.network.CadencePolicyInput(entityId = "u1", targetIntervalDays = 14),
        )

        assertTrue(result.isSuccess)
        assertEquals("p1", result.getOrThrow().id)
        assertEquals(14, result.getOrThrow().targetIntervalDays)

        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/api/v1/cadence-policies", request.path)
        val body = request.body.readUtf8()
        assertTrue(body.contains("\"entity_id\":\"u1\""))
        assertTrue(body.contains("\"target_interval_days\":14"))
        // An empty selection is sent as an empty array, never defaulted away.
        assertTrue(body.contains("\"qualifying_types\":[]"))
    }

    @Test
    fun `update cadence policy sends a PUT and parses the raw policy`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """{"id": "p1", "entity_id": "u1", "target_interval_days": 90,
                        "qualifying_types": ["visit"], "health": {"has_qualifying_interaction": true, "overdue_by": 0}}""",
                ),
        )

        val result = client.updateCadencePolicy(
            "p1",
            com.mycorrhizal.crm.model.network.CadencePolicyInput(
                entityId = "u1",
                targetIntervalDays = 90,
                qualifyingTypes = listOf("visit"),
            ),
        )

        assertTrue(result.isSuccess)
        assertEquals(90, result.getOrThrow().targetIntervalDays)
        assertEquals(listOf("visit"), result.getOrThrow().qualifyingTypes)

        val request = server.takeRequest()
        assertEquals("PUT", request.method)
        assertEquals("/api/v1/cadence-policies/p1", request.path)
        assertTrue(request.body.readUtf8().contains("\"qualifying_types\":[\"visit\"]"))
    }

    @Test
    fun `delete cadence policy sends a DELETE to the policy route`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"message": "Cadence policy deleted"}"""))

        val result = client.deleteCadencePolicy("p1")

        assertTrue(result.isSuccess)
        val request = server.takeRequest()
        assertEquals("DELETE", request.method)
        assertEquals("/api/v1/cadence-policies/p1", request.path)
    }

    // --- M22: household relationship & shared-address suggestions ---

    @Test
    fun `suggest household relationships posts an empty body and parses the response`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """
                    {
                      "message": "Relationship suggestions generated",
                      "household_id": "h1",
                      "suggested_edges": [
                        {"id": "e1", "source_id": "u1", "target_id": "u2", "type": "spouse_of", "status": "suggested", "source": "household-inferred", "confidence": 0.8}
                      ],
                      "total": 1
                    }
                    """.trimIndent(),
                ),
        )

        val result = client.suggestHouseholdRelationships("h1")

        assertTrue(result.isSuccess)
        assertEquals("h1", result.getOrThrow().householdId)
        assertEquals(1, result.getOrThrow().suggestedEdges.size)
        assertEquals("spouse_of", result.getOrThrow().suggestedEdges[0].type)
        assertEquals("suggested", result.getOrThrow().suggestedEdges[0].status)

        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/api/v1/households/h1/suggest-relationships", request.path)
        assertEquals(0, request.body.size)
    }

    @Test
    fun `suggest household relationships tolerates an absent suggested_edges key`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """{"message": "Relationship suggestions generated", "household_id": "h1", "total": 0}""",
                ),
        )

        val result = client.suggestHouseholdRelationships("h1")

        assertTrue(result.isSuccess)
        assertTrue(result.getOrThrow().suggestedEdges.isEmpty())
    }

    @Test
    fun `suggest address households parses the suggestions`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """
                    {
                      "suggestions": [
                        {
                          "address_hash": "ah1",
                          "member_hash": "mh1",
                          "member_vcard_uids": ["u1", "u2"],
                          "address": {"components": [{"kind": "locality", "value": "Berlin"}], "full": "1 Main St, Berlin"}
                        }
                      ],
                      "total": 1
                    }
                    """.trimIndent(),
                ),
        )

        val result = client.suggestAddressHouseholds()

        assertTrue(result.isSuccess)
        val suggestions = result.getOrThrow().suggestions
        assertEquals(1, suggestions.size)
        assertEquals(listOf("u1", "u2"), suggestions[0].memberVCardUids)
        assertEquals("Berlin", suggestions[0].address?.components?.get(0)?.value)

        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/api/v1/households/suggest-addresses", request.path)
    }

    @Test
    fun `suggest address households tolerates an absent suggestions key`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody("""{"total": 0}"""),
        )

        val result = client.suggestAddressHouseholds()

        assertTrue(result.isSuccess)
        assertTrue(result.getOrThrow().suggestions.isEmpty())
    }

    @Test
    fun `accept household suggestion posts member uids and unwraps the household`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(201)
                .setBody(
                    """{"household": {"id": "h9", "name": "Alice & Bob", "type": "family_unit"}}""",
                ),
        )

        val input = com.mycorrhizal.crm.model.network.AcceptHouseholdSuggestionInput(
            memberVCardUids = listOf("u1", "u2"),
        )
        val result = client.acceptHouseholdSuggestion(input)

        assertTrue(result.isSuccess)
        assertEquals("h9", result.getOrThrow().id)
        assertEquals("Alice & Bob", result.getOrThrow().name)

        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/api/v1/households/suggestions/accept", request.path)
        val body = request.body.readUtf8()
        assertTrue(body.contains("member_vcard_uids"))
        assertTrue(body.contains("u1"))
        assertTrue(body.contains("u2"))
    }

    @Test
    fun `dismiss household suggestion posts member uids`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody("""{"message": "Household suggestion dismissed"}"""),
        )

        val input = com.mycorrhizal.crm.model.network.DismissHouseholdSuggestionInput(
            memberVCardUids = listOf("u1", "u2"),
        )
        val result = client.dismissHouseholdSuggestion(input)

        assertTrue(result.isSuccess)

        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/api/v1/households/suggestions/dismiss", request.path)
        val body = request.body.readUtf8()
        assertTrue(body.contains("member_vcard_uids"))
        assertTrue(body.contains("u1"))
    }
}
