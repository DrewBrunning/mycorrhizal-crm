package com.mycorrhizal.crm

import com.mycorrhizal.crm.data.session.DefaultSessionManager
import com.mycorrhizal.crm.data.session.OIDC_PENDING_REQUEST_TTL_MILLIS
import com.mycorrhizal.crm.data.session.OidcPendingRequest
import com.mycorrhizal.crm.data.session.OidcPendingRequestStore
import com.mycorrhizal.crm.data.session.SessionPrefsStorage
import com.mycorrhizal.crm.data.session.TokenStorage
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.model.network.UserProfile
import com.mycorrhizal.crm.network.ApiError
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Issue #965: the security-critical OIDC native-return decisions are exercised
 * here without an Activity (the same "factor the logic out of MainActivity"
 * split the JaCoCo excludes assume). The start half asserts only the state +
 * one-way S256 challenge go into the URL; the return half asserts a callback is
 * redeemed only when it matches a live request this app started.
 */
class OidcLoginCoordinatorTest {

    private class FakeTokenStorage : TokenStorage {
        var stored: String? = null
        override suspend fun save(token: String) {
            stored = token
        }
        override suspend fun load(): String? = stored
        override suspend fun clear() {
            stored = null
        }
    }

    private class FakePrefsStorage : SessionPrefsStorage {
        var url: String? = null
        override suspend fun save(serverUrl: String?) {
            url = serverUrl
        }
        override suspend fun loadServerUrl(): String? = url
        override suspend fun clear() {
            url = null
        }
    }

    private class FakePendingStore : OidcPendingRequestStore {
        var request: OidcPendingRequest? = null
        var clearCount = 0
        override suspend fun save(state: String, codeVerifier: String) {
            request = OidcPendingRequest(state, codeVerifier, System.currentTimeMillis())
        }
        override suspend fun load(): OidcPendingRequest? = request
        override suspend fun clear() {
            request = null
            clearCount++
        }
    }

    private class Harness {
        val tokenStorage = FakeTokenStorage()
        val prefs = FakePrefsStorage()
        val session = DefaultSessionManager(tokenStorage, prefs)
        val pending = FakePendingStore()
        val auth = mockk<AuthRepository>()
        var now = 1_000L
        val launched = mutableListOf<String>()

        fun coordinator(): OidcLoginCoordinator = OidcLoginCoordinator(
            sessionManager = session,
            authRepository = auth,
            pendingStore = pending,
            launchBrowser = { launched += it },
            nowMillis = { now },
        )

        suspend fun withServer() {
            session.init()
            session.setServerUrl("https://crm.example.com")
        }
    }

    private fun seedPending(h: Harness, state: String = "state-a", verifier: String = "verifier-a") {
        h.pending.request = OidcPendingRequest(state, verifier, h.now)
    }

    // --- start ---

    @Test
    fun `start persists the binding and opens the auth URL with only state and S256 challenge`() = runTest {
        val h = Harness()

        h.coordinator().start("https://crm.example.com/")

        val pending = h.pending.request
        requireNotNull(pending)
        assertEquals(1, h.launched.size)
        val url = h.launched.single()
        assertTrue(
            "must target the native OIDC login route",
            url.startsWith("https://crm.example.com/api/v1/auth/oidc/login?client=android"),
        )
        assertTrue("must echo the generated state", url.contains("state=${pending.state}"))
        assertTrue(
            "must send the S256 challenge, never the verifier",
            url.contains("code_challenge=${OidcPkce.challenge(pending.codeVerifier)}"),
        )
        assertTrue(url.contains("code_challenge_method=S256"))
        assertFalse("the verifier must never reach the browser", url.contains(pending.codeVerifier))
    }

    // --- onCallback: ignored/failed returns ---

    @Test
    fun `a non-callback URI is ignored and leaves the pending request alone`() = runTest {
        val h = Harness()
        seedPending(h)

        val outcome = h.coordinator().onCallback(null)

        assertEquals(OidcCallbackOutcome.Ignored, outcome)
        assertEquals(0, h.pending.clearCount)
        requireNotNull(h.pending.request)
    }

    @Test
    fun `an error return clears the pending request and reports failure`() = runTest {
        val h = Harness()
        seedPending(h)

        val outcome = h.coordinator().onCallback(OidcReturn.Failure)

        assertEquals(OidcCallbackOutcome.Failed, outcome)
        assertNull(h.pending.request)
    }

