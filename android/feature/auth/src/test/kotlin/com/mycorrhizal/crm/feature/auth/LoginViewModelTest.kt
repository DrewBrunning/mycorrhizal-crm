package com.mycorrhizal.crm.feature.auth

import app.cash.turbine.test
import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.SessionState
import com.mycorrhizal.crm.domain.usecase.LoginUseCase
import com.mycorrhizal.crm.domain.usecase.LoginWithApiTokenUseCase
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class LoginViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private class Harness(
        val viewModel: LoginViewModel,
        val authRepository: AuthRepository,
        val sessionManager: SessionManager,
    )

    private fun harness(): Harness {
        val authRepository = mockk<AuthRepository>()
        coEvery { authRepository.observeSession() } returns MutableStateFlow(SessionState())
        val sessionManager = mockk<SessionManager>()
        every { sessionManager.observeSession() } returns MutableStateFlow(SessionState())
        coEvery { sessionManager.setServerUrl(any()) } returns Unit
        val viewModel = LoginViewModel(
            loginUseCase = LoginUseCase(authRepository),
            loginWithApiTokenUseCase = LoginWithApiTokenUseCase(authRepository),
            sessionManager = sessionManager,
        )
        return Harness(viewModel, authRepository, sessionManager)
    }

    @Test
    fun `initial state is empty`() {
        val h = harness()
        val state = h.viewModel.uiState.value
        assertFalse(state.isLoading)
        assertEquals("", state.serverUrl)
    }

    @Test
    fun `submit with blank server url shows error`() = runTest(mainDispatcherRule.testDispatcher) {
        val h = harness()
        h.viewModel.onSubmit()

        assertEquals("Server URL is required", h.viewModel.uiState.value.error)
    }

    @Test
    fun `blank identifier shows validation error without calling repository`() = runTest(mainDispatcherRule.testDispatcher) {
        val h = harness()
        h.viewModel.onServerUrlChange("https://crm.example.com")
        h.viewModel.onSubmit()
        advanceUntilIdle()

        assertEquals("Username or email is required", h.viewModel.uiState.value.error)
        coVerify(exactly = 0) { h.authRepository.login(any(), any()) }
    }

    @Test
    fun `successful password login emits LoggedIn`() = runTest(mainDispatcherRule.testDispatcher) {
        val h = harness()
        coEvery { h.authRepository.login("alice", "secret") } returns Result.success(Unit)

        h.viewModel.onServerUrlChange("https://crm.example.com/")
        h.viewModel.onIdentifierChange("alice")
        h.viewModel.onPasswordChange("secret")

        h.viewModel.events.test {
            h.viewModel.onSubmit()
            assertTrue(awaitItem() is LoginEvent.ServerUrlUpdated)
            assertTrue(awaitItem() is LoginEvent.LoggedIn)
        }
        // Trailing slash is trimmed before persisting.
        coVerify { h.sessionManager.setServerUrl("https://crm.example.com") }
    }

    @Test
    fun `failed password login surfaces the error message`() = runTest(mainDispatcherRule.testDispatcher) {
        val h = harness()
        coEvery { h.authRepository.login(any(), any()) } returns Result.failure(
            Exception("Invalid credentials"),
        )

        h.viewModel.onServerUrlChange("https://crm.example.com")
        h.viewModel.onIdentifierChange("alice")
        h.viewModel.onPasswordChange("wrong")
        h.viewModel.onSubmit()
        advanceUntilIdle()

        // LoginUseCase maps the failure to its message.
        assertEquals("Invalid credentials", h.viewModel.uiState.value.error)
        assertFalse(h.viewModel.uiState.value.isLoading)
    }

    @Test
    fun `api token mode validates the token prefix`() = runTest(mainDispatcherRule.testDispatcher) {
        val h = harness()
        h.viewModel.onServerUrlChange("https://crm.example.com")
        h.viewModel.onModeChange(LoginMode.API_TOKEN)
        h.viewModel.onApiTokenChange("not-a-token")
        h.viewModel.onSubmit()
        advanceUntilIdle()

        assertEquals("API tokens start with 'mycorrhizal_'", h.viewModel.uiState.value.error)
        coVerify(exactly = 0) { h.authRepository.loginWithApiToken(any()) }
    }

    @Test
    fun `successful api token login emits LoggedIn`() = runTest(mainDispatcherRule.testDispatcher) {
        val h = harness()
        coEvery { h.authRepository.loginWithApiToken("mycorrhizal_abc") } returns Result.success(Unit)

        h.viewModel.onServerUrlChange("https://crm.example.com")
        h.viewModel.onModeChange(LoginMode.API_TOKEN)
        h.viewModel.onApiTokenChange("mycorrhizal_abc")

        h.viewModel.events.test {
            h.viewModel.onSubmit()
            assertTrue(awaitItem() is LoginEvent.ServerUrlUpdated)
            assertTrue(awaitItem() is LoginEvent.LoggedIn)
        }
    }
}
