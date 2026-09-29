package com.mycorrhizal.crm.network

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.IOException
import java.net.InetSocketAddress
import java.net.ServerSocket
import java.net.UnknownHostException
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicReference

/**
 * ADR 0028 Decision 2 transport routing. The critical invariant: the sentinel
 * host is only ever routed to the local socket when a live path is known, and
 * never reaches the network resolver.
 */
class LocalServerTransportTest {

    private val runningSocket = LocalSocketPathProvider { "/data/app/local-server/sock" }
    private val notRunning = LocalSocketPathProvider { null }

    @Test
    fun `sentinel with a live socket path routes to the local socket`() {
        assertEquals(
            LocalTransport.LOCAL_SOCKET,
            selectTransport(LOCAL_SERVER_SENTINEL_HOST, "/data/app/local-server/sock"),
        )
    }

    @Test
    fun `sentinel without a running socket never takes the local transport`() {
        assertEquals(LocalTransport.NETWORK, selectTransport(LOCAL_SERVER_SENTINEL_HOST, null))
        assertEquals(LocalTransport.NETWORK, selectTransport(LOCAL_SERVER_SENTINEL_HOST, "  "))
    }

    @Test
    fun `an ordinary host is always network even when a local socket is running`() {
        assertEquals(LocalTransport.NETWORK, selectTransport("crm.example.com", "/tmp/sock"))
        assertEquals(LocalTransport.NETWORK, selectTransport(null, "/tmp/sock"))
    }

    @Test
    fun `dns resolves the sentinel locally without touching the system resolver`() {
        val addresses = ProfileAwareDns(runningSocket).lookup(LOCAL_SERVER_SENTINEL_HOST)
        assertEquals(1, addresses.size)
        assertEquals(LOCAL_SERVER_SENTINEL_HOST, addresses.single().hostName)
        assertTrue(addresses.single().isLoopbackAddress)
    }

    @Test
    fun `dns delegates ordinary hosts to the system resolver`() {
        val addresses = ProfileAwareDns(runningSocket).lookup("localhost")
        assertTrue(addresses.isNotEmpty())
        assertTrue(addresses.any { it.isLoopbackAddress })
    }

    @Test(expected = UnknownHostException::class)
    fun `dns never routes the sentinel to the network when no socket is running`() {
        // The sentinel is under .invalid, so the system resolver cannot resolve
        // it; a passthrough is therefore a loud failure, not a silent reach to
        // some other host.
        ProfileAwareDns(notRunning).lookup(LOCAL_SERVER_SENTINEL_HOST)
    }

    @Test
    fun `a non-sentinel host still connects over a real TCP socket`() {
        val server = ServerSocket(0)
        val accepted = CountDownLatch(1)
        val error = AtomicReference<Throwable?>(null)
        val serverThread = Thread {
            try {
                server.accept().use { /* accept once, immediately close */ }
            } catch (t: Throwable) {
                error.set(t)
            } finally {
                accepted.countDown()
            }
        }
        serverThread.start()

        val socket = ProfileAwareSocketFactory(runningSocket).createSocket("127.0.0.1", server.localPort)
        socket.connect(InetSocketAddress("127.0.0.1", server.localPort), 2_000)
        assertTrue(socket.isConnected)
        assertFalse(socket.isClosed)
        assertEquals(server.localPort, socket.port)
        socket.close()

        assertTrue(accepted.await(5, TimeUnit.SECONDS))
        assertEquals(null, error.get())
        server.close()
    }

    @Test
    fun `connect with no socket path and no reachable network host fails loudly`() {
        val socket = ProfileAwareSocketFactory(notRunning).createSocket()
        try {
            socket.connect(InetSocketAddress(LOCAL_SERVER_SENTINEL_HOST, 80), 1_000)
            // If a resolver hijacked .invalid, fail on the assertion below.
            throw AssertionError("expected the sentinel to be unreachable")
        } catch (expected: IOException) {
            // Correct: no silent fallback to another host.
        }
    }
}
