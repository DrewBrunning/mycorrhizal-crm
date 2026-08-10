package com.mycorrhizal.crm.feature.contacts

import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import androidx.lifecycle.SavedStateHandle
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class ContactDetailViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val contactRepository = mockk<ContactRepository>()

    private fun viewModel(id: Int): ContactDetailViewModel =
        ContactDetailViewModel(contactRepository, SavedStateHandle(mapOf("contactId" to id)))

    @Test
    fun `loads the contact on init`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        val vm = viewModel(5)
        advanceUntilIdle()

        val state = vm.uiState.value
        assertFalse(state.isLoading)
        assertEquals("Dana White", state.contact?.card?.name?.full)
        assertNull(state.error)
    }

    @Test
    fun `missing contact id sets an error`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = viewModel(0)
        advanceUntilIdle()

        assertEquals("Missing contact id", vm.uiState.value.error)
    }

    @Test
    fun `server failure surfaces the error message`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { contactRepository.getContact(5) } returns Result.failure(
            ApiError.Client(404, "Contact not found"),
        )

        val vm = viewModel(5)
        advanceUntilIdle()

        val state = vm.uiState.value
        assertFalse(state.isLoading)
        assertNull(state.contact)
        // ApiError maps 404 to a generic "Not found" display string (ticket §2.4).
        assertEquals("Not found", state.error)
    }

    @Test
    fun `onErrorShown clears the error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { contactRepository.getContact(5) } returns Result.failure(
            ApiError.Client(404, "Contact not found"),
        )

        val vm = viewModel(5)
        advanceUntilIdle()
        assertTrue(vm.uiState.value.error != null)

        vm.onErrorShown()
        assertNull(vm.uiState.value.error)
    }
}
