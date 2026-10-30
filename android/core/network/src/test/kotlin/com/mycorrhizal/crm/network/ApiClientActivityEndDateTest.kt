package com.mycorrhizal.crm.network

import com.mycorrhizal.crm.model.MoshiProvider
import com.mycorrhizal.crm.model.network.ActivityInput
import kotlinx.coroutines.runBlocking
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/** Issue #2006: the wire bodies of an activity write carrying the optional `end_date`. */
class ApiClientActivityEndDateTest {

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

    @Test
    fun `PUT sends end_date as null so clearing the field clears the stored end`() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"ID":7,"title":"Trip"}"""))

        client.updateActivity(7, ActivityInput(title = "Trip", endDate = null))

        val request = server.takeRequest()
        assertEquals("PUT", request.method)
        val body = request.body.readUtf8()
        assertTrue(body, body.contains(""""end_date":null"""))
    }

    @Test
    fun `PUT sends a picked end_date as the calendar-day instant`() = runBlocking {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"ID":7,"title":"Trip","end_date":"2026-05-15T00:00:00Z"}"""
            )
        )

        val result = client.updateActivity(
            7,
            ActivityInput(title = "Trip", endDate = "2026-05-15T00:00:00Z")
        )

        val body = server.takeRequest().body.readUtf8()
        assertTrue(body, body.contains(""""end_date":"2026-05-15T00:00:00Z""""))
        assertEquals("2026-05-15T00:00:00Z", result.getOrThrow().endDate)
    }

    @Test
    fun `POST with no end carries a null end_date, which the server reads as no end`() =
        runBlocking {
            server.enqueue(
                MockResponse().setResponseCode(200)
                    .setBody("""{"message":"ok","activity":{"ID":9,"title":"Lunch"}}""")
            )

            val result = client.createActivity(ActivityInput(title = "Lunch"))

            assertTrue(result.isSuccess)
            val body = server.takeRequest().body.readUtf8()
            assertTrue(body, body.contains(""""end_date":null"""))
        }
}
