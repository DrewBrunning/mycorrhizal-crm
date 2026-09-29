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

    private class FakeHost(private val startResult: Result<LocalServerEndpoint>) : LocalServerHost {
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

        override fun sessionTokenIfRunning(): String? = null
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
}
