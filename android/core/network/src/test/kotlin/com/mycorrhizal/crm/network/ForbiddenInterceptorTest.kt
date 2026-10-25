package com.mycorrhizal.crm.network

import java.util.concurrent.atomic.AtomicInteger
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Test

/**
 * Issue #1826: an authenticated 403 from the configured server signals
 * [ForbiddenNotifier] (the role may have changed). It is purely observational,
 * leaves 401 to the session-expiry flow, and ignores unauthenticated or
 * foreign-host 403s.
 */
class ForbiddenInterceptorTest {

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

    private fun client(
        notifier: ForbiddenNotifier,
        baseUrl: String = server.url(
            "/"
        ).toString()
    ): OkHttpClient = OkHttpClient.Builder()
        .addInterceptor(ForbiddenInterceptor(notifier, BaseUrlProvider { baseUrl }))
        .build()

    private fun counted(): Pair<ForbiddenNotifier, AtomicInteger> {
        val notifier = ForbiddenNotifier()
        val signals = AtomicInteger(0)
        notifier.register { signals.incrementAndGet() }
        return notifier to signals
    }

    private fun get(client: OkHttpClient, bearer: String? = "t1") = client.newCall(
        Request.Builder().url(
            server.url("/x")
        ).apply { bearer?.let { header("Authorization", "Bearer $it") } }.build()
    ).execute()

    @Test
    fun `an authenticated 403 signals forbidden`() {
        val (notifier, signals) = counted()
        server.enqueue(MockResponse().setResponseCode(403))

        val response = get(client(notifier))

        assertEquals(403, response.code)
        assertEquals(1, signals.get())
    }

    @Test
    fun `a 403 without a bearer does not signal`() {
        val (notifier, signals) = counted()
        server.enqueue(MockResponse().setResponseCode(403))

        get(client(notifier), bearer = null)

        assertEquals(0, signals.get())
    }

    @Test
    fun `a 401 is left to the session-expiry flow`() {
        val (notifier, signals) = counted()
        server.enqueue(MockResponse().setResponseCode(401))

        get(client(notifier))

        assertEquals(0, signals.get())
    }

    @Test
    fun `200 and 500 responses do not signal`() {
        val (notifier, signals) = counted()
        server.enqueue(MockResponse().setResponseCode(200))
        server.enqueue(MockResponse().setResponseCode(500))
        val client = client(notifier)

        get(client)
        get(client)

        assertEquals(0, signals.get())
    }

    @Test
    fun `a 403 from a foreign host does not signal`() {
        val (notifier, signals) = counted()
        server.enqueue(MockResponse().setResponseCode(403))

        get(client(notifier, baseUrl = "https://api.example.com"))

        assertEquals(0, signals.get())
    }

    @Test
    fun `a 403 with a blank configured base url does not signal`() {
        val (notifier, signals) = counted()
        server.enqueue(MockResponse().setResponseCode(403))

        get(client(notifier, baseUrl = ""))

        assertEquals(0, signals.get())
    }

    @Test
    fun `the response passes through untouched`() {
        val (notifier, _) = counted()
        server.enqueue(MockResponse().setResponseCode(403).setBody("forbidden"))

        val response = get(client(notifier))

        assertEquals(403, response.code)
        assertEquals("forbidden", response.body?.string())
    }

    @Test
    fun `NetworkFactory installs the interceptor when provided`() {
        val (notifier, signals) = counted()
        val baseUrl = BaseUrlProvider { server.url("/").toString() }
        val client = NetworkFactory.okHttpClient(
            tokenProvider = TokenProvider { "t1" },
            baseUrlProvider = baseUrl,
            forbiddenInterceptor = ForbiddenInterceptor(notifier, baseUrl)
        )
        server.enqueue(MockResponse().setResponseCode(403))

        get(client)

        assertEquals(1, signals.get())
    }
}
