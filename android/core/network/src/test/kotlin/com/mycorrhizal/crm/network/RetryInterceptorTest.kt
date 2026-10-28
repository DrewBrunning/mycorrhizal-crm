package com.mycorrhizal.crm.network

import java.io.IOException
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger
import okhttp3.Interceptor
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.fail
import org.junit.Before
import org.junit.Test

class RetryInterceptorTest {

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
    fun `retries a 500 response for GET requests`() {
        val interceptor = RetryInterceptor(maxRetries = 2, baseDelayMs = 1)
        val client = OkHttpClient.Builder().addInterceptor(interceptor).build()
        server.enqueue(MockResponse().setResponseCode(500))
        server.enqueue(MockResponse().setResponseCode(500))
        server.enqueue(MockResponse().setResponseCode(200).setBody("ok"))

        val response = client.newCall(Request.Builder().url(server.url("/x")).build()).execute()

        assertEquals(200, response.code)
        assertEquals(3, server.requestCount)
    }

    @Test
    fun `does not retry a POST request on 500`() {
        val interceptor = RetryInterceptor(maxRetries = 2, baseDelayMs = 1)
        val client = OkHttpClient.Builder().addInterceptor(interceptor).build()
        server.enqueue(MockResponse().setResponseCode(500))

        val response = client.newCall(
            Request.Builder().url(
                server.url("/x")
            ).post("{}".toRequestBody()).build()
        ).execute()

        assertEquals(500, response.code)
        assertEquals(1, server.requestCount)
    }

    @Test
    fun `gives up after maxRetries for a persistent 500`() {
        val interceptor = RetryInterceptor(maxRetries = 2, baseDelayMs = 1)
        val client = OkHttpClient.Builder().addInterceptor(interceptor).build()
        server.enqueue(MockResponse().setResponseCode(500))
        server.enqueue(MockResponse().setResponseCode(500))
        server.enqueue(MockResponse().setResponseCode(500))

        val response = client.newCall(Request.Builder().url(server.url("/x")).build()).execute()

        assertEquals(500, response.code)
        assertEquals(3, server.requestCount)
    }

    @Test
    fun `does not retry a 4xx response`() {
        val interceptor = RetryInterceptor(maxRetries = 3, baseDelayMs = 1)
        val client = OkHttpClient.Builder().addInterceptor(interceptor).build()
        server.enqueue(MockResponse().setResponseCode(404))

        val response = client.newCall(Request.Builder().url(server.url("/x")).build()).execute()

        assertEquals(404, response.code)
        assertEquals(1, server.requestCount)
    }

    @Test
    fun `network error is retried for GET`() {
        val interceptor = RetryInterceptor(maxRetries = 2, baseDelayMs = 1)
        val attempts = AtomicInteger()
        // Sits behind RetryInterceptor, so it sees every attempt that reaches the network.
        val counter = Interceptor { chain ->
            attempts.incrementAndGet()
            chain.proceed(chain.request())
        }
        val client = OkHttpClient.Builder()
            .addInterceptor(interceptor)
            .addInterceptor(counter)
            .callTimeout(2, TimeUnit.SECONDS)
            .build()
        server.shutdown() // force a connection error on every attempt

        try {
            client.newCall(Request.Builder().url("http://127.0.0.1:1/x").build()).execute()
            fail("expected an IOException after exhausting retries")
        } catch (_: IOException) {
            // expected
        }

        assertEquals(1 + 2, attempts.get())
    }
}
