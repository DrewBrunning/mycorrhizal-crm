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
 * Issue #678: any API response with a 401 must signal session expiry so the
 * app can clear the session and land on the auth flow. The interceptor is
 * purely observational — it must pass every response through untouched and
 * only ever signal on a real 401.
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

    private fun client(notifier: SessionExpiryNotifier): OkHttpClient =
        OkHttpClient.Builder()
            .addInterceptor(SessionExpiryInterceptor(notifier))
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
            sessionExpiryInterceptor = SessionExpiryInterceptor(notifier),
        )
        server.enqueue(MockResponse().setResponseCode(401))

        val response = get(client)

        assertEquals(401, response.code)
        assertEquals(1, signals.get())
    }
}
