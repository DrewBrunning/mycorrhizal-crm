package com.mycorrhizal.crm.feature.contacts

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.SessionState
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordInput
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.model.network.Organization
import com.mycorrhizal.crm.model.network.PersonalInfo
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import com.mycorrhizal.crm.ui.R
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import io.mockk.slot
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class ContactSectionsViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val contactRepository = mockk<ContactRepository>()
    private val authRepository = mockk<AuthRepository>()

    private val record = ContactRecordResponse(
        id = 5,
        uid = "uid-5",
        revision = 7,
        card = Card(
            name = Name(full = "Dana White"),
            organizations = listOf(Organization(id = "org-1", name = "Acme")),
            personalInfo = listOf(PersonalInfo(id = "pi-1", kind = "expertise", value = "Go"))
        )
    )

    private fun viewModel(
        getResult: Result<ContactRecordResponse> = Result.success(record),
        enabledContactFields: List<String>? = null
    ): ContactSectionsViewModel {
        coEvery { contactRepository.getContact(5) } returns getResult
        every { authRepository.observeSession() } returns
            flowOf(SessionState(dateFormat = "eu", enabledContactFields = enabledContactFields))
        return ContactSectionsViewModel(
            contactRepository,
            authRepository,
            SavedStateHandle(mapOf("contactId" to 5))
        )
    }

    @Test
    fun `load fetches the full record and the session date format and enabled fields`() = runTest {
        val vm = viewModel()
        advanceUntilIdle()

        assertEquals(record, vm.uiState.value.record)
        assertEquals("eu", vm.uiState.value.dateFormat)
        assertFalse(vm.uiState.value.isLoading)
        coVerify(exactly = 1) { contactRepository.getContact(5) }
    }

    @Test
    fun `load failure surfaces the message`() = runTest {
        val vm = viewModel(
            getResult = Result.failure(ApiError.Network(java.io.IOException("offline")))
        )
        advanceUntilIdle()

        assertNull(vm.uiState.value.record)
        assertTrue(vm.uiState.value.error?.isNotBlank() == true)
        assertFalse(vm.uiState.value.isLoading)
    }

    @Test
    fun `saving a skill PUTs the full record with the loaded revision and the new level and metadata`() =
        runTest {
            val vm = viewModel()
            advanceUntilIdle()
            val input = slot<ContactRecordInput>()
            coEvery { contactRepository.updateContact(5, capture(input), 7) } returns
                Result.success(record.copy(revision = 8))

            vm.savePersonalInfo(
                editingId = null,
                form = PersonalInfoFormData(
                    kind = "expertise",
                    value = "Rust",
                    level = "beginner",
                    sensitivity = "private",
                    notes = "learning"
                )
            )
            advanceUntilIdle()

            val entry = input.captured.card!!.personalInfo!!.last()
            assertEquals("Rust", entry.value)
            assertEquals("beginner", entry.level)
            assertEquals("private", input.captured.crm!!.personalInfoMeta!![entry.id]!!.sensitivity)
            assertEquals("learning", input.captured.crm!!.personalInfoMeta!![entry.id]!!.notes)
            // Untouched data rides along in the full-record PUT.
            assertEquals(listOf("org-1"), input.captured.card!!.organizations!!.map { it.id })
            assertEquals(8, vm.uiState.value.record?.revision)
            assertFalse(vm.uiState.value.isSaving)
        }

    @Test
    fun `a blank skill value is rejected locally without a PUT`() = runTest {
        val vm = viewModel()
        advanceUntilIdle()

        vm.savePersonalInfo(null, PersonalInfoFormData(kind = "hobby", value = "  "))
        advanceUntilIdle()

        assertEquals(R.string.contact_section_value_required, vm.uiState.value.errorRes)
        coVerify(exactly = 0) { contactRepository.updateContact(any(), any(), any()) }
    }

    @Test
    fun `a 412 shows the conflict dialog and loading latest refetches`() = runTest {
        val vm = viewModel()
        advanceUntilIdle()
        coEvery { contactRepository.updateContact(5, any(), 7) } returns
            Result.failure(ApiError.PreconditionFailed("stale"))

        vm.deleteOrganization("org-1")
        advanceUntilIdle()

        assertTrue(vm.uiState.value.conflict)
        assertFalse(vm.uiState.value.isSaving)

        vm.onConflictLoadLatest()
        advanceUntilIdle()

        assertFalse(vm.uiState.value.conflict)
        coVerify(exactly = 2) { contactRepository.getContact(5) }
    }

    @Test
    fun `a non-conflict failure leaves the record intact and surfaces the message`() = runTest {
        val vm = viewModel()
        advanceUntilIdle()
        coEvery { contactRepository.updateContact(5, any(), 7) } returns
            Result.failure(ApiError.Network(java.io.IOException("offline")))

        vm.deleteOrganization("org-1")
        advanceUntilIdle()

        assertEquals(record, vm.uiState.value.record)
        assertTrue(vm.uiState.value.error?.isNotBlank() == true)
        assertFalse(vm.uiState.value.isSaving)
    }

    @Test
    fun `work information saves through the same full-record path`() = runTest {
        val vm = viewModel()
        advanceUntilIdle()
        val input = slot<ContactRecordInput>()
        coEvery {
            contactRepository.updateContact(
                5,
                capture(input),
                7
            )
        } returns Result.success(record)

        vm.saveWorkInformation("  Analyst  ")
        advanceUntilIdle()

        assertEquals("Analyst", input.captured.crm!!.workInformation)
    }

    @Test
    fun `a blank anniversary date is rejected locally`() = runTest {
        val vm = viewModel()
        advanceUntilIdle()

        vm.saveAnniversary(null, AnniversaryFormData(kind = "wedding", date = ""))
        advanceUntilIdle()

        assertEquals(R.string.contact_section_date_required, vm.uiState.value.errorRes)
        coVerify(exactly = 0) { contactRepository.updateContact(any(), any(), any()) }
    }

    @Test
    fun `a missing contact id makes load a no-op`() = runTest {
        every { authRepository.observeSession() } returns flowOf(SessionState())
        val vm = ContactSectionsViewModel(
            contactRepository,
            authRepository,
            SavedStateHandle(emptyMap<String, Any>())
        )
        advanceUntilIdle()

        assertNull(vm.uiState.value.record)
        coVerify(exactly = 0) { contactRepository.getContact(any()) }
    }
}
