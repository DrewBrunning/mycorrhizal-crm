package com.mycorrhizal.crm

import com.mycorrhizal.crm.data.compat.DefaultServerCapabilitiesStore
import com.mycorrhizal.crm.data.session.AppLockController
import com.mycorrhizal.crm.data.session.AppLockState
import com.mycorrhizal.crm.data.session.DefaultSessionManager
import com.mycorrhizal.crm.data.session.ProfilesSnapshot
import com.mycorrhizal.crm.data.session.SessionPrefsStorage
import com.mycorrhizal.crm.data.session.TokenStorage
import com.mycorrhizal.crm.domain.profile.ServerProfile
import com.mycorrhizal.crm.domain.profile.ServerProfileKind
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.ServerCompatibilityRepository
import com.mycorrhizal.crm.domain.repository.SessionState
import com.mycorrhizal.crm.model.network.ServerHealth
import com.mycorrhizal.crm.model.network.UserProfile
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import java.io.IOException

/**
 * Issue #1400: a RESTORED session (no login call) must get its username /
 * isAdmin / language / dateFormat from `GET /users/me`, tolerate being offline,
 * and never show the previous profile's fields after a profile switch. Uses
 * the real [DefaultSessionManager] so the restore + switch transitions are the
 * production ones.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class MainViewModelProfileHydrationTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private class Tokens : TokenStorage {
        val tokens = mutableMapOf<String, String>()
        override suspend fun save(profileId: String, token: String) { tokens[profileId] = token }
        override suspend fun load(profileId: String): String? = tokens[profileId]
        override suspend fun clear(profileId: String) { tokens.remove(profileId) }
        override suspend fun loadLegacy(): String? = null
        override suspend fun clearLegacy() = Unit
    }

    private class Prefs : SessionPrefsStorage {
        var snapshot = ProfilesSnapshot()
        override suspend fun save(serverUrl: String?) = Unit
        override suspend fun loadServerUrl(): String? = null
        override suspend fun saveProfiles(snapshot: ProfilesSnapshot) { this.snapshot = snapshot }
        override suspend fun loadProfiles(): ProfilesSnapshot = snapshot
        override suspend fun clear() { snapshot = ProfilesSnapshot() }
    }

    private val admin = UserProfile(
        id = 1, username = "admin-alice", isAdmin = true, language = "de", dateFormat = "DD.MM.YYYY",
        selfContactVCardUid = "uid-1",
    )
    private val plain = UserProfile(id = 2, username = "bob", isAdmin = false, language = "en", dateFormat = "YYYY-MM-DD")

    /** Two stored, signed-in profiles; "a" is active -> a restored session. */
    private suspend fun restoredManager(): DefaultSessionManager {
        val tokens = Tokens().apply { this.tokens["a"] = "jwt-a"; this.tokens["b"] = "jwt-b" }
        val prefs = Prefs().apply {
            snapshot = ProfilesSnapshot(
                profiles = listOf(
                    ServerProfile("a", ServerProfileKind.Remote("https://a.example"), "a"),
                    ServerProfile("b", ServerProfileKind.Remote("https://b.example"), "b"),
                ),
                activeProfileId = "a",
            )
        }
        return DefaultSessionManager(tokens, prefs).also { it.init() }
    }

    private fun vm(manager: DefaultSessionManager, auth: AuthRepository): MainViewModel {
        val lock = mockk<AppLockController> { every { state } returns MutableStateFlow(AppLockState.Resolving) }
        val compat = mockk<ServerCompatibilityRepository> {
            coEvery { getServerHealth() } returns Result.success(ServerHealth())
        }
        return MainViewModel(manager, lock, compat, auth, DefaultServerCapabilitiesStore(), mockk(relaxed = true))
    }

    @Test
    fun `a restored session is hydrated from users-me`() = runTest(mainDispatcherRule.testDispatcher) {
        val manager = restoredManager()
        val auth = mockk<AuthRepository> { coEvery { fetchCurrentUser() } returns Result.success(admin) }

        val viewModel = vm(manager, auth)
        advanceUntilIdle()

        val s = viewModel.session.value
        assertTrue(s.isLoggedIn)
        assertEquals("admin-alice", s.username)
        assertTrue(s.isAdmin)
        assertEquals("de", s.language)
        assertEquals("DD.MM.YYYY", s.dateFormat)
        assertEquals(1, s.userId)
        assertEquals("uid-1", s.selfContactVCardUid)
        coVerify(exactly = 1) { auth.fetchCurrentUser() }
    }

    @Test
    fun `an offline restore keeps the session logged in and does not retry-loop`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val manager = restoredManager()
            val auth = mockk<AuthRepository> {
                coEvery { fetchCurrentUser() } returns Result.failure(IOException("offline"))
            }

            val viewModel = vm(manager, auth)
            advanceUntilIdle()

            val s = viewModel.session.value
            assertTrue(s.isLoggedIn)
            assertNull(s.username)
            assertFalse(s.isAdmin)
            coVerify(exactly = 1) { auth.fetchCurrentUser() }
        }

    @Test
    fun `a throwing fetch does not crash or log out`() = runTest(mainDispatcherRule.testDispatcher) {
        val manager = restoredManager()
        val auth = mockk<AuthRepository> { coEvery { fetchCurrentUser() } throws IllegalStateException("boom") }

        val viewModel = vm(manager, auth)
        advanceUntilIdle()

        assertTrue(viewModel.session.value.isLoggedIn)
    }

    @Test
    fun `a profile switch replaces the fields and never shows the old isAdmin`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val manager = restoredManager()
            val bobGate = CompletableDeferred<Result<UserProfile>>()
            val auth = mockk<AuthRepository>()
            coEvery { auth.fetchCurrentUser() } returns Result.success(admin) coAndThen { bobGate.await() }

            val viewModel = vm(manager, auth)
            advanceUntilIdle()
            assertTrue(viewModel.session.value.isAdmin)

            manager.switchProfile("b")
            advanceUntilIdle()
            // In flight: the previous profile's identity must already be gone.
            val during = viewModel.session.value
            assertNull(during.username)
            assertFalse(during.isAdmin)

            bobGate.complete(Result.success(plain))
            advanceUntilIdle()
            val after = viewModel.session.value
            assertEquals("bob", after.username)
            assertFalse(after.isAdmin)
            assertEquals("en", after.language)
        }

    @Test
    fun `a response for a profile that is no longer active is dropped`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val manager = restoredManager()
            val aGate = CompletableDeferred<Result<UserProfile>>()
            val auth = mockk<AuthRepository>()
            coEvery { auth.fetchCurrentUser() } coAnswers { aGate.await() } andThen Result.success(plain)

            val viewModel = vm(manager, auth)
            advanceUntilIdle()
            manager.switchProfile("b")
            advanceUntilIdle()
            assertEquals("bob", viewModel.session.value.username)

            aGate.complete(Result.success(admin))
            advanceUntilIdle()
            assertEquals("bob", viewModel.session.value.username)
            assertFalse(viewModel.session.value.isAdmin)
        }

    @Test
    fun `an already-populated session is not refetched`() = runTest(mainDispatcherRule.testDispatcher) {
        val manager = restoredManager()
        manager.setProfile(SessionState(username = "alice", isAdmin = true))
        val auth = mockk<AuthRepository>()

        vm(manager, auth)
        advanceUntilIdle()

        coVerify(exactly = 0) { auth.fetchCurrentUser() }
    }
}
