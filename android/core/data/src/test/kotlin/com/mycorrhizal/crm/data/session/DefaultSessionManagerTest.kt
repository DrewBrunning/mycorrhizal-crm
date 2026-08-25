package com.mycorrhizal.crm.data.session

import com.mycorrhizal.crm.domain.repository.SessionState
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
    fun `clearSession removes token and resets state`() = runTest {
        val (manager, tokenStorage) = manager()
        manager.setSession(
            serverUrl = "https://crm.example.com",
            token = "jwt-1",
            state = SessionState(userId = 7, username = "alice"),
        )
        manager.clearSession()

        assertNull(manager.bearerToken())
        assertNull(tokenStorage.stored)
        assertFalse(manager.observeSession().first().isLoggedIn)
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
}
