package com.mycorrhizal.crm.data.session

import com.mycorrhizal.crm.domain.repository.SessionState
import kotlinx.coroutines.flow.first
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
}
