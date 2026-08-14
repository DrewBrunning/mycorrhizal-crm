package com.mycorrhizal.crm.feature.cadence

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.CadencePolicyRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.SessionState
import com.mycorrhizal.crm.model.network.CadenceHealth
import com.mycorrhizal.crm.model.network.CadencePolicy
import com.mycorrhizal.crm.model.network.CadencePolicyInput
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.model.registry.CadenceQualifyingType
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import com.mycorrhizal.crm.ui.R
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class CadenceViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val policyRepository = mockk<CadencePolicyRepository>()
    private val contactRepository = mockk<ContactRepository>()
    private val authRepository = mockk<AuthRepository>()

    private fun viewModel(id: Int = 5): CadenceViewModel {
        stubSession()
        return CadenceViewModel(policyRepository, contactRepository, authRepository, SavedStateHandle(mapOf("contactId" to id)))
    }

    private val uid = "11111111-1111-1111-1111-111111111111"

    private fun stubContact() {
        coEvery { contactRepository.getContact(5) } returns Result.success(
            ContactRecordResponse(id = 5, card = Card(uid = uid, name = Name(full = "Dana White"))),
        )
    }

    private fun stubSession(dateFormat: String? = null) {
        coEvery { authRepository.observeSession() } returns flowOf(SessionState(dateFormat = dateFormat))
    }

    private fun policy(
        id: String = "p1",
        interval: Int = 30,
        qualifyingTypes: List<String> = emptyList(),
        health: CadenceHealth? = CadenceHealth(hasQualifyingInteraction = true, overdueBy = 0),
    ) = CadencePolicy(
        id = id,
        entityId = uid,
        targetIntervalDays = interval,
        qualifyingTypes = qualifyingTypes,
        health = health,
    )

    @Test
    fun `load resolves the vcard uid and takes the first policy`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact()
        coEvery { policyRepository.listForContact(uid) } returns Result.success(
            listOf(policy(interval = 45), policy(interval = 60)),
        )

        val vm = viewModel()
        advanceUntilIdle()

        assertFalse(vm.uiState.value.isLoading)
        assertEquals(uid, vm.uiState.value.contactVCardUid)
        assertEquals(45, vm.uiState.value.policy?.targetIntervalDays)
        assertNull(vm.uiState.value.error)
    }

    @Test
    fun `load with no policies leaves the empty state`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact()
        coEvery { policyRepository.listForContact(uid) } returns Result.success(emptyList())

        val vm = viewModel()
        advanceUntilIdle()

        assertNull(vm.uiState.value.policy)
        assertNull(vm.uiState.value.error)
    }

    @Test
    fun `missing contact id sets an error`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = viewModel(id = 0)
        advanceUntilIdle()

        assertFalse(vm.uiState.value.isLoading)
        assertEquals(R.string.cadence_error_missing_contact_id, vm.uiState.value.errorRes)
    }

    @Test
    fun `contact with no vcard uid sets an error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { contactRepository.getContact(5) } returns Result.success(
            ContactRecordResponse(id = 5, card = Card(name = Name(full = "No Uid"))),
        )

        val vm = viewModel()
        advanceUntilIdle()

        assertEquals(R.string.cadence_error_no_vcard_uid, vm.uiState.value.errorRes)
    }

    @Test
    fun `load failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact()
        coEvery { policyRepository.listForContact(uid) } returns Result.failure(
            ApiError.Client(500, "boom"),
        )

        val vm = viewModel()
        advanceUntilIdle()

        assertEquals("boom", vm.uiState.value.error)
    }

    @Test
    fun `create sends the interval and qualifying types, preserving an empty selection`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact()
        coEvery { policyRepository.listForContact(uid) } returns Result.success(emptyList())
        coEvery {
            policyRepository.create(
                match { input ->
                    input.entityId == uid &&
                        input.targetIntervalDays == 14 &&
                        input.qualifyingTypes == emptyList<String>()
                },
            )
        } returns Result.success(policy(interval = 14, qualifyingTypes = emptyList()))

        val vm = viewModel()
        advanceUntilIdle()

        vm.create(14, emptyList())
        advanceUntilIdle()

        assertEquals(14, vm.uiState.value.policy?.targetIntervalDays)
        coVerify {
            policyRepository.create(
                CadencePolicyInput(uid, 14, emptyList()),
            )
        }
    }

    @Test
    fun `create sends the selected qualifying types`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact()
        coEvery { policyRepository.listForContact(uid) } returns Result.success(emptyList())
        val selected = listOf(CadenceQualifyingType.CALL, CadenceQualifyingType.VISIT)
        coEvery {
            policyRepository.create(
                match { it.qualifyingTypes == selected },
            )
        } returns Result.success(policy(qualifyingTypes = selected))

        val vm = viewModel()
        advanceUntilIdle()

        vm.create(30, selected)
        advanceUntilIdle()

        assertEquals(selected, vm.uiState.value.policy?.qualifyingTypes)
    }

    @Test
    fun `update targets the policy id and resends entity_id`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact()
        coEvery { policyRepository.listForContact(uid) } returns Result.success(listOf(policy(id = "p1")))
        coEvery {
            policyRepository.update(
                "p1",
                match { input -> input.entityId == uid && input.targetIntervalDays == 90 },
            )
        } returns Result.success(policy(id = "p1", interval = 90))

        val vm = viewModel()
        advanceUntilIdle()

        vm.update("p1", 90, listOf(CadenceQualifyingType.MESSAGE))
        advanceUntilIdle()

        assertEquals(90, vm.uiState.value.policy?.targetIntervalDays)
        coVerify {
            policyRepository.update("p1", CadencePolicyInput(uid, 90, listOf(CadenceQualifyingType.MESSAGE)))
        }
    }

    @Test
    fun `delete returns to the empty state`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact()
        coEvery { policyRepository.listForContact(uid) } returns Result.success(listOf(policy(id = "p1")))
        coEvery { policyRepository.delete("p1") } returns Result.success(Unit)

        val vm = viewModel()
        advanceUntilIdle()

        vm.delete("p1")
        advanceUntilIdle()

        assertNull(vm.uiState.value.policy)
        coVerify { policyRepository.delete("p1") }
    }

    @Test
    fun `the server's health readout passes through untouched`() = runTest(mainDispatcherRule.testDispatcher) {
        // The ticket pins "health is read, never recomputed": a server verdict
        // that contradicts what the local dates would imply must surface as-is.
        stubContact()
        val verdict = CadenceHealth(
            hasQualifyingInteraction = true,
            lastInteraction = "2026-07-01T10:00:00Z",
            nextDue = "2026-07-31T00:00:00Z",
            overdueBy = 3,
        )
        coEvery { policyRepository.listForContact(uid) } returns Result.success(
            listOf(policy(interval = 30, health = verdict)),
        )

        val vm = viewModel()
        advanceUntilIdle()

        val health = vm.uiState.value.policy?.health
        assertEquals(3, health?.overdueBy)
        assertTrue(health?.isOverdue == true)
        assertEquals("2026-07-01T10:00:00Z", health?.lastInteraction)
        assertEquals("2026-07-31T00:00:00Z", health?.nextDue)
    }

    @Test
    fun `write failure surfaces the error and keeps the current policy`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact()
        coEvery { policyRepository.listForContact(uid) } returns Result.success(listOf(policy(id = "p1")))
        coEvery { policyRepository.update("p1", any()) } returns Result.failure(
            ApiError.Client(409, "conflict"),
        )

        val vm = viewModel()
        advanceUntilIdle()

        vm.update("p1", 15, emptyList())
        advanceUntilIdle()

        assertEquals("conflict", vm.uiState.value.error)
        assertFalse(vm.uiState.value.isMutating)
        assertEquals("p1", vm.uiState.value.policy?.id)
    }

    @Test
    fun `the session date format flows into the state`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact()
        coEvery { policyRepository.listForContact(uid) } returns Result.success(emptyList())

        val vm = viewModel()
        // Override after construction (the helper stubs a default); the collect
        // coroutine only runs once the dispatcher advances, so it sees "iso".
        stubSession(dateFormat = "iso")
        advanceUntilIdle()

        assertEquals("iso", vm.uiState.value.dateFormat)
    }

    @Test
    fun `an absent session date format leaves the state default`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact()
        coEvery { policyRepository.listForContact(uid) } returns Result.success(emptyList())

        val vm = viewModel()
        stubSession(dateFormat = null)
        advanceUntilIdle()

        assertNull(vm.uiState.value.dateFormat)
    }
}
