package com.mycorrhizal.crm.feature.settings

import android.content.Context
import com.mycorrhizal.crm.data.passkey.PasskeyResult
import com.mycorrhizal.crm.domain.repository.PasskeyRepository
import com.mycorrhizal.crm.model.network.WebAuthnCredential
import com.mycorrhizal.crm.model.network.WebAuthnRegisterResponse
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.FakePasskeyClient
import com.mycorrhizal.crm.testing.MainDispatcherRule
import com.mycorrhizal.crm.ui.R
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

/** Issue #1293 / ADR 0034: enrollment and management, driven by a fake Credential Manager. */
class PasskeysViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val context = mockk<Context>(relaxed = true)
    private val repo = mockk<PasskeyRepository>()
    private val client = FakePasskeyClient()

    private val phone = WebAuthnCredential("id-phone", "Phone", "2026-09-01T10:00:00Z", null)
    private val key = WebAuthnCredential("id-key", "YubiKey", "2026-09-02T10:00:00Z", "2026-09-03T10:00:00Z")

    private fun TestScope.vm(
        list: List<WebAuthnCredential> = listOf(phone),
        available: Boolean = true,
    ): PasskeysViewModel {
        coEvery { repo.listPasskeys() } returns Result.success(list)
        val vm = PasskeysViewModel(repo, client) { available }
        advanceUntilIdle()
        return vm
    }

    // --- load / gate ---

    @Test
    fun `load lists the credentials and reports the gate`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = vm(listOf(phone, key))
        val state = vm.uiState.value
        assertFalse(state.loading)
        assertTrue(state.available)
        assertEquals(listOf(phone, key), state.passkeys)
        assertTrue(state.canAdd)
    }

    @Test
    fun `a closed gate hides add but still lists`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = vm(available = false)
        assertFalse(vm.uiState.value.canAdd)
        assertEquals(listOf(phone), vm.uiState.value.passkeys)
        vm.startAdd()
        assertFalse(vm.uiState.value.adding)
    }

    @Test
    fun `a list failure surfaces the message`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repo.listPasskeys() } returns Result.failure(ApiError.Server(500, "boom"))
        val vm = PasskeysViewModel(repo, client) { true }
        advanceUntilIdle()
        assertFalse(vm.uiState.value.loading)
        assertEquals("Server error (500)", vm.uiState.value.error)
    }

    // --- enrollment ---

    @Test
    fun `adding a passkey runs begin, the ceremony and finish then refreshes the list`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = vm()
            coEvery { repo.beginRegistration("Laptop") } returns Result.success("""{"publicKey":{"challenge":"c"}}""")
            client.createResult = PasskeyResult.Success("""{"id":"att"}""")
            coEvery { repo.finishRegistration("""{"id":"att"}""") } returns
                Result.success(WebAuthnRegisterResponse("new", "Laptop", "2026-09-29T10:00:00Z", emptyList()))
            coEvery { repo.listPasskeys() } returns Result.success(listOf(phone, key))

            vm.startAdd()
            assertTrue(vm.uiState.value.adding)
            vm.addPasskey(context, "Laptop")
            advanceUntilIdle()

            val state = vm.uiState.value
            assertEquals(listOf("""{"publicKey":{"challenge":"c"}}"""), client.created)
            assertFalse(state.adding)
            assertFalse(state.busy)
            assertEquals(listOf(phone, key), state.passkeys)
            assertEquals(R.string.settings_passkeys_add_success, state.messageRes)
            assertNull(state.recoveryCodes)
        }

    @Test
    fun `the first second factor shows its recovery codes exactly once`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = vm(list = emptyList())
            coEvery { repo.beginRegistration(any()) } returns Result.success("{}")
            coEvery { repo.finishRegistration(any()) } returns
                Result.success(WebAuthnRegisterResponse("new", "P", "2026-09-29T10:00:00Z", listOf("AAAAA-BBBBB-CCCCC")))

            vm.addPasskey(context, "")
            advanceUntilIdle()
            assertEquals(listOf("AAAAA-BBBBB-CCCCC"), vm.uiState.value.recoveryCodes)

            vm.dismissRecoveryCodes()
            assertNull(vm.uiState.value.recoveryCodes)
        }

    @Test
    fun `a cancelled ceremony is silent and never calls finish`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = vm()
        coEvery { repo.beginRegistration(any()) } returns Result.success("{}")
        client.createResult = PasskeyResult.Cancelled

        vm.startAdd()
        vm.addPasskey(context, "x")
        advanceUntilIdle()

        val state = vm.uiState.value
        assertFalse(state.busy)
        assertFalse(state.adding)
        assertNull(state.error)
        assertNull(state.errorRes)
        assertNull(state.messageRes)
        coVerify(exactly = 0) { repo.finishRegistration(any()) }
        assertTrue(state.canAdd)
    }

    @Test
    fun `an unassociated server blocks enrollment persistently`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = vm()
        coEvery { repo.beginRegistration(any()) } returns Result.success("{}")
        client.createResult = PasskeyResult.NotAssociated

        vm.addPasskey(context, "x")
        advanceUntilIdle()

        val state = vm.uiState.value
        assertEquals(R.string.settings_passkeys_not_associated, state.blockedRes)
        assertFalse(state.canAdd)
        vm.startAdd()
        assertFalse(vm.uiState.value.adding)
    }

    @Test
    fun `a missing provider blocks enrollment persistently`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = vm()
        coEvery { repo.beginRegistration(any()) } returns Result.success("{}")
        client.createResult = PasskeyResult.NoProvider

        vm.addPasskey(context, "x")
        advanceUntilIdle()

        assertEquals(R.string.settings_passkeys_no_provider, vm.uiState.value.blockedRes)
        assertFalse(vm.uiState.value.canAdd)
    }

    @Test
    fun `an already registered passkey and generic failures show their message`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = vm()
            coEvery { repo.beginRegistration(any()) } returns Result.success("{}")

            client.createResult = PasskeyResult.AlreadyRegistered
            vm.addPasskey(context, "x")
            advanceUntilIdle()
            assertEquals(R.string.settings_passkeys_already_registered, vm.uiState.value.errorRes)

            vm.onErrorShown()
            client.createResult = PasskeyResult.Failed("boom")
            vm.addPasskey(context, "x")
            advanceUntilIdle()
            assertEquals(R.string.settings_passkeys_add_error, vm.uiState.value.errorRes)
            assertFalse(vm.uiState.value.busy)
        }

    @Test
    fun `an OIDC-provisioned account is refused with the server text and cannot enroll`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = vm()
            val oidc = "Two-factor authentication is not available for accounts that sign in through an identity provider"
            coEvery { repo.beginRegistration(any()) } returns Result.failure(ApiError.Client(403, oidc))

            vm.addPasskey(context, "x")
            advanceUntilIdle()

            val state = vm.uiState.value
            assertEquals(oidc, state.blockedText)
            assertFalse(state.canAdd)
            assertTrue(client.created.isEmpty())
        }

    @Test
    fun `other begin failures show the server message and keep enrollment available`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = vm()
            coEvery { repo.beginRegistration(any()) } returns Result.failure(ApiError.Client(409, "RP not configured"))

            vm.addPasskey(context, "x")
            advanceUntilIdle()

            assertEquals("RP not configured", vm.uiState.value.error)
            assertTrue(vm.uiState.value.canAdd)
        }

    @Test
    fun `a rejected attestation shows the server message`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = vm()
        coEvery { repo.beginRegistration(any()) } returns Result.success("{}")
        coEvery { repo.finishRegistration(any()) } returns Result.failure(ApiError.Client(400, "attestation invalid"))

        vm.addPasskey(context, "x")
        advanceUntilIdle()

        assertEquals("attestation invalid", vm.uiState.value.error)
        assertFalse(vm.uiState.value.busy)
    }

    // --- removal ---

    @Test
    fun `removal with a code succeeds and refreshes`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = vm(listOf(phone, key))
        coEvery { repo.removeWithCode("id-phone", "123456") } returns Result.success(Unit)
        coEvery { repo.listPasskeys() } returns Result.success(listOf(key))

        vm.requestRemove(phone)
        vm.removeWithCode("123456")
        advanceUntilIdle()

        val state = vm.uiState.value
        assertNull(state.removing)
        assertEquals(listOf(key), state.passkeys)
        assertEquals(R.string.settings_passkeys_remove_success, state.messageRes)
    }

    @Test
    fun `a blank code is never submitted`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = vm()
        vm.requestRemove(phone)
        vm.removeWithCode("  ")
        advanceUntilIdle()
        coVerify(exactly = 0) { repo.removeWithCode(any(), any()) }
    }

    @Test
    fun `a rejected proof keeps the dialog with the localized message`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = vm(listOf(phone, key))
        coEvery { repo.removeWithCode(any(), any()) } returns Result.failure(ApiError.Client(400, "bad"))

        vm.requestRemove(phone)
        vm.removeWithCode("000000")
        advanceUntilIdle()

        val state = vm.uiState.value
        assertEquals(phone, state.removing)
        assertEquals(R.string.settings_passkeys_invalid_proof, state.errorRes)
        assertEquals(listOf(phone, key), state.passkeys)
    }

    @Test
    fun `404 and 409 show the server message verbatim`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = vm(listOf(phone, key))
        coEvery { repo.removeWithCode(any(), any()) } returns Result.failure(ApiError.Client(404, "Passkey not found"))
        vm.requestRemove(phone)
        vm.removeWithCode("1")
        advanceUntilIdle()
        assertEquals("Passkey not found", vm.uiState.value.error)

        coEvery { repo.beginProof(any()) } returns Result.failure(ApiError.Client(409, "No other passkey is registered"))
        vm.removeWithAnotherPasskey(context)
        advanceUntilIdle()
        assertEquals("No other passkey is registered", vm.uiState.value.error)
        assertTrue(client.requested.isEmpty())
    }

    @Test
    fun `removal by another passkey passes the removed id as exclude_id and sends the assertion`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = vm(listOf(phone, key))
            coEvery { repo.beginProof("id-phone") } returns Result.success("""{"publicKey":{"challenge":"p"}}""")
            client.getResult = PasskeyResult.Success("""{"id":"other-key"}""")
            coEvery { repo.removeWithAssertion("id-phone", """{"id":"other-key"}""") } returns Result.success(Unit)
            coEvery { repo.listPasskeys() } returns Result.success(listOf(key))

            vm.requestRemove(phone)
            assertTrue(vm.uiState.value.canProveWithPasskey)
            vm.removeWithAnotherPasskey(context)
            advanceUntilIdle()

            coVerify { repo.beginProof("id-phone") }
            assertEquals(listOf("""{"publicKey":{"challenge":"p"}}"""), client.requested)
            assertNull(vm.uiState.value.removing)
            assertEquals(listOf(key), vm.uiState.value.passkeys)
        }

    @Test
    fun `the passkey proof is unavailable with only one passkey`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = vm(listOf(phone))
        vm.requestRemove(phone)
        assertFalse(vm.uiState.value.canProveWithPasskey)

        vm.removeWithAnotherPasskey(context)
        advanceUntilIdle()
        coVerify(exactly = 0) { repo.beginProof(any()) }
    }

    @Test
    fun `the passkey proof is unavailable behind a closed gate`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = vm(listOf(phone, key), available = false)
        vm.requestRemove(phone)
        assertFalse(vm.uiState.value.canProveWithPasskey)
    }

    @Test
    fun `cancelling the proof ceremony is silent and keeps the dialog`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = vm(listOf(phone, key))
        coEvery { repo.beginProof(any()) } returns Result.success("{}")
        client.getResult = PasskeyResult.Cancelled

        vm.requestRemove(phone)
        vm.removeWithAnotherPasskey(context)
        advanceUntilIdle()

        val state = vm.uiState.value
        assertEquals(phone, state.removing)
        assertFalse(state.busy)
        assertNull(state.error)
        assertNull(state.errorRes)
        coVerify(exactly = 0) { repo.removeWithAssertion(any(), any()) }
    }

    @Test
    fun `proof ceremony outcomes map to their messages`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = vm(listOf(phone, key))
        coEvery { repo.beginProof(any()) } returns Result.success("{}")
        vm.requestRemove(phone)

        val expected = listOf(
            PasskeyResult.NoMatchingPasskey to R.string.settings_passkeys_no_other_here,
            PasskeyResult.NotAssociated to R.string.settings_passkeys_not_associated,
            PasskeyResult.NoProvider to R.string.settings_passkeys_no_provider,
            PasskeyResult.Failed("x") to R.string.settings_passkeys_invalid_proof,
            PasskeyResult.AlreadyRegistered to R.string.settings_passkeys_invalid_proof,
        )
        for ((result, res) in expected) {
            client.getResult = result
            vm.removeWithAnotherPasskey(context)
            advanceUntilIdle()
            assertEquals(res, vm.uiState.value.errorRes)
            assertFalse(vm.uiState.value.busy)
        }
    }

    @Test
    fun `a rejected assertion shows the localized proof failure`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = vm(listOf(phone, key))
        coEvery { repo.beginProof(any()) } returns Result.success("{}")
        coEvery { repo.removeWithAssertion(any(), any()) } returns Result.failure(ApiError.Client(400, "bad"))

        vm.requestRemove(phone)
        vm.removeWithAnotherPasskey(context)
        advanceUntilIdle()

        assertEquals(R.string.settings_passkeys_invalid_proof, vm.uiState.value.errorRes)
        assertEquals(phone, vm.uiState.value.removing)
    }

    @Test
    fun `dismissing dialogs and messages clears them`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = vm()
        vm.startAdd()
        vm.dismissAdd()
        assertFalse(vm.uiState.value.adding)
        vm.requestRemove(phone)
        vm.dismissRemove()
        assertNull(vm.uiState.value.removing)
        vm.onMessageShown()
        assertNull(vm.uiState.value.messageRes)
    }

    @Test
    fun `nothing can be started while busy`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = vm()
        coEvery { repo.beginRegistration(any()) } returns Result.success("{}")
        client.createResult = PasskeyResult.Failed()

        vm.addPasskey(context, "a")
        vm.addPasskey(context, "b") // ignored: the first is still in flight
        advanceUntilIdle()

        coVerify(exactly = 1) { repo.beginRegistration(any()) }
    }
}
