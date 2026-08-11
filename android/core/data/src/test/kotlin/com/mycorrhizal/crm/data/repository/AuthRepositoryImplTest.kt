package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.data.session.DefaultSessionManager
import com.mycorrhizal.crm.data.session.FakeSessionPrefsStorage
import com.mycorrhizal.crm.data.session.FakeTokenStorage
import com.mycorrhizal.crm.model.network.UserProfile
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.LoginResult
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class AuthRepositoryImplTest {

    private class Harness {
        val apiClient = mockk<ApiClient>()
        val tokenStorage = FakeTokenStorage()
        val sessionManager = DefaultSessionManager(tokenStorage, FakeSessionPrefsStorage())
        val repository = AuthRepositoryImpl(apiClient, sessionManager)
    }

    @Test
    fun `login persists the token and profile`() = runTest {
        val h = Harness()
        h.sessionManager.setServerUrl("https://crm.example.com")
        coEvery { h.apiClient.login(any(), any()) } returns Result.success(
            LoginResult(token = "jwt-123", language = "en", dateFormat = "eu"),
        )
        coEvery { h.apiClient.currentUser() } returns Result.success(
            UserProfile(id = 7, username = "alice", isAdmin = true, language = "en"),
        )

        val result = h.repository.login("alice", "secret")

        assertTrue(result.isSuccess)
        assertEquals("jwt-123", h.tokenStorage.stored)
        assertEquals("jwt-123", h.sessionManager.bearerToken())
        val state = h.sessionManager.observeSession().first()
        assertTrue(state.isLoggedIn)
        assertEquals(7, state.userId)
        assertEquals("alice", state.username)
        assertTrue(state.isAdmin)
    }

    @Test
    fun `login persists the token before fetching the profile`() = runTest {
        val h = Harness()
        h.sessionManager.setServerUrl("https://crm.example.com")
        coEvery { h.apiClient.login(any(), any()) } returns Result.success(
            LoginResult(token = "jwt-123", language = "en", dateFormat = "eu"),
        )
        // The profile fetch must observe the token already in the session — the
        // AuthInterceptor reads bearerToken() synchronously, so if the token is
        // persisted only after currentUser() the request goes out unauthenticated
        // and the backend answers 401 "Authorization token required".
        var tokenSeenDuringProfileFetch: String? = null
        coEvery { h.apiClient.currentUser() } answers {
            tokenSeenDuringProfileFetch = h.sessionManager.bearerToken()
            Result.success(UserProfile(id = 7, username = "alice"))
        }

        val result = h.repository.login("alice", "secret")

        assertTrue(result.isSuccess)
        assertEquals("jwt-123", tokenSeenDuringProfileFetch)
    }

    @Test
    fun `login propagates failure when server rejects credentials`() = runTest {
        val h = Harness()
        coEvery { h.apiClient.login(any(), any()) } returns Result.failure(
            ApiError.Client(401, "Invalid credentials"),
        )

        val result = h.repository.login("alice", "wrong")

        assertTrue(result.isFailure)
        assertFalse(h.sessionManager.observeSession().first().isLoggedIn)
    }

    @Test
    fun `login fails when no token cookie is returned`() = runTest {
        val h = Harness()
        coEvery { h.apiClient.login(any(), any()) } returns Result.success(
            LoginResult(token = null, language = "en", dateFormat = "eu"),
        )

        val result = h.repository.login("alice", "secret")

        assertTrue(result.isFailure)
        assertFalse(h.sessionManager.observeSession().first().isLoggedIn)
    }

    @Test
    fun `loginWithApiToken stores the token and fetches profile`() = runTest {
        val h = Harness()
        coEvery { h.apiClient.currentUser() } returns Result.success(
            UserProfile(id = 3, username = "bot", isAdmin = false),
        )

        val result = h.repository.loginWithApiToken("mycorrhizal_abc123")

        assertTrue(result.isSuccess)
        assertEquals("mycorrhizal_abc123", h.tokenStorage.stored)
        assertTrue(h.sessionManager.observeSession().first().isLoggedIn)
    }

    @Test
    fun `logout clears the session`() = runTest {
        val h = Harness()
        coEvery { h.apiClient.login(any(), any()) } returns Result.success(
            LoginResult(token = "jwt-123", language = null, dateFormat = null),
        )
        coEvery { h.apiClient.currentUser() } returns Result.success(UserProfile(id = 1))
        h.repository.login("alice", "secret")

        h.repository.logout()

        assertEquals(null, h.tokenStorage.stored)
        assertFalse(h.sessionManager.observeSession().first().isLoggedIn)
    }
}
