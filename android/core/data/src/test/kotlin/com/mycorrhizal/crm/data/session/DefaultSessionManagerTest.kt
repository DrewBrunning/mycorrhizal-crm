package com.mycorrhizal.crm.data.session

import com.mycorrhizal.crm.domain.repository.SessionState
import app.cash.turbine.test
import kotlinx.coroutines.async
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class DefaultSessionManagerTest {

    private fun manager(): Pair<DefaultSessionManager, FakeTokenStorage> {
        val tokenStorage = FakeTokenStorage()
        val manager = DefaultSessionManager(tokenStorage, FakeSessionPrefsStorage())
        return manager to tokenStorage
    }

    @Test
    fun `bearerToken is empty before init`() {
        val (manager, _) = manager()
        assertNull(manager.bearerToken())
    }

    @Test
    fun `init hydrates the token from storage`() = runTest {
        val (manager, tokenStorage) = manager()
        tokenStorage.stored = "stored-jwt"
        manager.init()

        assertEquals("stored-jwt", manager.bearerToken())
    }

    @Test
    fun `setSession persists token and flips isLoggedIn`() = runTest {
        val (manager, tokenStorage) = manager()
        manager.init()
        manager.setServerUrl("https://crm.example.com")
        manager.setSession(
            serverUrl = "https://crm.example.com",
            token = "jwt-1",
            state = SessionState(userId = 7, username = "alice"),
        )

        assertEquals("jwt-1", manager.bearerToken())
        assertEquals("jwt-1", tokenStorage.stored)
        assertEquals("https://crm.example.com", manager.baseUrl())
        val state = manager.observeSession().first()
        assertTrue(state.isLoggedIn)
        assertEquals(7, state.userId)
    }

    @Test
    fun `clearSession removes the token and profile but keeps the server url`() = runTest {
        val (manager, tokenStorage) = manager()
        manager.setSession(
            serverUrl = "https://crm.example.com",
            token = "jwt-1",
            state = SessionState(userId = 7, username = "alice"),
        )
        manager.clearSession()

        assertNull(manager.bearerToken())
        assertNull(tokenStorage.stored)
        // Issue #723: the server URL is non-credential device config — logout
        // keeps it so the login screen can pre-fill it (in memory AND prefs).
        assertEquals("https://crm.example.com", manager.serverUrl())
        assertEquals("https://crm.example.com", manager.baseUrl())
        val state = manager.observeSession().first()
        assertFalse(state.isLoggedIn)
        assertEquals("https://crm.example.com", state.serverUrl)
    }

    @Test
    fun `clearSession keepServerUrl=false wipes the server url too`() = runTest {
        val (manager, tokenStorage) = manager()
        manager.setSession(
            serverUrl = "https://crm.example.com",
            token = "jwt-1",
            state = SessionState(userId = 7, username = "alice"),
        )

        manager.clearSession(keepServerUrl = false)

        assertNull(manager.bearerToken())
        assertNull(tokenStorage.stored)
        assertNull(manager.serverUrl())
        assertEquals(SessionState(), manager.observeSession().first())
    }

    @Test
    fun `clearSession keeps the server url across a process restart`() = runTest {
        val tokenStorage = FakeTokenStorage()
        val prefsStorage = FakeSessionPrefsStorage()
        val first = DefaultSessionManager(tokenStorage, prefsStorage)
        first.setSession("https://crm.example.com", "jwt-1", SessionState(userId = 7))

        first.clearSession()

        // A fresh manager (new process) hydrates the retained URL — logout
        // must not force the user to re-type it after an app relaunch.
        val restarted = DefaultSessionManager(tokenStorage, prefsStorage)
        restarted.init()
        assertEquals("https://crm.example.com", restarted.serverUrl())
        assertNull(restarted.bearerToken())
    }

    @Test
    fun `clearSession wipes cached user data through the session data cleaner`() = runTest {
        val tokenStorage = FakeTokenStorage()
        val cleaner = RecordingSessionDataCleaner()
        val manager = DefaultSessionManager(tokenStorage, FakeSessionPrefsStorage(), cleaner)
        manager.setSession("https://crm.example.com", "jwt-1", SessionState(userId = 7))

        manager.clearSession()

        assertEquals(1, cleaner.clearCount)
    }

    @Test
    fun `clearSession does not require a session data cleaner`() = runTest {
        val manager = DefaultSessionManager(FakeTokenStorage(), FakeSessionPrefsStorage())
        manager.setSession("https://crm.example.com", "jwt-1", SessionState(userId = 7))

        manager.clearSession()

        assertNull(manager.bearerToken())
    }

    private class RecordingSessionDataCleaner : SessionDataCleaner {
        var clearCount = 0
        override suspend fun clear() {
            clearCount++
        }
    }

    // Issue #957 (finding #1): the FCM-deregistration bug was that
    // clearSession() dropped the bearer BEFORE the reactive listener that
    // deregisters the device ever ran, so the request went out unauthenticated
    // and 401ed. These pin the fix: SessionTeardown.beforeClear must see the
    // still-valid session, must run exactly once per real clear, must not run
    // again for a redundant/already-logged-out clear (no request to make), and
    // a teardown failure must never block the local clear itself.

    @Test
    fun `clearSession runs the teardown step while the session is still authenticated`() = runTest {
        // A fake that reads the manager's own bearerToken() at call time --
        // the same thing AuthInterceptor reads -- proves teardown genuinely
        // runs before the token is nulled, not just "before the flow emits".
        var tokenSeenDuringTeardown: String? = null
        lateinit var manager: DefaultSessionManager
        manager = DefaultSessionManager(
            FakeTokenStorage(),
            FakeSessionPrefsStorage(),
            sessionTeardown = SessionTeardown { tokenSeenDuringTeardown = manager.bearerToken() },
        )
        manager.setSession("https://crm.example.com", "jwt-1", SessionState(userId = 7))

        manager.clearSession()

        assertEquals("jwt-1", tokenSeenDuringTeardown)
        assertNull("the token must still be dropped after teardown runs", manager.bearerToken())
    }

    @Test
    fun `a teardown failure does not block the local clear`() = runTest {
        val manager = DefaultSessionManager(
            FakeTokenStorage(),
            FakeSessionPrefsStorage(),
            sessionTeardown = SessionTeardown { error("network unreachable") },
        )
        manager.setSession("https://crm.example.com", "jwt-1", SessionState(userId = 7))

        manager.clearSession()

        assertNull("the local clear must still happen despite the teardown throwing", manager.bearerToken())
        assertFalse(manager.observeSession().first().isLoggedIn)
    }

    @Test
    fun `clearSession on an already-logged-out session makes no teardown call`() = runTest {
        var calls = 0
        val manager = DefaultSessionManager(
            FakeTokenStorage(),
            FakeSessionPrefsStorage(),
            sessionTeardown = SessionTeardown { calls++ },
        )

        manager.clearSession()

        assertEquals("no active session means nothing to tear down", 0, calls)
    }

    @Test
    fun `a redundant clearSession call after a real one makes no second teardown call`() = runTest {
        var calls = 0
        val manager = DefaultSessionManager(
            FakeTokenStorage(),
            FakeSessionPrefsStorage(),
            sessionTeardown = SessionTeardown { calls++ },
        )
        manager.setSession("https://crm.example.com", "jwt-1", SessionState(userId = 7))

        manager.clearSession()
        manager.clearSession()

        assertEquals(1, calls)
    }

    @Test
    fun `isClearingSession is true only for the duration of the teardown call`() = runTest {
        lateinit var manager: DefaultSessionManager
        var duringTeardown = false
        manager = DefaultSessionManager(
            FakeTokenStorage(),
            FakeSessionPrefsStorage(),
            sessionTeardown = SessionTeardown { duringTeardown = manager.isClearingSession() },
        )
        manager.setSession("https://crm.example.com", "jwt-1", SessionState(userId = 7))
        assertFalse("not clearing before logout", manager.isClearingSession())

        manager.clearSession()

        assertTrue("the flag must have been up while teardown ran", duringTeardown)
        assertFalse("the flag must drop back down once clearSession returns", manager.isClearingSession())
    }

    @Test
    fun `setServerUrl persists the origin`() = runTest {
        val (manager, _) = manager()
        manager.setServerUrl("https://beta.example.com")

        assertEquals("https://beta.example.com", manager.baseUrl())
        assertEquals("https://beta.example.com", manager.serverUrl())
    }

    @Test
    fun `setProfile merges profile without clearing login`() = runTest {
        val (manager, _) = manager()
        manager.setSession("https://crm.example.com", "jwt", SessionState(username = "alice"))
        manager.setProfile(SessionState(isAdmin = true, language = "de"))

        val state = manager.observeSession().first()
        assertTrue(state.isLoggedIn)
        assertTrue(state.isAdmin)
        assertEquals("de", state.language)
        assertEquals("alice", state.username)
    }

    // Issue #814: a token_version bump (2FA confirm/disable) re-issues the
    // session as a fresh bearer token. setToken swaps it in place — the
    // session stays logged in and the server URL/profile are untouched.
    @Test
    fun `setToken replaces the token in place without disturbing the session`() = runTest {
        val (manager, tokenStorage) = manager()
        manager.setSession(
            serverUrl = "https://crm.example.com",
            token = "old-jwt",
            state = SessionState(userId = 7, username = "alice"),
        )

        manager.setToken("reissued-jwt")

        assertEquals("reissued-jwt", manager.bearerToken())
        assertEquals("reissued-jwt", tokenStorage.stored)
        assertEquals("https://crm.example.com", manager.baseUrl())
        val state = manager.observeSession().first()
        assertTrue(state.isLoggedIn)
        assertEquals(7, state.userId)
        assertEquals("alice", state.username)
    }

    // M5 §5: a cold-start OIDC deep link must await hydration before reading
    // the server URL / writing the session — otherwise it races the async
    // startup init() (review-pass fix).
    @Test
    fun `awaitHydrated suspends until init completes`() = runTest {
        val (manager, tokenStorage) = manager()
        tokenStorage.stored = "stored-jwt"

        var hydrated = false
        // The async job's only observable effect is `hydrated` flipping to
        // true once awaitHydrated returns; the Deferred itself is never read.
        async { manager.awaitHydrated(); hydrated = true }

        // Not yet hydrated: init hasn't run, await is still suspended.
        advanceUntilIdle()
        assertFalse(hydrated)

        manager.init()
        advanceUntilIdle()

        assertTrue(hydrated)
        assertEquals("stored-jwt", manager.bearerToken())
    }

    // Issue #678: the session state machine must walk the full lifecycle —
    // logged-out → authenticated → (401) → logged-out → re-authenticated —
    // emitting the right SessionState at every step, so the auth-flow branch
    // in the app is driven by real state and no stale authed UI survives a
    // cleared session.
    @Test
    fun `session walks the full lifecycle state machine`() = runTest {
        val (manager, _) = manager()

        manager.observeSession().test {
            // logged-out (initial)
            assertEquals(SessionState(), awaitItem())

            // authenticated
            manager.setSession(
                serverUrl = "https://crm.example.com",
                token = "jwt-1",
                state = SessionState(userId = 7, username = "alice"),
            )
            val authenticated = awaitItem()
            assertTrue(authenticated.isLoggedIn)
            assertEquals(7, authenticated.userId)
            assertEquals("alice", authenticated.username)

            // (401) → logged-out
            manager.clearSession()
            val loggedOut = awaitItem()
            assertFalse(loggedOut.isLoggedIn)
            assertNull(loggedOut.userId)
            assertFalse("no stale username survives logout", loggedOut.username != null)
            // Issue #723: the server URL is not part of the dropped session —
            // it survives logout so the login screen can pre-fill it.
            assertEquals("https://crm.example.com", loggedOut.serverUrl)

            // re-authenticated
            manager.setSession(
                serverUrl = "https://crm.example.com",
                token = "jwt-2",
                state = SessionState(userId = 7, username = "alice"),
            )
            val reAuthenticated = awaitItem()
            assertTrue(reAuthenticated.isLoggedIn)
            assertEquals("jwt-2", manager.bearerToken())
            assertEquals("alice", reAuthenticated.username)
        }
    }

    // Issue #678: process-death restore — a fresh manager instance (a new
    // process) must hydrate the persisted token and surface the same
    // logged-in state the old instance held. Only the token + server URL are
    // persisted; the profile fields (userId/username/admin) are refetched from
    // the server after restore (AuthRepositoryImpl re-derives them on login).
    @Test
    fun `a fresh instance restores the session after process death`() = runTest {
        val tokenStorage = FakeTokenStorage()
        val prefsStorage = FakeSessionPrefsStorage()
        val first = DefaultSessionManager(tokenStorage, prefsStorage)
        first.setSession(
            serverUrl = "https://crm.example.com",
            token = "jwt-1",
            state = SessionState(userId = 7, username = "alice"),
        )

        val restarted = DefaultSessionManager(tokenStorage, prefsStorage)
        restarted.init()

        assertEquals("jwt-1", restarted.bearerToken())
        assertEquals("https://crm.example.com", restarted.baseUrl())
        val state = restarted.observeSession().first()
        assertTrue(state.isLoggedIn)
    }
}
