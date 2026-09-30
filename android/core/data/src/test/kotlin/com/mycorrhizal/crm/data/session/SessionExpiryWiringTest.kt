package com.mycorrhizal.crm.data.session

import com.mycorrhizal.crm.domain.repository.SessionState
import com.mycorrhizal.crm.network.SessionExpiryNotifier
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Issue #678: the glue between the network layer's 401 detection and the
 * session store. A session must never survive a 401 — the wiring clears it,
 * which flips the app to the auth flow.
 */
class SessionExpiryWiringTest {

    @Test
    fun `a session-expiry signal clears the session`() = runTest {
        val notifier = SessionExpiryNotifier()
        val tokenStorage = FakeTokenStorage()
        val prefsStorage = FakeSessionPrefsStorage()
        val cleaner = RecordingCleaner()
        val manager = DefaultSessionManager(tokenStorage, prefsStorage, cleaner)
        manager.setSession(
            serverUrl = "https://crm.example.com",
            token = "jwt-1",
            state = SessionState(userId = 7, username = "alice"),
        )

        SessionExpiryWiring(notifier, manager).start(this)
        notifier.onSessionExpired()
        advanceUntilIdle()

        assertNull("the bearer token must be dropped", manager.bearerToken())
        assertNull("the persisted token must be dropped", tokenStorage.stored)
        assertFalse("the session must flip to logged-out", manager.observeSession().first().isLoggedIn)
        assertEquals("local PII must be wiped on session end", 1, cleaner.clearCount)
        // Issue #723: a 401-driven logout goes through the same clearSession as
        // an explicit logout — the server URL survives it too, so the login
        // screen that follows is pre-filled.
        assertEquals("https://crm.example.com", manager.serverUrl())
    }

    @Test
    fun `a signal with no active session is a harmless no-op`() = runTest {
        val notifier = SessionExpiryNotifier()
        val tokenStorage = FakeTokenStorage()
        val cleaner = RecordingCleaner()
        val manager = DefaultSessionManager(tokenStorage, FakeSessionPrefsStorage(), cleaner)

        SessionExpiryWiring(notifier, manager).start(this)
        notifier.onSessionExpired()
        advanceUntilIdle()

        assertNull(manager.bearerToken())
        assertFalse(manager.observeSession().first().isLoggedIn)
    }

    @Test
    fun `the session survives an authenticated request`() = runTest {
        val notifier = SessionExpiryNotifier()
        val manager = DefaultSessionManager(FakeTokenStorage(), FakeSessionPrefsStorage())
        manager.setSession("https://crm.example.com", "jwt-1", SessionState(userId = 7))

        SessionExpiryWiring(notifier, manager).start(this)

        assertTrue(manager.observeSession().first().isLoggedIn)
    }

    @Test
    fun `a fresh process restart wires and clears through the same path`() = runTest {
        // Process-death restore: a fresh manager instance hydrates the stored
        // token; a subsequent 401 must still clear it.
        val notifier = SessionExpiryNotifier()
        val tokenStorage = FakeTokenStorage()
        val prefsStorage = FakeSessionPrefsStorage()
        val first = DefaultSessionManager(tokenStorage, prefsStorage)
        first.setSession("https://crm.example.com", "jwt-1", SessionState(userId = 7))

        val restarted = DefaultSessionManager(tokenStorage, prefsStorage)
        restarted.init()
        SessionExpiryWiring(notifier, restarted).start(this)

        assertEquals("jwt-1", restarted.bearerToken())
        notifier.onSessionExpired()
        advanceUntilIdle()

        assertNull(restarted.bearerToken())
        assertFalse(restarted.observeSession().first().isLoggedIn)
    }

    // Issue #722: when this install holds a device grant, a 401 first tries one
    // grant exchange. A successful exchange keeps the session — the JWT simply
    // expired, not the account.
    @Test
    fun `a successful grant refresh on 401 keeps the session`() = runTest {
        val notifier = SessionExpiryNotifier()
        val manager = DefaultSessionManager(FakeTokenStorage(), FakeSessionPrefsStorage())
        manager.setSession("https://crm.example.com", "jwt-1", SessionState(userId = 7))
        var refreshTried = false

        SessionExpiryWiring(notifier, manager, refresher = {
            refreshTried = true
            manager.setToken("jwt-2")
            true
        }).start(this)

        notifier.onSessionExpired()
        advanceUntilIdle()

        assertTrue("the grant refresh must be attempted", refreshTried)
        assertEquals("jwt-2", manager.bearerToken())
        assertTrue(manager.observeSession().first().isLoggedIn)
    }

