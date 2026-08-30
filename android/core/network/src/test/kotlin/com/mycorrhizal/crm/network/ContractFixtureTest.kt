package com.mycorrhizal.crm.network

import com.mycorrhizal.crm.model.MoshiProvider
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import kotlinx.coroutines.runBlocking

/**
 * Issues #257 + #266 — pins Android's real Moshi parsing against the backend's
 * documented response contract. Fixtures live in /testdata/contract-fixtures/
 * (repo root; see its README) and are wired onto this module's test classpath
 * by the `resources.srcDirs` entry in build.gradle.kts, so this is the same
 * file web's contractFixtures.test.ts reads, not a copy. The fixtures are
 * GENERATED from backend/openapi.yaml's response examples by
 * `go run ./cmd/gencontract` (the drift test backend/contract_fixtures_test.go
 * enforces regeneration when the spec changes), so these assertions pin the
 * spec-derived contract rather than a hand-captured server response.
 *
 * Deliberately uses [MoshiProvider.get] -- the production singleton with
 * codegen adapters -- rather than a locally rebuilt `Moshi.Builder()` (the
 * pattern [ApiClientTest] uses for its own hand-written response bodies).
 * A rebuilt Moshi never exercises the app's actual generated adapters; this
 * suite's whole point is pinning those, so it uses the real one.
 *
 * Scope: only `dashboard.json` and `contacts-list.json`. `contact-detail.json`
 * is web-only -- [ApiClient] has no method for GET /contacts/:id/detail yet
 * (only [ApiClient.getContact] for the plain, non-composite
 * ContactRecordResponse). Add Android coverage for that fixture once Android
 * grows a client method for the composite endpoint.
 */
