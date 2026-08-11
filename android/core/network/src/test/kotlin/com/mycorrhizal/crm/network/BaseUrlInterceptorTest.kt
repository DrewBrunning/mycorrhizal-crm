package com.mycorrhizal.crm.network

import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Test

class BaseUrlInterceptorTest {

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
    fun `rewrites the request origin onto the configured server`() {
        val interceptor = BaseUrlInterceptor(BaseUrlProvider { server.url("/").toString().trimEnd('/') })
        val client = OkHttpClient.Builder().addInterceptor(interceptor).build()
        server.enqueue(MockResponse().setResponseCode(200))

        // Request built against the placeholder origin; interceptor must point it at the server.
        client.newCall(Request.Builder().url("http://placeholder/api/v1/contacts").build()).execute()

        val recorded = server.takeRequest()
        assertEquals("/api/v1/contacts", recorded.path)
    }

    @Test
    fun `leaves the request untouched when base url is blank`() {
        val interceptor = BaseUrlInterceptor(BaseUrlProvider { "" })
        val client = OkHttpClient.Builder().addInterceptor(interceptor).build()
        server.enqueue(MockResponse().setResponseCode(200))

        val request = Request.Builder().url(server.url("/api/v1/contacts")).build()
        client.newCall(request).execute()

        val recorded = server.takeRequest()
        assertEquals("/api/v1/contacts", recorded.path)
    }

    @Test
    fun `leaves the request untouched when base url is malformed`() {
        val interceptor = BaseUrlInterceptor(BaseUrlProvider { "not a url" })
        val client = OkHttpClient.Builder().addInterceptor(interceptor).build()
        server.enqueue(MockResponse().setResponseCode(200))

        val request = Request.Builder().url(server.url("/api/v1/contacts")).build()
        client.newCall(request).execute()

        val recorded = server.takeRequest()
        assertEquals("/api/v1/contacts", recorded.path)
    }
}
