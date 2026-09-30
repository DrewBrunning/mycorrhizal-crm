package com.mycorrhizal.crm.di

import com.mycorrhizal.crm.data.local.LocalServerEndpoint
import com.mycorrhizal.crm.data.local.LocalServerHost
import com.mycorrhizal.crm.network.LOCAL_SERVER_SENTINEL_URL
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import okhttp3.Interceptor
import okhttp3.Protocol
import okhttp3.Request
import okhttp3.Response
import okhttp3.ResponseBody.Companion.toResponseBody
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertThrows
import org.junit.Test
import java.io.IOException

/**
 * Issue #1262 / ADR 0028 Decision 2: the lazy-start seam. The embedded server
 * must be up before a `Local`-profile request reaches the socket transport, and
 * a failure to start must surface as an ordinary network-shaped error.
 */
class LocalServerWakeInterceptorTest {

    private val host = mockk<LocalServerHost>()

    private fun chainFor(url: String): Interceptor.Chain {
        val request = Request.Builder().url(url).build()
        val response = Response.Builder()
            .request(request)
            .protocol(Protocol.HTTP_1_1)
            .code(200)
            .message("OK")
            .body("{}".toResponseBody())
            .build()
        return mockk {
            every { this@mockk.request() } returns request
            every { proceed(any()) } returns response
        }
    }

    @Test
    fun `a sentinel request starts the embedded server first`() {
        coEvery { host.ensureStarted() } returns
            Result.success(LocalServerEndpoint(socketPath = "/sock", sessionToken = "tok"))

        val chain = chainFor("$LOCAL_SERVER_SENTINEL_URL/health")
        val response = LocalServerWakeInterceptor(host).intercept(chain)

        assertEquals(200, response.code)
        coVerify(exactly = 1) { host.ensureStarted() }
    }

    private fun capturingChain(url: String, authorization: String?): Pair<Interceptor.Chain, MutableList<Request>> {
        val request = Request.Builder().url(url).apply {
            if (authorization != null) header("Authorization", authorization)
        }.build()
        val sent = mutableListOf<Request>()
        val chain = mockk<Interceptor.Chain> {
            every { this@mockk.request() } returns request
            every { proceed(any()) } answers {
                val r = firstArg<Request>()
                sent += r
                Response.Builder().request(r).protocol(Protocol.HTTP_1_1).code(200).message("OK")
                    .body("{}".toResponseBody()).build()
            }
        }
        return chain to sent
    }

    // Issue #1353: a request queued across a start carries the previous token;
    // the wake step re-stamps it with the token of the server that answers.
    @Test
    fun `a stale bearer is re-stamped with the running server's token`() {
        coEvery { host.ensureStarted() } returns
            Result.success(LocalServerEndpoint(socketPath = "/sock", sessionToken = "t2"))
        val (chain, sent) = capturingChain("$LOCAL_SERVER_SENTINEL_URL/contacts", "Bearer t1")

        LocalServerWakeInterceptor(host).intercept(chain)

        assertEquals("Bearer t2", sent.single().header("Authorization"))
    }

    @Test
    fun `a bearer already matching the running token is left untouched`() {
        coEvery { host.ensureStarted() } returns
            Result.success(LocalServerEndpoint(socketPath = "/sock", sessionToken = "t2"))
        val (chain, sent) = capturingChain("$LOCAL_SERVER_SENTINEL_URL/contacts", "Bearer t2")

        LocalServerWakeInterceptor(host).intercept(chain)

        assertEquals("Bearer t2", sent.single().header("Authorization"))
    }

    @Test
    fun `an unauthenticated sentinel request is never given a bearer`() {
        coEvery { host.ensureStarted() } returns
            Result.success(LocalServerEndpoint(socketPath = "/sock", sessionToken = "t2"))
        val (chain, sent) = capturingChain("$LOCAL_SERVER_SENTINEL_URL/health", null)

        LocalServerWakeInterceptor(host).intercept(chain)

        assertNull(sent.single().header("Authorization"))
    }

    @Test
    fun `a non-bearer Authorization header is not rewritten`() {
        coEvery { host.ensureStarted() } returns
            Result.success(LocalServerEndpoint(socketPath = "/sock", sessionToken = "t2"))
        val (chain, sent) = capturingChain("$LOCAL_SERVER_SENTINEL_URL/x", "Basic abc")

        LocalServerWakeInterceptor(host).intercept(chain)

        assertEquals("Basic abc", sent.single().header("Authorization"))
    }

    @Test
    fun `a remote request keeps its own bearer and never touches the embedded server`() {
        val (chain, sent) = capturingChain("https://crm.example.com/x", "Bearer remote")

        LocalServerWakeInterceptor(host).intercept(chain)

        assertEquals("Bearer remote", sent.single().header("Authorization"))
        coVerify(exactly = 0) { host.ensureStarted() }
    }

    @Test
    fun `a remote request never touches the embedded server`() {
        val chain = chainFor("https://crm.example.com/health")

        LocalServerWakeInterceptor(host).intercept(chain)

        coVerify(exactly = 0) { host.ensureStarted() }
    }

    @Test
    fun `a start failure becomes an IOException, not a silent pass-through`() {
        coEvery { host.ensureStarted() } returns Result.failure(IllegalStateException("no binary"))

        val chain = chainFor("$LOCAL_SERVER_SENTINEL_URL/health")

        assertThrows(IOException::class.java) {
            LocalServerWakeInterceptor(host).intercept(chain)
        }
    }
}