class ContractFixtureTest {

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
        client = ApiClient(okHttp, MoshiProvider.get())
    }

    @After
    fun teardown() {
        server.shutdown()
    }

    @Test
    fun `getDashboard parses the spec-derived composite fixture`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(200).setBody(readFixture("dashboard.json")))

        val result = client.getDashboard()

        assertTrue("expected success, got $result", result.isSuccess)
        val dashboard = result.getOrThrow()

        // The fixture's one upcoming_reminders entry is the fixture contact's
        // reminder, with its contact_name enrichment embedded server-side.
        assertEquals(1, dashboard.upcomingReminders.size)
        assertEquals("Fix Primary", dashboard.upcomingReminders[0].contactName)
        // Empty blocks in the fixture must still parse to [], never null
        // (DashboardResponse's `= emptyList()` default only fires on an
        // *absent* key -- a real Moshi bug here would surface as this list
        // being null and the `List<...>` type crashing elsewhere).
        assertTrue(dashboard.birthdays.isEmpty())
        assertTrue(dashboard.favorites.isEmpty())
    }

    @Test
    fun `listContacts parses the spec-derived list fixture`() = runBlocking {
        val raw = readFixture("contacts-list.json")
        server.enqueue(MockResponse().setResponseCode(200).setBody(raw))

        val result = client.listContacts()

        assertTrue("expected success, got $result", result.isSuccess)
        val page = result.getOrThrow()

        assertTrue(page.contacts.isNotEmpty())
        // archived/is_favorite are documented never-omitempty: every row must
        // carry them as real booleans. A typed parse alone cannot prove this
        // (an absent key silently falls back to the `= false` default), so
        // pin presence on the RAW JSON -- the same trap-8 distinction web's
        // contractFixtures.test.ts asserts.
        val fixturePrimary = rawRoot(raw).let { root ->
            (root["contacts"] as List<*>).first { (it as Map<*, *>)["uid"] == FIXTURE_PRIMARY_UID }
        } as Map<*, *>
        assertTrue("archived must be present on the wire", fixturePrimary.containsKey("archived"))
        assertTrue("is_favorite must be present on the wire", fixturePrimary.containsKey("is_favorite"))

        val primary = page.contacts.find { it.firstname == "Fixture" && it.lastname == "Primary" }
        assertNotNull("fixture contact should be in the fixture", primary)
        assertEquals("1990-06-15", primary!!.birthday)
        assertEquals("fixture.primary@example.com", primary.primaryEmail)
        // The list query doesn't select `circles` -- absent on the wire, and
        // this pins it stays null through Moshi rather than throwing.
        assertNull(primary.circles)
    }

    // MAINT-02 (issue #491): the Android client must tolerate unknown response
    // fields. Adding a response field is the canonical additive (non-breaking)
    // change, and a client that rejects unknown fields silently converts every
    // additive change into a breaking one -- so this property is a client-side
    // requirement, asserted here. Moshi's default is to ignore unknown fields
    // (no `failOnUnknown`), and these tests pin that: a fixture with a
    // not-yet-existent field injected at the top level and on a contact must
    // still parse, and the known fields must survive. It deliberately uses
    // [MoshiProvider.get], the same production singleton as the other tests in
    // this file, because a locally rebuilt Moshi could silently differ in
    // unknown-field handling (e.g. by setting failOnUnknown).
    @Test
    fun `listContacts tolerates unknown response fields`() = runBlocking {
        val raw = readFixture("contacts-list.json")
        val withUnknown = injectUnknownField(raw)
        server.enqueue(MockResponse().setResponseCode(200).setBody(withUnknown))

        val result = client.listContacts()

        assertTrue("unknown fields must not fail parsing, got $result", result.isSuccess)
        val page = result.getOrThrow()

        assertTrue(page.contacts.isNotEmpty())
        val primary = page.contacts.find { it.firstname == "Fixture" && it.lastname == "Primary" }
        assertNotNull("known contact must survive unknown fields", primary)
        assertEquals("1990-06-15", primary!!.birthday)
        assertEquals("fixture.primary@example.com", primary.primaryEmail)
    }

    @Test
    fun `getDashboard tolerates unknown response fields`() = runBlocking {
        val raw = readFixture("dashboard.json")
        server.enqueue(MockResponse().setResponseCode(200).setBody(injectUnknownField(raw)))

        val result = client.getDashboard()

        assertTrue("unknown fields must not fail parsing, got $result", result.isSuccess)
        val dashboard = result.getOrThrow()
        assertEquals(1, dashboard.upcomingReminders.size)
        assertEquals("Fix Primary", dashboard.upcomingReminders[0].contactName)
        assertTrue(dashboard.birthdays.isEmpty())
    }

    private fun readFixture(name: String): String =
        checkNotNull(javaClass.classLoader?.getResourceAsStream(name)) {
            "missing test resource $name -- check build.gradle.kts's resources.srcDirs"
        }.bufferedReader().readText()

    /**
     * Parses a fixture's raw JSON into generic Map/List structures (not the
     * typed data classes), so a test can distinguish an *absent* key from one
     * whose value equals the typed default -- the trap-8 distinction.
     */
    private fun rawRoot(raw: String): Map<*, *> =
        MoshiProvider.get().adapter(Any::class.java).fromJson(raw) as Map<*, *>

    /**
     * Injects a not-yet-existent field into a fixture's top-level object (and
     * into every contact row) so the MAINT-02 unknown-field tolerance tests
     * exercise a response shape that is *newer* than this client. Re-serializes
     * through the same Moshi singleton so the injected payload is valid JSON.
     */
    private fun injectUnknownField(raw: String): String {
        val root = rawRoot(raw).toMutableMap()
        root["__maint02_unknown_field__"] = mapOf("nested" to listOf("a", 1, null))
        (root["contacts"] as? List<*>)?.let { contacts ->
            root["contacts"] = contacts.map { contact ->
                (contact as Map<*, *>).toMutableMap().apply {
                    this["__maint02_unknown_field__"] = listOf("x", "y")
                }
            }
        }
        @Suppress("UNCHECKED_CAST")
        val json = MoshiProvider.get().adapter<Any>(Any::class.java).toJson(root as Any)
        return json
    }

    companion object {
        private const val FIXTURE_PRIMARY_UID = "458bc9ba-b9a7-4853-a3f8-d9cd907bbc9f"
    }
}