    // Issue #722: a 401 whose grant exchange fails (grant revoked, server
    // rejects) falls back to the normal clear — a revoked device must end
    // exactly as a 401 always has.
    @Test
    fun `a failed grant refresh on 401 clears the session`() = runTest {
        val notifier = SessionExpiryNotifier()
        val manager = DefaultSessionManager(FakeTokenStorage(), FakeSessionPrefsStorage())
        manager.setSession("https://crm.example.com", "jwt-1", SessionState(userId = 7))

        SessionExpiryWiring(notifier, manager, refresher = { false }).start(this)

        notifier.onSessionExpired()
        advanceUntilIdle()

        assertNull(manager.bearerToken())
        assertFalse(manager.observeSession().first().isLoggedIn)
    }

    @Test
    fun `signals arriving before registration are not lost once registered`() = runTest {
        // A 401 that fires before the wiring registers must still clear the
        // session once the wiring is in place. Registration happens before any
        // request in production, but this pins the no-loss guarantee regardless.
        val notifier = SessionExpiryNotifier()
        val manager = DefaultSessionManager(FakeTokenStorage(), FakeSessionPrefsStorage())
        manager.setSession("https://crm.example.com", "jwt-1", SessionState(userId = 7))

        SessionExpiryWiring(notifier, manager).start(this)
        notifier.onSessionExpired()
        advanceUntilIdle()

        assertNull(manager.bearerToken())
    }

    // Issue #1312: a Local profile's 401 re-mints (restart + adopt the fresh
    // token) and must neither clear the session nor fall through to the grant
    // refresher.
    @Test
    fun `a successful local re-mint on 401 keeps the session and skips the grant refresher`() = runTest {
        val notifier = SessionExpiryNotifier()
        val manager = DefaultSessionManager(FakeTokenStorage(), FakeSessionPrefsStorage())
        manager.activateLocalProfile("old-token")
        var grantTried = false

        SessionExpiryWiring(
            notifier,
            manager,
            refresher = { grantTried = true; true },
            localRemint = { manager.activateLocalProfile("new-token"); true },
        ).start(this)
        notifier.onSessionExpired()
        advanceUntilIdle()

        assertEquals("new-token", manager.bearerToken())
        assertTrue(manager.observeSession().first().isLoggedIn)
        assertFalse("a Local profile has no grant to exchange", grantTried)
    }

    @Test
    fun `a failed local re-mint on 401 clears the session`() = runTest {
        val notifier = SessionExpiryNotifier()
        val manager = DefaultSessionManager(FakeTokenStorage(), FakeSessionPrefsStorage())
        manager.activateLocalProfile("old-token")

        SessionExpiryWiring(notifier, manager, localRemint = { false }).start(this)
        notifier.onSessionExpired()
        advanceUntilIdle()

        assertNull(manager.bearerToken())
        assertFalse(manager.observeSession().first().isLoggedIn)
    }

    // A Remote profile (localRemint returns null) keeps the existing behavior.
    @Test
    fun `a null local re-mint falls through to the grant refresher for a Remote profile`() = runTest {
        val notifier = SessionExpiryNotifier()
        val manager = DefaultSessionManager(FakeTokenStorage(), FakeSessionPrefsStorage())
        manager.setSession("https://crm.example.com", "jwt-1", SessionState(userId = 7))
        var grantTried = false

        SessionExpiryWiring(
            notifier,
            manager,
            refresher = { grantTried = true; false },
            localRemint = { null },
        ).start(this)
        notifier.onSessionExpired()
        advanceUntilIdle()

        assertTrue(grantTried)
        assertNull(manager.bearerToken())
    }

    private class RecordingCleaner : SessionDataCleaner {
        var clearCount = 0
        override suspend fun clear() {
            clearCount++
        }
    }

    // Issue #957 (finding #1, point 2): a 401 fired by clearSession()'s own
    // teardown call (the bearer it just used is being invalidated) must not
    // race a device-grant refresh that would silently resurrect the session
    // the app is in the middle of ending.
    @Test
    fun `a 401 raised by clearSession's own teardown call does not trigger a refresh`() = runTest {
        val notifier = SessionExpiryNotifier()
        var refreshAttempts = 0
        lateinit var manager: DefaultSessionManager
        // The teardown step re-fires the notifier itself -- exactly what a
        // real network call 401ing mid-teardown does via
        // SessionExpiryInterceptor -- while clearSession is still running.
        manager = DefaultSessionManager(
            FakeTokenStorage(),
            FakeSessionPrefsStorage(),
            sessionTeardown = SessionTeardown { notifier.onSessionExpired() },
        )
        manager.setSession("https://crm.example.com", "jwt-1", SessionState(userId = 7))
        // No grant to exchange -- the outer 401 falls through to clearSession,
        // which is where the re-entrant signal from teardown fires.
        SessionExpiryWiring(notifier, manager, refresher = { refreshAttempts++; false }).start(this)

        notifier.onSessionExpired()
        advanceUntilIdle()

        assertEquals("only the original 401 may attempt a refresh, not the re-entrant one", 1, refreshAttempts)
        assertNull("the session must still end up cleared", manager.bearerToken())
    }

