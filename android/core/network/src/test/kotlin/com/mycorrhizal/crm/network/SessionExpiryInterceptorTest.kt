package com.mycorrhizal.crm.network

import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Test

/**
 * Issue #678: any API response with a 401 from the configured server must
 * signal session expiry so the app can clear the session and land on the auth
 * flow. The interceptor is purely observational — it passes every response
 * through untouched and only signals on a real, same-origin 401 (an external
 * host's 401 must never clear a valid session; the app's OkHttp client doubles
 * as Coil's photo client).
 */
class SessionExpiryInterceptorTest {

    private lateinit var server: MockWebServer

    @Before
    fun setup() {
        server = MockWebServer()
        server.start()
    }

    @After
    fun teardown() {
        server.shutdown()
    }

    private fun client(notifier: SessionExpiryNotifier, baseUrl: String = server.url("/").toString()): OkHttpClient =
        OkHttpClient.Builder()
            .addInterceptor(SessionExpiryInterceptor(notifier, BaseUrlProvider { baseUrl }))
            .build()

    private fun get(client: OkHttpClient) =
        client.newCall(Request.Builder().url(server.url("/x")).build()).execute()

    private fun notifier(): Pair<SessionExpiryNotifier, java.util.concurrent.atomic.AtomicInteger> {
        val notifier = SessionExpiryNotifier()
        val signals = java.util.concurrent.atomic.AtomicInteger(0)
        notifier.register { signals.incrementAndGet() }
        return notifier to signals
    }

    @Test
    fun `a 401 response signals session expiry`() {
        val (notifier, signals) = notifier()
        server.enqueue(MockResponse().setResponseCode(401))

        val response = get(client(notifier))

        assertEquals(401, response.code)
        assertEquals(1, signals.get())
    }

    @Test
    fun `a 403 response does not signal session expiry`() {
        val (notifier, signals) = notifier()
        server.enqueue(MockResponse().setResponseCode(403))

        val response = get(client(notifier))

        assertEquals(403, response.code)
        assertEquals(0, signals.get())
    }

    @Test
    fun `a 500 response does not signal session expiry`() {
        val (notifier, signals) = notifier()
        server.enqueue(MockResponse().setResponseCode(500))

        val response = get(client(notifier))

        assertEquals(500, response.code)
        assertEquals(0, signals.get())
    }

    @Test
    fun `a 200 response does not signal session expiry`() {
        val (notifier, signals) = notifier()
        server.enqueue(MockResponse().setResponseCode(200).setBody("ok"))

        val response = get(client(notifier))

        assertEquals(200, response.code)
        assertEquals(0, signals.get())
    }

    @Test
    fun `a 401 from a foreign host does not signal session expiry`() {
        // The client is Coil's too; an external avatar/image 401 must not log
        // the user out. Same origin-check as AuthInterceptor.
        val (notifier, signals) = notifier()
        server.enqueue(MockResponse().setResponseCode(401))

        val response = get(client(notifier, baseUrl = "https://api.example.com"))

        assertEquals(401, response.code)
        assertEquals(0, signals.get())
    }

    @Test
    fun `a 401 with a blank configured base url does not signal session expiry`() {
        val (notifier, signals) = notifier()
        server.enqueue(MockResponse().setResponseCode(401))

        val response = get(client(notifier, baseUrl = ""))

        assertEquals(401, response.code)
        assertEquals(0, signals.get())
    }

    @Test
    fun `the response body passes through untouched`() {
        val (notifier, _) = notifier()
        server.enqueue(MockResponse().setResponseCode(401).setBody("expired"))

        val response = get(client(notifier))

        assertEquals(401, response.code)
        assertEquals("expired", response.body?.string())
    }

    @Test
    fun `NetworkFactory installs the interceptor when provided`() {
        val (notifier, signals) = notifier()
        val client = NetworkFactory.okHttpClient(
            tokenProvider = TokenProvider { null },
            baseUrlProvider = BaseUrlProvider { server.url("/").toString() },
            sessionExpiryInterceptor = SessionExpiryInterceptor(notifier, BaseUrlProvider { server.url("/").toString() }),
        )
        server.enqueue(MockResponse().setResponseCode(401))

        val response = get(client)

        assertEquals(401, response.code)
        assertEquals(1, signals.get())
    }
}
