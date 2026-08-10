package com.mycorrhizal.crm.network

import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Before
import org.junit.Test

/**
 * Pins the interceptor ORDER the auth design depends on: BaseUrl must run
 * before Auth, because Auth's host check compares the request URL against the
 * configured server. If Auth runs first, the URL is still the placeholder
 * origin and the check always fails — every authenticated request would go
 * out without the token (a real bug the isolated interceptor tests can't see).
 */
class AuthInterceptorOrderingTest {

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

    @Test
    fun `a request through the real stack carries the bearer token`() {
        // The full client: BaseUrl first (rewrites the placeholder), then Auth
        // (sees the real host and attaches the token).
        val client = NetworkFactory.okHttpClient(
            tokenProvider = TokenProvider { "secret-jwt" },
            baseUrlProvider = BaseUrlProvider { server.url("/").toString().trimEnd('/') },
        )
        server.enqueue(MockResponse().setResponseCode(200))

        // Built against the placeholder origin, as the ApiClient does.
        client.newCall(Request.Builder().url("http://mycorrhizal.invalid/api/v1/contacts").build()).execute()

        val recorded = server.takeRequest()
        assertEquals("/api/v1/contacts", recorded.path)
        assertEquals("Bearer secret-jwt", recorded.getHeader("Authorization"))
    }

    @Test
    fun `base url rewrite forces every request to the configured host`() {
        // With BaseUrl registered first, a request addressed anywhere gets
        // rewritten onto the configured server before Auth runs — so the token
        // can only ever reach the configured host (the security property the
        // ordering exists to guarantee).
        val client = NetworkFactory.okHttpClient(
            tokenProvider = TokenProvider { "secret-jwt" },
            baseUrlProvider = BaseUrlProvider { server.url("/").toString().trimEnd('/') },
        )
        server.enqueue(MockResponse().setResponseCode(200))

        // Even an unrelated placeholder URL is rewritten to the configured server.
        client.newCall(Request.Builder().url("http://whatever.invalid/somewhere").build()).execute()

        val recorded = server.takeRequest()
        assertEquals("/somewhere", recorded.path)
        assertEquals("Bearer secret-jwt", recorded.getHeader("Authorization"))
    }

    @Test
    fun `login carries no token but the host is still rewritten`() {
        val client = NetworkFactory.okHttpClient(
            tokenProvider = TokenProvider { null },
            baseUrlProvider = BaseUrlProvider { server.url("/").toString().trimEnd('/') },
        )
        server.enqueue(MockResponse().setResponseCode(200))

        client.newCall(Request.Builder().url("http://mycorrhizal.invalid/api/v1/login").post(okhttp3.RequestBody.create(null, "{}")).build()).execute()

        val recorded = server.takeRequest()
        assertEquals("/api/v1/login", recorded.path)
        assertNull(recorded.getHeader("Authorization"))
    }
}
