package com.mycorrhizal.crm.network

import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Before
import org.junit.Test

/**
 * ADR 0028 Decision 3 / issue #1265: an archived profile "can still be browsed,
 * and every write is blocked by an interceptor that fails writes on archived
 * profiles with a clear message."
 */
class ArchivedProfileWriteInterceptorTest {

    private lateinit var server: MockWebServer
    private var archived = true

    private fun client(): OkHttpClient = NetworkFactory.okHttpClient(
        tokenProvider = TokenProvider { null },
        baseUrlProvider = BaseUrlProvider { server.url("/").toString().trimEnd('/') },
        archivedProfileProvider = ArchivedProfileProvider { archived },
    )

    private fun request(method: String): Request {
        val builder = Request.Builder().url("http://mycorrhizal.invalid/api/v1/contacts")
        val body = "{}".toRequestBody()
        return when (method) {
            "GET" -> builder.get()
            "HEAD" -> builder.head()
            "POST" -> builder.post(body)
            "PUT" -> builder.put(body)
            "PATCH" -> builder.patch(body)
            "DELETE" -> builder.delete(body)
            else -> builder.method(method, null)
        }.build()
    }

    @Before
    fun setup() {
        server = MockWebServer()
        server.start()
        archived = true
    }

    @After
    fun teardown() {
        server.shutdown()
    }

    @Test
    fun `reads still go through on an archived profile`() {
        server.enqueue(MockResponse().setResponseCode(200))
        server.enqueue(MockResponse().setResponseCode(200))

        assertEquals(200, client().newCall(request("GET")).execute().use { it.code })
        assertEquals(200, client().newCall(request("HEAD")).execute().use { it.code })
        assertEquals(2, server.requestCount)
    }

    @Test
    fun `every write method is refused before reaching the server`() {
        listOf("POST", "PUT", "PATCH", "DELETE").forEach { method ->
            try {
                client().newCall(request(method)).execute().close()
                fail("$method should have been blocked")
            } catch (e: ArchivedProfileWriteException) {
                assertEquals(method, e.method)
                assertTrue(e.message.orEmpty().contains("read-only archive"))
            }
        }
        assertEquals("a blocked write must never leave the process", 0, server.requestCount)
    }

    @Test
    fun `writes go through when the profile is not archived`() {
        archived = false
        server.enqueue(MockResponse().setResponseCode(201))

        assertEquals(201, client().newCall(request("POST")).execute().use { it.code })
        assertEquals(1, server.requestCount)
    }

    @Test
    fun `the archived flag is read per request so a switch takes effect immediately`() {
        server.enqueue(MockResponse().setResponseCode(201))
        val shared = client()

        try {
            shared.newCall(request("POST")).execute().close()
            fail("expected the archived write to be blocked")
        } catch (_: ArchivedProfileWriteException) {
            // expected
        }
        archived = false
        assertEquals(201, shared.newCall(request("POST")).execute().use { it.code })
    }

    @Test
    fun `no provider means no interceptor is installed`() {
        server.enqueue(MockResponse().setResponseCode(201))
        val unguarded = NetworkFactory.okHttpClient(
            tokenProvider = TokenProvider { null },
            baseUrlProvider = BaseUrlProvider { server.url("/").toString().trimEnd('/') },
        )

        assertEquals(201, unguarded.newCall(request("POST")).execute().use { it.code })
    }

    @Test
    fun `the exception maps to a dedicated ApiError with a clear message`() {
        val error = ArchivedProfileWriteException("POST").toApiError()

        assertTrue(error is ApiError.ArchivedProfile)
        assertNotNull(error.cause)
        assertTrue(error.displayMessage.contains("read-only archive"))
        assertNull((error as ApiError).cause?.cause)
    }
}
