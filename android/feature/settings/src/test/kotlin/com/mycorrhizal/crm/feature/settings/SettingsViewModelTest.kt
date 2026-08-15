package com.mycorrhizal.crm.feature.settings

import android.content.Context
import com.mycorrhizal.crm.domain.repository.AppSettingsRepository
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.SessionState
import com.mycorrhizal.crm.domain.repository.TrackingSettingsRepository
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import com.mycorrhizal.crm.ui.R
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class SettingsViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val authRepository = mockk<AuthRepository>()
    private val trackingSettings = mockk<TrackingSettingsRepository>()
    private val appSettings = mockk<AppSettingsRepository>()
    private val appContext = mockk<Context>(relaxed = true)

    private fun viewModel(
        session: SessionState = SessionState(),
        themePreference: String = AppSettingsRepository.THEME_SYSTEM,
    ): SettingsViewModel {
        coEvery { trackingSettings.callTrackingEnabled() } returns false
        coEvery { trackingSettings.smsTrackingEnabled() } returns false
        coEvery { trackingSettings.notificationsEnabled() } returns true
        every { authRepository.observeSession() } returns MutableStateFlow(session)
        coEvery { appSettings.themePreference() } returns flowOf(themePreference)
        return SettingsViewModel(authRepository, trackingSettings, appSettings, appContext)
    }

    @Test
    fun `exposes the current session`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = viewModel(
            SessionState(serverUrl = "https://crm.example.com", username = "alice", isAdmin = true, language = "en"),
        )
        advanceUntilIdle()

        val state = vm.uiState.value
        assertEquals("https://crm.example.com", state.session.serverUrl)
        assertEquals("alice", state.session.username)
        assertTrue(state.session.isAdmin)
    }

    @Test
    fun `logout clears the session and emits LoggedOut`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { authRepository.logout() } returns Unit
        val vm = viewModel(
            SessionState(serverUrl = "https://crm.example.com", username = "alice", isLoggedIn = true),
        )
        advanceUntilIdle()

        vm.logout()
        advanceUntilIdle()

        coVerify { authRepository.logout() }
        assertEquals(SettingsEvent.LoggedOut, vm.events.value)
    }

    @Test
    fun `tracking toggles persist their preference`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { trackingSettings.setCallTrackingEnabled(true) } returns Unit
        coEvery { trackingSettings.setSmsTrackingEnabled(true) } returns Unit
        val vm = viewModel()
        advanceUntilIdle()

        vm.setCallTrackingEnabled(true)
        vm.setSmsTrackingEnabled(true)
        advanceUntilIdle()

        assertTrue(vm.uiState.value.callTrackingEnabled)
        assertTrue(vm.uiState.value.smsTrackingEnabled)
        coVerify { trackingSettings.setCallTrackingEnabled(true) }
        coVerify { trackingSettings.setSmsTrackingEnabled(true) }
    }

    // --- M25 ---

    @Test
    fun `updateLanguage persists server-side, caches locally and emits LocaleChanged`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { authRepository.updateLanguage("de") } returns Result.success(Unit)
            coEvery { appSettings.setLanguageOverride("de") } returns Unit
            val vm = viewModel(SessionState(language = "en"))
            advanceUntilIdle()

            vm.updateLanguage("de")
            advanceUntilIdle()

            coVerify { authRepository.updateLanguage("de") }
            coVerify { appSettings.setLanguageOverride("de") }
            assertEquals(SettingsEvent.LocaleChanged, vm.events.value)
        }

    @Test
    fun `updateLanguage to the current value is a no-op`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = viewModel(SessionState(language = "de"))
        advanceUntilIdle()

        vm.updateLanguage("de")
        advanceUntilIdle()

        coVerify(exactly = 0) { authRepository.updateLanguage(any()) }
        assertNull(vm.events.value)
    }

    @Test
    fun `updateDateFormat persists server-side`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { authRepository.updateDateFormat("us") } returns Result.success(Unit)
        val vm = viewModel(SessionState(dateFormat = "eu"))
        advanceUntilIdle()

        vm.updateDateFormat("us")
        advanceUntilIdle()

        coVerify { authRepository.updateDateFormat("us") }
        assertNull(vm.events.value)
    }

    @Test
    fun `setThemePreference persists locally`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { appSettings.setThemePreference(AppSettingsRepository.THEME_DARK) } returns Unit
        val vm = viewModel()
        advanceUntilIdle()

        vm.setThemePreference(AppSettingsRepository.THEME_DARK)
        advanceUntilIdle()

        coVerify { appSettings.setThemePreference(AppSettingsRepository.THEME_DARK) }
    }

    @Test
    fun `changePassword success logs out and emits PasswordChanged`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { authRepository.changePassword("old", "new-pass") } returns Result.success(Unit)
            coEvery { authRepository.logout() } returns Unit
            val vm = viewModel()
            advanceUntilIdle()

            vm.changePassword("old", "new-pass", "new-pass")
            advanceUntilIdle()

            coVerify { authRepository.changePassword("old", "new-pass") }
            coVerify { authRepository.logout() }
            assertEquals(SettingsEvent.PasswordChanged, vm.events.value)
            assertNull(vm.uiState.value.passwordError)
        }

    @Test
    fun `changePassword with a wrong current password surfaces the server error`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { authRepository.changePassword("wrong", "new-pass") } returns
                Result.failure(ApiError.Client(400, "Invalid value for field 'current_password'"))
            val vm = viewModel()
            advanceUntilIdle()

            vm.changePassword("wrong", "new-pass", "new-pass")
            advanceUntilIdle()

            assertEquals("Invalid value for field 'current_password'", vm.uiState.value.passwordError)
            assertNull(vm.events.value)
        }

    @Test
    fun `changePassword with mismatched confirm never reaches the server`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = viewModel()
            advanceUntilIdle()

            vm.changePassword("old", "new-pass", "different")
            advanceUntilIdle()

            coVerify(exactly = 0) { authRepository.changePassword(any(), any()) }
            assertEquals(R.string.settings_password_mismatch, vm.uiState.value.passwordErrorRes)
            assertNull(vm.uiState.value.passwordError)
        }

    @Test
    fun `themePreference flows into the ui state`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = viewModel(themePreference = AppSettingsRepository.THEME_DARK)
        advanceUntilIdle()

        assertEquals(AppSettingsRepository.THEME_DARK, vm.uiState.value.themePreference)
    }
}
