package com.mycorrhizal.crm.data.local

import com.mycorrhizal.crm.data.session.DefaultSessionManager
import com.mycorrhizal.crm.data.session.FakeSessionPrefsStorage
import com.mycorrhizal.crm.data.session.FakeTokenStorage
import com.mycorrhizal.crm.domain.repository.SessionState
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** Issue #1312: the Local-profile 401 recovery path (restart + adopt a fresh token). */
class LocalSessionReminterTest {

    private class FakeHost(
        private val startResult: Result<LocalServerEndpoint>,
        private val runningToken: String? = null,
    ) : LocalServerHost {
        val calls = mutableListOf<String>()

        override suspend fun ensureStarted(): Result<LocalServerEndpoint> {
            calls += "start"
            return startResult
        }

        override suspend fun stop() {
            calls += "stop"
        }

        override suspend fun deleteLocalData() = Unit

        override fun socketPathIfRunning(): String? = null

        override fun sessionTokenIfRunning(): String? = runningToken
    }

    private fun manager() = DefaultSessionManager(FakeTokenStorage(), FakeSessionPrefsStorage())

    @Test
    fun `a Local profile restarts the host and adopts the fresh token`() = runTest {
        val manager = manager()
        manager.activateLocalProfile("stale-token")
        val host = FakeHost(Result.success(LocalServerEndpoint("/sock", "fresh-token")))

        val result = LocalSessionReminter(host, manager).remint()

        assertEquals(true, result)
        assertEquals(listOf("stop", "start"), host.calls)
        assertEquals("fresh-token", manager.bearerToken())
    }

    @Test
    fun `a running server whose token differs from the stored one is adopted without a restart`() = runTest {
        val manager = manager()
        manager.activateLocalProfile("stale-token")
        val host = FakeHost(Result.success(LocalServerEndpoint("/sock", "unused")), runningToken = "running-token")

        assertEquals(true, LocalSessionReminter(host, manager).remint())
        assertTrue(host.calls.isEmpty())
        assertEquals("running-token", manager.bearerToken())
    }

    @Test
    fun `a running server whose token is the rejected one is restarted`() = runTest {
        val manager = manager()
        manager.activateLocalProfile("same-token")
        val host = FakeHost(Result.success(LocalServerEndpoint("/sock", "fresh-token")), runningToken = "same-token")

        assertEquals(true, LocalSessionReminter(host, manager).remint())
        assertEquals(listOf("stop", "start"), host.calls)
        assertEquals("fresh-token", manager.bearerToken())
    }

    @Test
    fun `a failed restart reports false and adopts nothing`() = runTest {
        val manager = manager()
        manager.activateLocalProfile("stale-token")
        val host = FakeHost(Result.failure(IllegalStateException("boom")))

        val result = LocalSessionReminter(host, manager).remint()

        assertEquals(false, result)
        assertEquals("stale-token", manager.bearerToken())
    }

    @Test
    fun `a Remote profile is not applicable and the host is untouched`() = runTest {
        val manager = manager()
        manager.setSession("https://crm.example.com", "jwt-1", SessionState(userId = 7))
        val host = FakeHost(Result.success(LocalServerEndpoint("/sock", "x")))

        assertNull(LocalSessionReminter(host, manager).remint())
        assertTrue(host.calls.isEmpty())
    }

    @Test
    fun `no active profile is not applicable`() = runTest {
        val host = FakeHost(Result.success(LocalServerEndpoint("/sock", "x")))
        assertNull(LocalSessionReminter(host, manager()).remint())
        assertTrue(host.calls.isEmpty())
    }

    // Issue #1353: a 401 for a token older than the running server must never
    // stop/restart it.
    @Test
    fun `a stale-token 401 after adoption does not restart the host`() = runTest {
        val manager = manager()
        manager.activateLocalProfile("t2") // already adopted
        val host = FakeHost(Result.success(LocalServerEndpoint("/sock", "unused")), runningToken = "t2")

        assertEquals(true, LocalSessionReminter(host, manager).remint(rejectedBearer = "t1"))
        assertTrue("no stop/start for a stale token", host.calls.isEmpty())
        assertEquals("t2", manager.bearerToken())
    }

    @Test
    fun `a stale-token 401 with an unadopted running token adopts it without a restart`() = runTest {
        val manager = manager()
        manager.activateLocalProfile("t1")
        val host = FakeHost(Result.success(LocalServerEndpoint("/sock", "unused")), runningToken = "t2")

        assertEquals(true, LocalSessionReminter(host, manager).remint(rejectedBearer = "t1"))
        assertTrue(host.calls.isEmpty())
        assertEquals("t2", manager.bearerToken())
    }

    @Test
    fun `a 401 carrying the running token restarts exactly once`() = runTest {
        val manager = manager()
        manager.activateLocalProfile("t2")
        val host = FakeHost(Result.success(LocalServerEndpoint("/sock", "t3")), runningToken = "t2")

        assertEquals(true, LocalSessionReminter(host, manager).remint(rejectedBearer = "t2"))
        assertEquals(listOf("stop", "start"), host.calls)
        assertEquals("t3", manager.bearerToken())
    }

    @Test
    fun `a 401 with no rejected bearer and a stopped server starts it`() = runTest {
        val manager = manager()
        manager.activateLocalProfile("t1")
        val host = FakeHost(Result.success(LocalServerEndpoint("/sock", "t2")), runningToken = null)

        assertEquals(true, LocalSessionReminter(host, manager).remint(rejectedBearer = "t1"))
        assertEquals(listOf("stop", "start"), host.calls)
        assertEquals("t2", manager.bearerToken())
    }
}
