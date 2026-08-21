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
 * Issue #257 — pins Android's real Moshi parsing against real captured
 * backend responses. Fixtures live in /testdata/contract-fixtures/ (repo
 * root; see its README) and are wired onto this module's test classpath by
 * the `resources.srcDirs` entry in build.gradle.kts, so this is the same
 * file web's contractFixtures.test.ts reads, not a copy.
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
    fun `getDashboard parses a real composite response`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(200).setBody(readFixture("dashboard.json")))

        val result = client.getDashboard()

        assertTrue("expected success, got $result", result.isSuccess)
        val dashboard = result.getOrThrow()

        // The capture's one upcoming_reminders entry is the fixture contact's
        // reminder, with its contact_name enrichment embedded server-side.
        assertEquals(1, dashboard.upcomingReminders.size)
        assertEquals("Fix Primary", dashboard.upcomingReminders[0].contactName)
        // Empty blocks in the capture must still parse to [], never null
        // (DashboardResponse's `= emptyList()` default only fires on an
        // *absent* key -- a real Moshi bug here would surface as this list
        // being null and the `List<...>` type crashing elsewhere).
        assertTrue(dashboard.birthdays.isEmpty())
        assertTrue(dashboard.favorites.isEmpty())
    }

    @Test
    fun `listContacts parses a real list response`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(200).setBody(readFixture("contacts-list.json")))

        val result = client.listContacts()

        assertTrue("expected success, got $result", result.isSuccess)
        val page = result.getOrThrow()

        assertTrue(page.contacts.isNotEmpty())
        // archived/is_favorite are documented never-omitempty -- every row
        // must have parsed them as real booleans, not fallen back to the
        // data class default because the key was silently missing.
        val primary = page.contacts.find { it.firstname == "Fixture" && it.lastname == "Primary" }
        assertNotNull("fixture contact should be in the capture", primary)
        assertEquals("1990-06-15", primary!!.birthday)
        assertEquals("fixture.primary@example.com", primary.primaryEmail)
        // The list query doesn't select `circles` -- absent on the wire, and
        // this pins it stays null through Moshi rather than throwing.
        assertNull(primary.circles)
    }

    private fun readFixture(name: String): String =
        checkNotNull(javaClass.classLoader?.getResourceAsStream(name)) {
            "missing test resource $name -- check build.gradle.kts's resources.srcDirs"
        }.bufferedReader().readText()
}
