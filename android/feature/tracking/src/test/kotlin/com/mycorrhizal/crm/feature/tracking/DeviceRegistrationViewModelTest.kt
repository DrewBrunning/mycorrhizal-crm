package com.mycorrhizal.crm.feature.tracking

import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.domain.repository.SessionState
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Rule
import org.junit.Test

/**
 * M5 §5a (issue #152): the token-registration ViewModel — a login transition
 * registers the device, a logout transition deletes it, and only transitions
 * count (a session that never changes logs nothing twice).
 */
class DeviceRegistrationViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    @Test
    fun `login registers and logout deletes`() = runTest(mainDispatcherRule.testDispatcher) {
        val sessionFlow = MutableStateFlow(SessionState())
        val sessionManager = mockk<SessionManager>()
        every { sessionManager.observeSession() } returns sessionFlow
        val manager = mockk<DeviceRegistrationManager>()
        coEvery { manager.register(any()) } returns Result.success(Unit)
        coEvery { manager.delete() } returns Result.success(Unit)

        DeviceRegistrationViewModel(sessionManager, manager)
        advanceUntilIdle()
        coVerify(exactly = 0) { manager.register(any()) }

        sessionFlow.value = SessionState(isLoggedIn = true)
        advanceUntilIdle()
        coVerify(exactly = 1) { manager.register(any()) }

        sessionFlow.value = SessionState(isLoggedIn = false)
        advanceUntilIdle()
        coVerify(exactly = 1) { manager.delete() }

        // A second login registers again (tokens rotate between sessions).
        sessionFlow.value = SessionState(isLoggedIn = true)
        advanceUntilIdle()
        coVerify(exactly = 2) { manager.register(any()) }
    }

    @Test
    fun `starting already logged in registers exactly once`() = runTest(mainDispatcherRule.testDispatcher) {
        val sessionFlow = MutableStateFlow(SessionState(isLoggedIn = true))
        val sessionManager = mockk<SessionManager>()
        every { sessionManager.observeSession() } returns sessionFlow
        val manager = mockk<DeviceRegistrationManager>()
        coEvery { manager.register(any()) } returns Result.success(Unit)

        DeviceRegistrationViewModel(sessionManager, manager)
        advanceUntilIdle()

        // The single logged-in emission registers once (a fresh install/launch
        // must (re)enroll the device), but stays idle for identical re-emissions.
        coVerify(exactly = 1) { manager.register(any()) }
    }

    @Test
    fun `a session that never changes logs nothing`() = runTest(mainDispatcherRule.testDispatcher) {
        val sessionFlow = MutableStateFlow(SessionState(isLoggedIn = true))
        val sessionManager = mockk<SessionManager>()
        every { sessionManager.observeSession() } returns sessionFlow
        val manager = mockk<DeviceRegistrationManager>()
        coEvery { manager.register(any()) } returns Result.success(Unit)

        DeviceRegistrationViewModel(sessionManager, manager)
        advanceUntilIdle()
        advanceUntilIdle()

        coVerify(exactly = 1) { manager.register(any()) }
        coVerify(exactly = 0) { manager.delete() }
    }
}
