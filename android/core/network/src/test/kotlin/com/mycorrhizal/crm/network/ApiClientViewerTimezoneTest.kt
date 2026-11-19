package com.mycorrhizal.crm.network

import com.mycorrhizal.crm.model.MoshiProvider
import java.time.ZoneId
import java.util.TimeZone
import kotlinx.coroutines.runBlocking
import okhttp3.HttpUrl
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Before
import org.junit.Test

/**
 * Issue #2617 / #2679 (backend #2762): the five display reads that decide "today" carry the
 * device's IANA zone as an optional `tz` query parameter, so the server's day boundary matches the
 * device's. An invalid zone is a 400 (`validation_timezone_invalid`), so one is never sent.
 */
class ApiClientViewerTimezoneTest {

    private lateinit var server: MockWebServer
    private lateinit var okHttp: OkHttpClient
    private val originalDefault: TimeZone = TimeZone.getDefault()

    @Before
    fun setup() {
        server = MockWebServer()
        server.start()
        okHttp = OkHttpClient.Builder()
            .addInterceptor(
                BaseUrlInterceptor(BaseUrlProvider { server.url("/").toString().trimEnd('/') })
            )
            .build()
    }

    @After
    fun teardown() {
        TimeZone.setDefault(originalDefault)
        server.shutdown()
    }

    private fun clientIn(zoneId: String) =
        ApiClient(okHttp, MoshiProvider.get(), timeZoneId = { zoneId })

    /** Runs [call] against a server that answers `{}`, returning the request URL it made. */
    private fun urlOf(client: ApiClient, call: suspend (ApiClient) -> Unit): HttpUrl {
        server.enqueue(MockResponse().setResponseCode(200).setBody("{}"))
        runBlocking { call(client) }
        return server.takeRequest().requestUrl!!
    }

    private val reads: List<Pair<String, suspend (ApiClient) -> Unit>> = listOf(
        "/api/v1/dashboard" to { c -> c.getDashboard() },
        "/api/v1/occasions/upcoming" to { c -> c.getUpcomingOccasions(30) },
        "/api/v1/occasion-obligations/gift-shopping-list" to { c -> c.getGiftShoppingList(30) },
        "/api/v1/contacts/birthdays" to { c -> c.listUpcomingBirthdays() },
        "/api/v1/reminders/upcoming" to { c -> c.listUpcomingReminders() }
    )

    @Test
    fun `every day-relative read sends the device zone as tz`() {
        val client = clientIn("Pacific/Auckland")
        for ((path, call) in reads) {
            val url = urlOf(client, call)
            assertEquals(path, url.encodedPath)
            assertEquals("tz on $path", "Pacific/Auckland", url.queryParameter("tz"))
        }
    }

    @Test
    fun `tz rides along with the existing window parameter`() {
        val client = clientIn("America/Chicago")

        val occasions = urlOf(client) { it.getUpcomingOccasions(90) }
        val gifts = urlOf(client) { it.getGiftShoppingList(90) }

        assertEquals("90", occasions.queryParameter("days"))
        assertEquals("America/Chicago", occasions.queryParameter("tz"))
        assertEquals("90", gifts.queryParameter("days"))
        assertEquals("America/Chicago", gifts.queryParameter("tz"))
        assertNull(occasions.queryParameter("include_sensitive"))
    }

    @Test
    fun `the default client reads the current device zone on every request`() {
        val client = ApiClient(okHttp, MoshiProvider.get())

        TimeZone.setDefault(TimeZone.getTimeZone("America/Chicago"))
        assertEquals("America/Chicago", urlOf(client) { it.getDashboard() }.queryParameter("tz"))

        // The user crosses a zone: the next request follows, nothing is cached.
        TimeZone.setDefault(TimeZone.getTimeZone("Asia/Tokyo"))
        assertEquals("Asia/Tokyo", urlOf(client) { it.getDashboard() }.queryParameter("tz"))
        assertEquals(ZoneId.of("Asia/Tokyo"), ZoneId.systemDefault())
    }

    @Test
    fun `a zone the server would reject as invalid is never sent`() {
        val invalid = listOf(
            "",
            "   ",
            "Local",
            "local",
            "Z",
            "GMT+05:00",
            "UTC+01:00",
            "../etc/passwd",
            "/Europe/Berlin",
            "Europe//Berlin",
            "Not/AZone",
            "Europe/" + "B".repeat(70)
        )
        for (zone in invalid) {
            val url = urlOf(clientIn(zone)) { it.getDashboard() }
            assertNull("tz must be omitted for \"$zone\"", url.queryParameter("tz"))
            assertEquals("/api/v1/dashboard", url.encodedPath)
        }
    }

    @Test
    fun `plain IANA names with digits and signs are sent`() {
        for (zone in listOf("UTC", "Etc/GMT+5", "America/Argentina/Buenos_Aires", "Etc/GMT-14")) {
            val url = urlOf(clientIn(zone)) { it.listUpcomingReminders() }
            assertEquals(zone, url.queryParameter("tz"))
        }
    }
}
