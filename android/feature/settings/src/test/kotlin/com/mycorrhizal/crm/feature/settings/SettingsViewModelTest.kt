package com.mycorrhizal.crm.feature.settings

import android.content.Context
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.SessionState
import com.mycorrhizal.crm.domain.repository.TrackingSettingsRepository
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class SettingsViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val authRepository = mockk<AuthRepository>()
    private val trackingSettings = mockk<TrackingSettingsRepository>()
    private val appContext = mockk<Context>(relaxed = true)

    private fun viewModel(): SettingsViewModel {
        coEvery { trackingSettings.callTrackingEnabled() } returns false
        coEvery { trackingSettings.smsTrackingEnabled() } returns false
        coEvery { trackingSettings.notificationsEnabled() } returns true
        return SettingsViewModel(authRepository, trackingSettings, appContext)
    }

    @Test
    fun `exposes the current session`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { authRepository.observeSession() } returns MutableStateFlow(
            SessionState(serverUrl = "https://crm.example.com", username = "alice", isAdmin = true, language = "en"),
        )

        val vm = viewModel()
        advanceUntilIdle()

        val state = vm.uiState.value
        assertEquals("https://crm.example.com", state.session.serverUrl)
        assertEquals("alice", state.session.username)
        assertTrue(state.session.isAdmin)
    }

    @Test
    fun `logout clears the session and emits LoggedOut`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { authRepository.observeSession() } returns MutableStateFlow(
            SessionState(serverUrl = "https://crm.example.com", username = "alice", isLoggedIn = true),
        )
        coEvery { authRepository.logout() } returns Unit

        val vm = viewModel()
        advanceUntilIdle()

        vm.logout()
        advanceUntilIdle()

        coVerify { authRepository.logout() }
        assertEquals(SettingsEvent.LoggedOut, vm.events.value)
    }

    @Test
    fun `tracking toggles persist their preference`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { authRepository.observeSession() } returns MutableStateFlow(SessionState())
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
}