    // Issue #957 (finding #1, point 2) — the actual reported bug: an
    // EXPLICIT logout calls clearSession() directly, outside any refresh
    // SessionExpiryWiring itself started. If teardown's own call 401s and a
    // device grant is enrolled, a refresh that ignored isClearingSession()
    // would SUCCEED and silently log the user back in seconds after they
    // chose to log out. The guard must stop that regardless of whether a
    // refresh WOULD have succeeded.
    @Test
    fun `an explicit logout's teardown 401 cannot silently resurrect the session via a working grant`() = runTest {
        val notifier = SessionExpiryNotifier()
        var grantExchangeCalled = false
        lateinit var manager: DefaultSessionManager
        manager = DefaultSessionManager(
            FakeTokenStorage(),
            FakeSessionPrefsStorage(),
            sessionTeardown = SessionTeardown { notifier.onSessionExpired() },
        )
        manager.setSession("https://crm.example.com", "jwt-1", SessionState(userId = 7))
        // A grant exchange that would succeed if ever attempted -- the guard
        // must prevent it from being attempted at all here, not rely on it
        // failing.
        SessionExpiryWiring(notifier, manager, refresher = {
            grantExchangeCalled = true
            manager.setToken("resurrected-jwt")
            true
        }).start(this)

        // The explicit logout path: AuthRepositoryImpl.logout() calls this
        // directly, never through SessionExpiryWiring.
        manager.clearSession()
        advanceUntilIdle()

        assertFalse("teardown's own 401 must never reach the grant exchange", grantExchangeCalled)
        assertNull("the session must end up logged out, not resurrected", manager.bearerToken())
        assertFalse(manager.observeSession().first().isLoggedIn)
    }

    // Issue #967: a burst of concurrent 401s -- or the grant exchange's own
    // 401 re-firing this listener recursively -- must collapse into exactly
    // one refresh attempt, not a storm of grant-exchange requests.
    @Test
    fun `concurrent 401 signals collapse into a single refresh attempt`() = runTest {
        val notifier = SessionExpiryNotifier()
        val manager = DefaultSessionManager(FakeTokenStorage(), FakeSessionPrefsStorage())
        manager.setSession("https://crm.example.com", "jwt-1", SessionState(userId = 7))
        var refreshStarts = 0
        val refresherEntered = kotlinx.coroutines.CompletableDeferred<Unit>()
        val releaseRefresher = kotlinx.coroutines.CompletableDeferred<Unit>()

        SessionExpiryWiring(notifier, manager, refresher = {
            refreshStarts++
            refresherEntered.complete(Unit)
            releaseRefresher.await()
            true
        }).start(this)

        // Fire a burst before the first refresh has had a chance to finish.
        notifier.onSessionExpired()
        refresherEntered.await()
        notifier.onSessionExpired()
        notifier.onSessionExpired()
        releaseRefresher.complete(Unit)
        advanceUntilIdle()

        assertEquals("only one refresh may be in flight at a time", 1, refreshStarts)
    }

    // Issue #967: once a refresh completes (successfully or not), the guard
    // must release so the NEXT, unrelated 401 is handled normally.
    @Test
    fun `the single-flight guard releases after a refresh completes`() = runTest {
        val notifier = SessionExpiryNotifier()
        val manager = DefaultSessionManager(FakeTokenStorage(), FakeSessionPrefsStorage())
        manager.setSession("https://crm.example.com", "jwt-1", SessionState(userId = 7))
        var refreshStarts = 0

        SessionExpiryWiring(notifier, manager, refresher = { refreshStarts++; true }).start(this)

        notifier.onSessionExpired()
        advanceUntilIdle()
        notifier.onSessionExpired()
        advanceUntilIdle()

        assertEquals("a second, later 401 must start its own refresh", 2, refreshStarts)
    }

    // Issue #1353: the rejected bearer reaches the Local re-mint hook.
    @Test
    fun `the rejected bearer is passed to the local re-mint hook`() = runTest {
        val notifier = SessionExpiryNotifier()
        val manager = DefaultSessionManager(FakeTokenStorage(), FakeSessionPrefsStorage())
        manager.setSession("https://crm.example.com", "jwt-1", SessionState(userId = 7))
        val seen = mutableListOf<String?>()

        SessionExpiryWiring(notifier, manager, localRemint = { seen += it; true }).start(this)
        notifier.onSessionExpired("stale-token")
        advanceUntilIdle()
        notifier.onSessionExpired()
        advanceUntilIdle()

        assertEquals(listOf("stale-token", null), seen)
    }

