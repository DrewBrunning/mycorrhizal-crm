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

class AuthInterceptorTest {

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

    private fun interceptor(token: String?): AuthInterceptor = AuthInterceptor(
        tokenProvider = TokenProvider { token },
        baseUrlProvider = BaseUrlProvider { server.url("/").toString().trimEnd('/') },
    )

    @Test
    fun `adds Bearer header when a token is present`() {
        val client = OkHttpClient.Builder().addInterceptor(interceptor("test-jwt")).build()
        server.enqueue(MockResponse().setResponseCode(200))

        client.newCall(Request.Builder().url(server.url("/test")).build()).execute()

        val recorded = server.takeRequest()
        assertEquals("Bearer test-jwt", recorded.getHeader("Authorization"))
    }

    @Test
    fun `omits header when no token is stored`() {
        val client = OkHttpClient.Builder().addInterceptor(interceptor(null)).build()
        server.enqueue(MockResponse().setResponseCode(200))

        client.newCall(Request.Builder().url(server.url("/test")).build()).execute()

        val recorded = server.takeRequest()
        assertNull(recorded.getHeader("Authorization"))
    }

    @Test
    fun `omits header when token is blank`() {
        val client = OkHttpClient.Builder().addInterceptor(interceptor("  ")).build()
        server.enqueue(MockResponse().setResponseCode(200))

        client.newCall(Request.Builder().url(server.url("/test")).build()).execute()

        val recorded = server.takeRequest()
        assertNull(recorded.getHeader("Authorization"))
    }

    @Test
    fun `does not overwrite an existing Authorization header`() {
        val client = OkHttpClient.Builder().addInterceptor(interceptor("token")).build()
        server.enqueue(MockResponse().setResponseCode(200))

        val request = Request.Builder()
            .url(server.url("/test"))
            .header("Authorization", "Basic abc")
            .build()
        client.newCall(request).execute()

        val recorded = server.takeRequest()
        assertEquals("Basic abc", recorded.getHeader("Authorization"))
    }

    @Test
    fun `omits header when the request targets a different host`() {
        // The configured origin never receives a request here; the actual
        // request goes to the mock server whose host differs — the JWT must
        // not be attached to a host other than the configured origin.
        val auth = AuthInterceptor(
            tokenProvider = TokenProvider { "secret-jwt" },
            baseUrlProvider = BaseUrlProvider { "https://configured.example.com" },
        )
        val client = OkHttpClient.Builder().addInterceptor(auth).build()
        server.enqueue(MockResponse().setResponseCode(200))

        client.newCall(Request.Builder().url(server.url("/x")).build()).execute()

        val recorded = server.takeRequest()
        assertNull(recorded.getHeader("Authorization"))
    }

    @Test
    fun `omits header when the base url is not configured`() {
        val auth = AuthInterceptor(
            tokenProvider = TokenProvider { "secret-jwt" },
            baseUrlProvider = BaseUrlProvider { "" },
        )
        val client = OkHttpClient.Builder().addInterceptor(auth).build()
        server.enqueue(MockResponse().setResponseCode(200))

        client.newCall(Request.Builder().url(server.url("/x")).build()).execute()

        val recorded = server.takeRequest()
        assertNull(recorded.getHeader("Authorization"))
    }
}
