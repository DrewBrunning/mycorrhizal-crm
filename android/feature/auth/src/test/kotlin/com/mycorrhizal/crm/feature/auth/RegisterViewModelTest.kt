package com.mycorrhizal.crm.feature.auth

import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.model.network.PasswordStrength
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.just
import io.mockk.mockk
import io.mockk.runs
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class RegisterViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val authRepository = mockk<AuthRepository>()
    private val sessionManager = mockk<SessionManager>()

    private fun viewModel(): RegisterViewModel {
        coEvery { sessionManager.serverUrl() } returns "https://crm.example.com"
        coEvery { sessionManager.setServerUrl(any()) } just runs
        return RegisterViewModel(authRepository, sessionManager)
    }

    private fun weakStrength() = PasswordStrength(isValid = false, score = 1, feedback = "Password is too short")
    private fun strongStrength() = PasswordStrength(isValid = true, score = 4, feedback = "Password is very strong")

    @Test
    fun `a known-weak password blocks submit without calling register`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { authRepository.checkPasswordStrength("short") } returns Result.success(weakStrength())
            val vm = viewModel()
            advanceUntilIdle()

            vm.onUsernameChange("alice")
            vm.onEmailChange("alice@example.com")
            vm.onPasswordChange("short")
            advanceUntilIdle()
            assertTrue(vm.uiState.value.passwordChecked)

            vm.submit()
            advanceUntilIdle()

            assertFalse(vm.uiState.value.isLoading)
            assertEquals("Password is too short", vm.uiState.value.error)
            coVerify(exactly = 0) { authRepository.register(any(), any(), any()) }
        }

    @Test
    fun `a strong password submits register and then auto-logs-in`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { authRepository.checkPasswordStrength("hunter2hunter2") } returns Result.success(strongStrength())
            coEvery { authRepository.register("alice", "alice@example.com", "hunter2hunter2") } returns Result.success(Unit)
            coEvery { authRepository.login("alice@example.com", "hunter2hunter2") } returns Result.success(Unit)
            val vm = viewModel()
            advanceUntilIdle()

            vm.onUsernameChange("alice")
            vm.onEmailChange("alice@example.com")
            vm.onPasswordChange("hunter2hunter2")
            advanceUntilIdle()

            vm.submit()
            advanceUntilIdle()

            coVerify { authRepository.register("alice", "alice@example.com", "hunter2hunter2") }
            coVerify { authRepository.login("alice@example.com", "hunter2hunter2") }
            assertEquals(RegisterEvent.Registered, vm.events.first())
        }

    @Test
    fun `a duplicate account surfaces the server message and does not login`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { authRepository.checkPasswordStrength("hunter2hunter2") } returns Result.success(strongStrength())
            coEvery { authRepository.register(any(), any(), any()) } returns
                Result.failure(ApiError.Client(409, "User already exists"))
            val vm = viewModel()
            advanceUntilIdle()

            vm.onUsernameChange("alice")
            vm.onEmailChange("alice@example.com")
            vm.onPasswordChange("hunter2hunter2")
            advanceUntilIdle()

            vm.submit()
            advanceUntilIdle()

            assertFalse(vm.uiState.value.isLoading)
            assertEquals("User already exists", vm.uiState.value.error)
            coVerify(exactly = 0) { authRepository.login(any(), any()) }
        }

    @Test
    fun `a failed auto-login after a successful register surfaces the error instead of pretending`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { authRepository.checkPasswordStrength("hunter2hunter2") } returns Result.success(strongStrength())
            coEvery { authRepository.register(any(), any(), any()) } returns Result.success(Unit)
            coEvery { authRepository.login(any(), any()) } returns
                Result.failure(ApiError.Client(500, "boom"))
            val vm = viewModel()
            advanceUntilIdle()

            vm.onUsernameChange("alice")
            vm.onEmailChange("alice@example.com")
            vm.onPasswordChange("hunter2hunter2")
            advanceUntilIdle()

            vm.submit()
            advanceUntilIdle()

            assertEquals("boom", vm.uiState.value.error)
            assertTrue(vm.uiState.value.isLoading.not())
        }

    @Test
    fun `blank fields block submit with a localized error`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = viewModel()
            advanceUntilIdle()

            vm.submit()
            advanceUntilIdle()

            assertTrue(vm.uiState.value.errorRes != null)
            coVerify(exactly = 0) { authRepository.register(any(), any(), any()) }
        }

    @Test
    fun `an invalid server url blocks submit`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { authRepository.checkPasswordStrength(any()) } returns Result.success(strongStrength())
            val vm = viewModel()
            advanceUntilIdle()
            vm.onServerUrlChange("not a url")

            vm.onUsernameChange("alice")
            vm.onEmailChange("alice@example.com")
            vm.onPasswordChange("hunter2hunter2")
            vm.submit()
            advanceUntilIdle()

            assertEquals(com.mycorrhizal.crm.ui.R.string.login_error_valid_server_url, vm.uiState.value.errorRes)
            coVerify(exactly = 0) { authRepository.register(any(), any(), any()) }
        }
}
