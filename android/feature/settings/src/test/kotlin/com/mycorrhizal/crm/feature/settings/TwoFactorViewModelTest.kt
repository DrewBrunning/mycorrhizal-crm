package com.mycorrhizal.crm.feature.settings

import android.content.Context
import com.mycorrhizal.crm.data.passkey.PasskeyResult
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.PasskeyRepository
import com.mycorrhizal.crm.domain.repository.SecondFactorProof
import com.mycorrhizal.crm.model.network.WebAuthnCredential
import com.mycorrhizal.crm.testing.FakePasskeyClient
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.model.network.MessageResponse
import com.mycorrhizal.crm.model.network.TwoFactorConfirmResponse
import com.mycorrhizal.crm.model.network.TwoFactorSetupResponse
import com.mycorrhizal.crm.model.network.TwoFactorStatusResponse
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class TwoFactorViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val authRepository = mockk<AuthRepository>()
    private val passkeyRepository = mockk<PasskeyRepository>()
    private val passkeyClient = FakePasskeyClient()
    private val context = mockk<Context>(relaxed = true)

    private val phone = WebAuthnCredential("id-phone", "Phone", "2026-09-01T10:00:00Z", null)

    private fun enabledVm(
        enabled: Boolean,
        passkeys: List<WebAuthnCredential> = emptyList(),
        available: Boolean = true,
    ): TwoFactorViewModel {
        coEvery { authRepository.getTwoFactorStatus() } returns
            Result.success(TwoFactorStatusResponse(enabled = enabled))
        coEvery { passkeyRepository.listPasskeys() } returns Result.success(passkeys)
        return TwoFactorViewModel(authRepository, passkeyRepository, passkeyClient) { available }
    }

    @Test
    fun `load reflects 2fa disabled`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = enabledVm(false)
        advanceUntilIdle()

        val state = vm.uiState.value
        assertFalse(state.loading)
        assertEquals(false, state.enabled)
        assertEquals(null, state.error)
    }

    @Test
    fun `load reflects 2fa enabled`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = enabledVm(true)
        advanceUntilIdle()

        assertEquals(true, vm.uiState.value.enabled)
    }

    @Test
    fun `load failure surfaces the error and leaves enabled unknown`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { authRepository.getTwoFactorStatus() } returns
            Result.failure(ApiError.Server(500, "boom"))
        coEvery { passkeyRepository.listPasskeys() } returns Result.success(emptyList())
        val vm = TwoFactorViewModel(authRepository, passkeyRepository, passkeyClient) { true }
        advanceUntilIdle()

        val state = vm.uiState.value
        assertFalse(state.loading)
        assertEquals(null, state.enabled)
        assertEquals("Server error (500)", state.error)
    }

    @Test
    fun `startSetup surfaces the pending secret and otpauth url`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = enabledVm(false)
        advanceUntilIdle()
        coEvery { authRepository.setupTwoFactor() } returns Result.success(
            TwoFactorSetupResponse(secret = "JBSWY3DPEHPK3PXP", otpauthUrl = "otpauth://totp/x?secret=JBSWY3DPEHPK3PXP"),
        )

        vm.startSetup()
        advanceUntilIdle()

        val state = vm.uiState.value
        assertEquals("JBSWY3DPEHPK3PXP", state.setup?.secret)
        assertFalse(state.busy)
    }

    @Test
    fun `setup on an oidc account surfaces the server's 403 message`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = enabledVm(false)
        advanceUntilIdle()
        coEvery { authRepository.setupTwoFactor() } returns Result.failure(
            ApiError.Client(403, "Two-factor authentication is unavailable for accounts that sign in with SSO"),
        )

        vm.startSetup()
        advanceUntilIdle()

        val state = vm.uiState.value
        assertEquals("Two-factor authentication is unavailable for accounts that sign in with SSO", state.error)
        assertNull(state.setup)
    }

    @Test
    fun `confirmSetup enables 2fa and shows the recovery codes exactly once`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = enabledVm(false)
        advanceUntilIdle()
        coEvery { authRepository.setupTwoFactor() } returns Result.success(
            TwoFactorSetupResponse(secret = "JBSWY3DPEHPK3PXP", otpauthUrl = "otpauth://totp/x"),
        )
        coEvery { authRepository.confirmTwoFactor("123456") } returns Result.success(
            TwoFactorConfirmResponse(message = "Two-factor authentication enabled", recoveryCodes = listOf("AAAAA-BBBBB-CCCCC")),
        )

        vm.startSetup()
        advanceUntilIdle()
        vm.confirmSetup("123456")
        advanceUntilIdle()

        val state = vm.uiState.value
        assertEquals(true, state.enabled)
        assertNull(state.setup)
        assertEquals(listOf("AAAAA-BBBBB-CCCCC"), state.recoveryCodes)
        assertFalse(state.busy)
    }

    @Test
    fun `a wrong code while confirming keeps the wizard open with a localized error`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = enabledVm(false)
        advanceUntilIdle()
        coEvery { authRepository.setupTwoFactor() } returns Result.success(
            TwoFactorSetupResponse(secret = "JBSWY3DPEHPK3PXP", otpauthUrl = "otpauth://totp/x"),
        )
        coEvery { authRepository.confirmTwoFactor(any()) } returns Result.failure(
            ApiError.Client(400, "Invalid value for field 'code'"),
        )

        vm.startSetup()
        advanceUntilIdle()
        vm.confirmSetup("000000")
        advanceUntilIdle()

        val state = vm.uiState.value
        assertNotNull(state.setup)
        assertEquals(com.mycorrhizal.crm.ui.R.string.settings_two_factor_invalid_code, state.errorRes)
        assertFalse(state.busy)
    }

    @Test
    fun `disabling with a live code turns 2fa off`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = enabledVm(true)
        advanceUntilIdle()
        coEvery { authRepository.disableTwoFactor("654321") } returns Result.success(
            MessageResponse(message = "Two-factor authentication disabled"),
        )

        vm.requestDisable()
        assertEquals(TwoFactorPrompt.DISABLE, vm.uiState.value.prompt)
        vm.submitPromptCode("654321")
        advanceUntilIdle()

        val state = vm.uiState.value
        assertEquals(false, state.enabled)
        assertNull(state.prompt)
        assertFalse(state.busy)
        coVerify { authRepository.disableTwoFactor("654321") }
    }

    @Test
    fun `regenerating recovery codes shows the fresh set exactly once`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = enabledVm(true)
        advanceUntilIdle()
        coEvery { authRepository.regenerateRecoveryCodes("123456") } returns Result.success(
            TwoFactorConfirmResponse(recoveryCodes = listOf("NEWAA-BBBBB-CCCCC")),
        )

        vm.requestRegenerate()
        vm.submitPromptCode("123456")
        advanceUntilIdle()

        val state = vm.uiState.value
        assertEquals(listOf("NEWAA-BBBBB-CCCCC"), state.recoveryCodes)
        assertNull(state.prompt)
        assertFalse(state.busy)
    }

    @Test
    fun `a wrong code while disabling keeps the prompt with a localized error`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = enabledVm(true)
        advanceUntilIdle()
        coEvery { authRepository.disableTwoFactor(any()) } returns Result.failure(
            ApiError.Client(400, "Invalid value for field 'code'"),
        )

        vm.requestDisable()
        vm.submitPromptCode("000000")
        advanceUntilIdle()

        val state = vm.uiState.value
        assertEquals(TwoFactorPrompt.DISABLE, state.prompt)
        assertEquals(com.mycorrhizal.crm.ui.R.string.settings_two_factor_invalid_code, state.errorRes)
        assertFalse(state.busy)
    }

    @Test
    fun `dismissing the recovery codes clears them`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = enabledVm(true)
        advanceUntilIdle()
        coEvery { authRepository.regenerateRecoveryCodes(any()) } returns Result.success(
            TwoFactorConfirmResponse(recoveryCodes = listOf("NEWAA-BBBBB-CCCCC")),
        )

        vm.requestRegenerate()
        vm.submitPromptCode("123456")
        advanceUntilIdle()
        assertNotNull(vm.uiState.value.recoveryCodes)

        vm.dismissRecoveryCodes()

        assertNull(vm.uiState.value.recoveryCodes)
    }

    @Test
    fun `closing the setup wizard drops the pending secret`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = enabledVm(false)
        advanceUntilIdle()
        coEvery { authRepository.setupTwoFactor() } returns Result.success(
            TwoFactorSetupResponse(secret = "JBSWY3DPEHPK3PXP", otpauthUrl = "otpauth://totp/x"),
        )

        vm.startSetup()
        advanceUntilIdle()
        assertNotNull(vm.uiState.value.setup)

        vm.closeSetup()

        assertNull(vm.uiState.value.setup)
        // The secret is transient — nothing is left in state after closing.
        assertNull(vm.uiState.value.error)
    }

    @Test
    fun `dismissing a code prompt clears it`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = enabledVm(true)
        advanceUntilIdle()

        vm.requestRegenerate()
        assertEquals(TwoFactorPrompt.REGENERATE, vm.uiState.value.prompt)
        vm.dismissPrompt()

        assertNull(vm.uiState.value.prompt)
    }

    @Test
    fun `a blank prompt code is ignored without calling the repository`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = enabledVm(true)
        advanceUntilIdle()

        vm.requestDisable()
        vm.submitPromptCode("   ")
        advanceUntilIdle()

        assertFalse(vm.uiState.value.busy)
        coVerify(exactly = 0) { authRepository.disableTwoFactor(any()) }
        coVerify(exactly = 0) { authRepository.regenerateRecoveryCodes(any()) }
    }

    @Test
    fun `onErrorShown clears the error`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = enabledVm(false)
        advanceUntilIdle()
        coEvery { authRepository.setupTwoFactor() } returns Result.failure(
            ApiError.Client(403, "Two-factor authentication is unavailable for accounts that sign in with SSO"),
        )

        vm.startSetup()
        advanceUntilIdle()
        assertNotNull(vm.uiState.value.error)

        vm.onErrorShown()

        assertNull(vm.uiState.value.error)
        assertNull(vm.uiState.value.errorRes)
    }

    // --- issue #1337: enabling TOTP on a passkey-only account needs a live proof ---

    private val setupResponse = TwoFactorSetupResponse(secret = "JBSWY3DPEHPK3PXP", otpauthUrl = "otpauth://totp/x")

    @Test
    fun `load records a passkey and the gate`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = enabledVm(false, passkeys = listOf(phone))
        advanceUntilIdle()
        assertTrue(vm.uiState.value.hasPasskey)
        assertTrue(vm.uiState.value.passkeysAvailable)
        assertTrue(vm.uiState.value.canProveWithPasskey)
    }

    @Test
    fun `a failed passkey lookup leaves setup proof-free`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { authRepository.getTwoFactorStatus() } returns Result.success(TwoFactorStatusResponse(enabled = false))
        coEvery { passkeyRepository.listPasskeys() } returns Result.failure(ApiError.Server(500, "boom"))
        val vm = TwoFactorViewModel(authRepository, passkeyRepository, passkeyClient) { true }
        advanceUntilIdle()
        assertFalse(vm.uiState.value.hasPasskey)
        assertNull(vm.uiState.value.error)
    }

    @Test
    fun `startSetup with a passkey asks for a proof and does not touch the server`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = enabledVm(false, passkeys = listOf(phone))
            advanceUntilIdle()

            vm.startSetup()
            advanceUntilIdle()

            assertTrue(vm.uiState.value.proofPrompt)
            assertNull(vm.uiState.value.setup)
            coVerify(exactly = 0) { authRepository.setupTwoFactor(any()) }
        }

    @Test
    fun `a recovery code proves setup`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = enabledVm(false, passkeys = listOf(phone))
        advanceUntilIdle()
        coEvery { authRepository.setupTwoFactor(SecondFactorProof.Code("AAAAA-BBBBB-CCCCC")) } returns
            Result.success(setupResponse)

        vm.startSetup()
        vm.submitSetupProofCode("  AAAAA-BBBBB-CCCCC ")
        advanceUntilIdle()

        assertEquals(setupResponse, vm.uiState.value.setup)
        assertFalse(vm.uiState.value.proofPrompt)
        assertFalse(vm.uiState.value.busy)
    }

    @Test
    fun `a rejected proof keeps the proof dialog open with the localized message`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = enabledVm(false, passkeys = listOf(phone))
            advanceUntilIdle()
            coEvery { authRepository.setupTwoFactor(any()) } returns Result.failure(ApiError.Client(400, "Invalid code"))

            vm.startSetup()
            vm.submitSetupProofCode("000000")
            advanceUntilIdle()

            val state = vm.uiState.value
            assertTrue(state.proofPrompt)
            assertNull(state.setup)
            assertEquals(R.string.settings_two_factor_invalid_code, state.errorRes)
            assertFalse(state.busy)
        }

    @Test
    fun `a locked-out proof shows the server message`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = enabledVm(false, passkeys = listOf(phone))
        advanceUntilIdle()
        coEvery { authRepository.setupTwoFactor(any()) } returns Result.failure(ApiError.Client(429, "locked"))

        vm.startSetup()
        vm.submitSetupProofCode("000000")
        advanceUntilIdle()

        assertTrue(vm.uiState.value.proofPrompt)
        assertNotNull(vm.uiState.value.error)
    }

    @Test
    fun `a blank code or a call outside the proof dialog is ignored`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = enabledVm(false, passkeys = listOf(phone))
        advanceUntilIdle()

        vm.submitSetupProofCode("123456") // dialog not open yet
        vm.startSetup()
        vm.submitSetupProofCode("   ")
        vm.dismissSetupProof()
        assertFalse(vm.uiState.value.proofPrompt)
        vm.submitSetupProofWithPasskey(context) // dialog closed again
        advanceUntilIdle()

        coVerify(exactly = 0) { authRepository.setupTwoFactor(any()) }
        coVerify(exactly = 0) { passkeyRepository.beginProof(any()) }
    }

    @Test
    fun `an existing passkey proves setup`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = enabledVm(false, passkeys = listOf(phone))
        advanceUntilIdle()
        coEvery { passkeyRepository.beginProof(null) } returns Result.success("""{"publicKey":{}}""")
        passkeyClient.getResult = PasskeyResult.Success("""{"id":"asserted"}""")
        coEvery { authRepository.setupTwoFactor(SecondFactorProof.Assertion("""{"id":"asserted"}""")) } returns
            Result.success(setupResponse)

        vm.startSetup()
        vm.submitSetupProofWithPasskey(context)
        advanceUntilIdle()

        assertEquals(listOf("""{"publicKey":{}}"""), passkeyClient.requested)
        assertEquals(setupResponse, vm.uiState.value.setup)
        assertFalse(vm.uiState.value.proofPrompt)
    }

    @Test
    fun `a passkey proof that yields nothing stops before setup`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = enabledVm(false, passkeys = listOf(phone))
        advanceUntilIdle()
        coEvery { passkeyRepository.beginProof(null) } returns Result.success("{}")
        vm.startSetup()

        passkeyClient.getResult = PasskeyResult.Cancelled
        vm.submitSetupProofWithPasskey(context)
        advanceUntilIdle()
        assertNull(vm.uiState.value.errorRes)
        assertFalse(vm.uiState.value.busy)

        passkeyClient.getResult = PasskeyResult.NoMatchingPasskey
        vm.submitSetupProofWithPasskey(context)
        advanceUntilIdle()
        assertEquals(R.string.settings_passkeys_no_other_here, vm.uiState.value.errorRes)

        passkeyClient.getResult = PasskeyResult.NotAssociated
        vm.submitSetupProofWithPasskey(context)
        advanceUntilIdle()
        assertEquals(R.string.settings_passkeys_not_associated, vm.uiState.value.errorRes)

        passkeyClient.getResult = PasskeyResult.NoProvider
        vm.submitSetupProofWithPasskey(context)
        advanceUntilIdle()
        assertEquals(R.string.settings_passkeys_no_provider, vm.uiState.value.errorRes)

        passkeyClient.getResult = PasskeyResult.Failed("boom")
        vm.submitSetupProofWithPasskey(context)
        advanceUntilIdle()
        assertEquals(R.string.settings_passkeys_invalid_proof, vm.uiState.value.errorRes)

        passkeyClient.getResult = PasskeyResult.AlreadyRegistered
        vm.submitSetupProofWithPasskey(context)
        advanceUntilIdle()
        assertEquals(R.string.settings_passkeys_invalid_proof, vm.uiState.value.errorRes)

        assertTrue(vm.uiState.value.proofPrompt)
        coVerify(exactly = 0) { authRepository.setupTwoFactor(any()) }
    }

    @Test
    fun `a failed proof begin surfaces the server message`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = enabledVm(false, passkeys = listOf(phone))
        advanceUntilIdle()
        coEvery { passkeyRepository.beginProof(null) } returns Result.failure(ApiError.Client(409, "No passkey is registered"))

        vm.startSetup()
        vm.submitSetupProofWithPasskey(context)
        advanceUntilIdle()

        assertNotNull(vm.uiState.value.error)
        assertTrue(passkeyClient.requested.isEmpty())
    }

    @Test
    fun `the passkey route is a no-op when the ceremony gate is closed`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = enabledVm(false, passkeys = listOf(phone), available = false)
        advanceUntilIdle()
        assertFalse(vm.uiState.value.canProveWithPasskey)

        vm.startSetup()
        vm.submitSetupProofWithPasskey(context)
        advanceUntilIdle()

        coVerify(exactly = 0) { passkeyRepository.beginProof(any()) }
    }

    @Test
    fun `dismissing the proof dialog clears it and its error`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = enabledVm(false, passkeys = listOf(phone))
        advanceUntilIdle()
        coEvery { authRepository.setupTwoFactor(any()) } returns Result.failure(ApiError.Client(400, "bad"))
        vm.startSetup()
        vm.submitSetupProofCode("000000")
        advanceUntilIdle()
        assertNotNull(vm.uiState.value.errorRes)

        vm.dismissSetupProof()

        assertFalse(vm.uiState.value.proofPrompt)
        assertNull(vm.uiState.value.errorRes)
    }
}