    // Issue #1353 end to end through the real reminter: a burst of stale-token
    // 401s after adoption, and serial ones after the burst, never restart.
    @Test
    fun `a burst of stale-token 401s produces zero restarts`() = runTest {
        val notifier = SessionExpiryNotifier()
        val manager = DefaultSessionManager(FakeTokenStorage(), FakeSessionPrefsStorage())
        manager.activateLocalProfile("t2")
        val host = CountingHost(runningToken = "t2")
        val reminter = com.mycorrhizal.crm.data.local.LocalSessionReminter(host, manager)

        SessionExpiryWiring(notifier, manager, localRemint = { reminter.remint(it) }).start(this)
        repeat(5) { notifier.onSessionExpired("t1") }
        advanceUntilIdle()
        repeat(3) {
            notifier.onSessionExpired("t1")
            advanceUntilIdle()
        }

        assertEquals("stale 401s must never stop the server", 0, host.stops)
        assertEquals("stale 401s must never start the server", 0, host.starts)
        assertEquals("t2", manager.bearerToken())
        assertTrue("the session must survive", manager.observeSession().first().isLoggedIn)
    }

    @Test
    fun `a 401 carrying the running token restarts once even in a burst`() = runTest {
        val notifier = SessionExpiryNotifier()
        val manager = DefaultSessionManager(FakeTokenStorage(), FakeSessionPrefsStorage())
        manager.activateLocalProfile("t2")
        val host = CountingHost(runningToken = "t2", mintsOnStart = "t3")
        val reminter = com.mycorrhizal.crm.data.local.LocalSessionReminter(host, manager)

        SessionExpiryWiring(notifier, manager, localRemint = { reminter.remint(it) }).start(this)
        repeat(4) { notifier.onSessionExpired("t2") }
        advanceUntilIdle()

        assertEquals(1, host.stops)
        assertEquals(1, host.starts)
        assertEquals("t3", manager.bearerToken())
        // Later 401s for the old token are now stale -> no further restart.
        notifier.onSessionExpired("t2")
        advanceUntilIdle()
        assertEquals(1, host.starts)
    }

    // Cold start: stored T1, the freshly started server minted T2. A 401 for T1
    // adopts T2 with no restart.
    @Test
    fun `cold start with a stale stored token adopts the running token without a restart`() = runTest {
        val notifier = SessionExpiryNotifier()
        val manager = DefaultSessionManager(FakeTokenStorage(), FakeSessionPrefsStorage())
        manager.activateLocalProfile("t1")
        val host = CountingHost(runningToken = "t2")
        val reminter = com.mycorrhizal.crm.data.local.LocalSessionReminter(host, manager)

        SessionExpiryWiring(notifier, manager, localRemint = { reminter.remint(it) }).start(this)
        notifier.onSessionExpired("t1")
        advanceUntilIdle()

        assertEquals(0, host.stops)
        assertEquals(0, host.starts)
        assertEquals("t2", manager.bearerToken())
    }

    // Remote profile: the bearer plumbing changes nothing -- the grant refresh
    // still runs and a failed refresh still clears.
    @Test
    fun `a remote profile still refreshes then clears regardless of the rejected bearer`() = runTest {
        val notifier = SessionExpiryNotifier()
        val manager = DefaultSessionManager(FakeTokenStorage(), FakeSessionPrefsStorage())
        manager.setSession("https://crm.example.com", "jwt-1", SessionState(userId = 7))
        var refreshes = 0
        val host = CountingHost(runningToken = null)
        val reminter = com.mycorrhizal.crm.data.local.LocalSessionReminter(host, manager)

        SessionExpiryWiring(
            notifier,
            manager,
            refresher = { refreshes++; false },
            localRemint = { reminter.remint(it) },
        ).start(this)
        notifier.onSessionExpired("jwt-1")
        advanceUntilIdle()

        assertEquals(1, refreshes)
        assertEquals(0, host.starts + host.stops)
        assertNull(manager.bearerToken())
    }

    private class CountingHost(
        private val runningToken: String?,
        private val mintsOnStart: String = "minted",
    ) : com.mycorrhizal.crm.data.local.LocalServerHost {
        var starts = 0
        var stops = 0

        override suspend fun ensureStarted() = Result.success(
            com.mycorrhizal.crm.data.local.LocalServerEndpoint("/sock", mintsOnStart).also { starts++ },
        )

        override suspend fun stop() {
            stops++
        }

        override suspend fun deleteLocalData() = Unit

        override fun socketPathIfRunning(): String? = null

        override fun sessionTokenIfRunning(): String? = runningToken
    }
}