    // --- onCallback: binding checks ---

    @Test
    fun `a success with no pending request is refused without exchanging`() = runTest {
        val h = Harness()
        h.withServer()

        val outcome = h.coordinator().onCallback(success(state = "state-a"))

        assertEquals(OidcCallbackOutcome.Failed, outcome)
        coVerify(exactly = 0) { h.auth.completeOidcNativeLogin(any(), any()) }
        assertFalse(h.session.observeSession().first().isLoggedIn)
    }

    @Test
    fun `a success with a mismatched state is refused without exchanging`() = runTest {
        val h = Harness()
        h.withServer()
        seedPending(h, state = "state-from-this-app")

        val outcome = h.coordinator().onCallback(success(state = "attacker-state"))

        assertEquals(OidcCallbackOutcome.Failed, outcome)
        coVerify(exactly = 0) { h.auth.completeOidcNativeLogin(any(), any()) }
        assertFalse(h.session.observeSession().first().isLoggedIn)
    }

    @Test
    fun `a success past the pending-request TTL is refused`() = runTest {
        val h = Harness()
        h.withServer()
        seedPending(h)
        h.now += OIDC_PENDING_REQUEST_TTL_MILLIS + 1

        val outcome = h.coordinator().onCallback(success(state = "state-a"))

        assertEquals(OidcCallbackOutcome.Failed, outcome)
        coVerify(exactly = 0) { h.auth.completeOidcNativeLogin(any(), any()) }
    }

    @Test
    fun `a success without a server URL is refused`() = runTest {
        val h = Harness()
        h.session.init() // hydrated, but no server URL persisted
        seedPending(h)

        val outcome = h.coordinator().onCallback(success(state = "state-a"))

        assertEquals(OidcCallbackOutcome.Failed, outcome)
        coVerify(exactly = 0) { h.auth.completeOidcNativeLogin(any(), any()) }
    }

    // --- onCallback: redemption ---

    @Test
    fun `a matching success redeems the code with the on-device verifier and persists the session`() = runTest {
        val h = Harness()
        h.withServer()
        seedPending(h, state = "state-a", verifier = "verifier-a")
        coEvery { h.auth.completeOidcNativeLogin("code-1", "verifier-a") } returns Result.success("jwt-1")
        coEvery { h.auth.fetchCurrentUser() } returns
            Result.success(UserProfile(id = 7, username = "alice", isAdmin = true))

        val outcome = h.coordinator().onCallback(
            OidcReturn.Success(state = "state-a", code = "code-1", language = "de", dateFormat = "eu"),
        )

        assertEquals(OidcCallbackOutcome.Succeeded, outcome)
        assertEquals("jwt-1", h.tokenStorage.stored)
        val session = h.session.observeSession().first()
        assertTrue(session.isLoggedIn)
        assertEquals(7, session.userId)
        assertEquals("alice", session.username)
        assertEquals("de", session.language)
        assertEquals("eu", session.dateFormat)
    }

    @Test
    fun `a rejected code leaves the session logged out`() = runTest {
        val h = Harness()
        h.withServer()
        seedPending(h)
        coEvery { h.auth.completeOidcNativeLogin(any(), any()) } returns
            Result.failure(ApiError.Client(401, "Invalid or expired authorization code"))

        val outcome = h.coordinator().onCallback(success(state = "state-a"))

        assertEquals(OidcCallbackOutcome.Failed, outcome)
        assertNull(h.tokenStorage.stored)
        assertFalse(h.session.observeSession().first().isLoggedIn)
    }

    @Test
    fun `a failed profile fetch clears the just-minted session`() = runTest {
        val h = Harness()
        h.withServer()
        seedPending(h)
        coEvery { h.auth.completeOidcNativeLogin(any(), any()) } returns Result.success("jwt-1")
        coEvery { h.auth.fetchCurrentUser() } returns Result.failure(ApiError.Client(401, "expired"))

        val outcome = h.coordinator().onCallback(success(state = "state-a"))

        assertEquals(OidcCallbackOutcome.Failed, outcome)
        assertNull(h.tokenStorage.stored)
        assertFalse(h.session.observeSession().first().isLoggedIn)
    }

    private fun success(state: String): OidcReturn.Success =
        OidcReturn.Success(state = state, code = "code-1", language = null, dateFormat = null)
}
