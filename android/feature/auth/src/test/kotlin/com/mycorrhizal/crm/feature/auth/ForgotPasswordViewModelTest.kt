package com.mycorrhizal.crm.feature.auth

import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.just
import io.mockk.mockk
import io.mockk.runs
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class ForgotPasswordViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val authRepository = mockk<AuthRepository>()
    private val sessionManager = mockk<SessionManager>()

    private fun viewModel(): ForgotPasswordViewModel {
        coEvery { sessionManager.serverUrl() } returns "https://crm.example.com"
        coEvery { sessionManager.setServerUrl(any()) } just runs
        return ForgotPasswordViewModel(authRepository, sessionManager)
    }

    @Test
    fun `requesting a reset advances to the confirm step with the server message`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { authRepository.requestPasswordReset("alice@example.com") } returns Result.success(
                "If an account exists, password reset instructions were sent",
            )
            val vm = viewModel()
            advanceUntilIdle()

            vm.onEmailChange("alice@example.com")
            vm.requestReset()
            advanceUntilIdle()

            assertEquals(PasswordResetStep.CONFIRM, vm.uiState.value.step)
            assertEquals("If an account exists, password reset instructions were sent", vm.uiState.value.requestMessage)
            coVerify { authRepository.requestPasswordReset("alice@example.com") }
        }

    @Test
    fun `a blank email blocks the request`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = viewModel()
            advanceUntilIdle()

            vm.requestReset()
            advanceUntilIdle()

            assertTrue(vm.uiState.value.errorRes != null)
            coVerify(exactly = 0) { authRepository.requestPasswordReset(any()) }
        }

    @Test
    fun `a failed request stays on the request step and is retryable`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { authRepository.requestPasswordReset(any()) } returns
                Result.failure(com.mycorrhizal.crm.network.ApiError.Client(400, "invalid email"))
            val vm = viewModel()
            advanceUntilIdle()

            vm.onEmailChange("bad")
            vm.requestReset()
            advanceUntilIdle()

            assertEquals(PasswordResetStep.REQUEST, vm.uiState.value.step)
            assertEquals("invalid email", vm.uiState.value.error)
        }

    @Test
    fun `confirming a reset with a token and matching passwords completes the flow`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { authRepository.requestPasswordReset(any()) } returns Result.success("sent")
            coEvery { authRepository.confirmPasswordReset("token-1", "newpass123") } returns Result.success(Unit)
            val vm = viewModel()
            advanceUntilIdle()
            vm.onEmailChange("alice@example.com")
            vm.requestReset()
            advanceUntilIdle()

            vm.onTokenChange("token-1")
            vm.confirmReset("token-1", "newpass123", "newpass123")
            advanceUntilIdle()

            assertEquals(PasswordResetStep.DONE, vm.uiState.value.step)
            coVerify { authRepository.confirmPasswordReset("token-1", "newpass123") }
        }

    @Test
    fun `mismatched confirm passwords block the reset`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { authRepository.requestPasswordReset(any()) } returns Result.success("sent")
            val vm = viewModel()
            advanceUntilIdle()
            vm.onEmailChange("alice@example.com")
            vm.requestReset()
            advanceUntilIdle()

            vm.confirmReset("token-1", "newpass123", "different")
            advanceUntilIdle()

            assertEquals(PasswordResetStep.CONFIRM, vm.uiState.value.step)
            coVerify(exactly = 0) { authRepository.confirmPasswordReset(any(), any()) }
        }
}
