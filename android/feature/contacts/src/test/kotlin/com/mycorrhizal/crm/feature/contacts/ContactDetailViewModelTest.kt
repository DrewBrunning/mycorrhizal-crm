package com.mycorrhizal.crm.feature.contacts

import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.ReminderRepository
import com.mycorrhizal.crm.domain.repository.SessionState
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactFieldValuesResponse
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.FieldDefinition
import com.mycorrhizal.crm.model.network.FieldDefinitionsResponse
import com.mycorrhizal.crm.model.network.FieldValue
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import com.mycorrhizal.crm.ui.R
import androidx.lifecycle.SavedStateHandle
import io.mockk.coEvery
import io.mockk.every
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

class ContactDetailViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val contactRepository = mockk<ContactRepository>()
    private val reminderRepository = mockk<ReminderRepository>()
    private val authRepository = mockk<AuthRepository>()
    private val apiClient = mockk<ApiClient>()

    private fun viewModel(id: Int, dateFormat: String? = null): ContactDetailViewModel {
        coEvery { contactRepository.getDeviceLookupKey(any()) } returns null
        every { authRepository.observeSession() } returns flowOf(SessionState(dateFormat = dateFormat))
        coEvery { apiClient.listFieldDefinitions(any()) } returns Result.success(FieldDefinitionsResponse())
        coEvery { apiClient.listContactFieldValues(any()) } returns Result.success(ContactFieldValuesResponse())
        return ContactDetailViewModel(
            contactRepository,
            reminderRepository,
            authRepository,
            apiClient,
            SavedStateHandle(mapOf("contactId" to id)),
        )
    }

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

        assertEquals(R.string.contact_error_missing_id, vm.uiState.value.errorRes)
    }

    @Test
    fun `string contact id from navigation is parsed as an int`() = runTest(mainDispatcherRule.testDispatcher) {
        // Navigation Compose stores route args as Strings unless NavType.IntType
        // is declared; the ViewModel must tolerate both shapes.
        val record = ContactRecordResponse(id = 9, card = Card(name = Name(full = "Erin")))
        coEvery { contactRepository.getContact(9) } returns Result.success(record)
        coEvery { contactRepository.getDeviceLookupKey(9) } returns null
        every { authRepository.observeSession() } returns flowOf(SessionState())
        coEvery { apiClient.listFieldDefinitions(any()) } returns Result.success(FieldDefinitionsResponse())
        coEvery { apiClient.listContactFieldValues(any()) } returns Result.success(ContactFieldValuesResponse())

        val vm = ContactDetailViewModel(
            contactRepository,
            reminderRepository,
            authRepository,
            apiClient,
            SavedStateHandle(mapOf("contactId" to "9")),
        )
        advanceUntilIdle()

        assertEquals("Erin", vm.uiState.value.contact?.card?.name?.full)
    }

    @Test
    fun `dateFormat reflects the signed-in user's date_format preference`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        val vm = viewModel(5, dateFormat = "us")
        advanceUntilIdle()

        assertEquals("us", vm.uiState.value.dateFormat)
    }

    @Test
    fun `dateFormat is null when the session carries no preference`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        val vm = viewModel(5, dateFormat = null)
        advanceUntilIdle()

        assertNull(vm.uiState.value.dateFormat)
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

    @Test
    fun `completeReminder reloads the contact`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { reminderRepository.complete(7) } returns Result.success(null)

        val vm = viewModel(5)
        advanceUntilIdle()

        vm.completeReminder(7)
        advanceUntilIdle()

        // complete() succeeded -> load() was re-run; the contact is still present.
        assertEquals("Dana White", vm.uiState.value.contact?.card?.name?.full)
    }

    @Test
    fun `completeReminder failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { reminderRepository.complete(7) } returns Result.failure(
            ApiError.Client(404, "Reminder not found"),
        )

        val vm = viewModel(5)
        advanceUntilIdle()

        vm.completeReminder(7)
        advanceUntilIdle()

        assertEquals("Not found", vm.uiState.value.error)
    }

    @Test
    fun `custom field definitions and values load into state keyed by definition id`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { contactRepository.getDeviceLookupKey(5) } returns null
        every { authRepository.observeSession() } returns flowOf(SessionState())
        coEvery { apiClient.listFieldDefinitions(any()) } returns Result.success(
            FieldDefinitionsResponse(fieldDefinitions = listOf(FieldDefinition(id = "d1", label = "Coffee order", type = "string"))),
        )
        coEvery { apiClient.listContactFieldValues(5) } returns Result.success(
            ContactFieldValuesResponse(fieldValues = listOf(FieldValue(id = 1, fieldDefinitionId = "d1", value = "Latte"))),
        )

        val vm = ContactDetailViewModel(contactRepository, reminderRepository, authRepository, apiClient, SavedStateHandle(mapOf("contactId" to 5)))
        advanceUntilIdle()

        val state = vm.uiState.value
        assertEquals(1, state.fieldDefinitions.size)
        assertEquals("Coffee order", state.fieldDefinitions.first().label)
        assertEquals("Latte", state.fieldValuesByDefinitionId["d1"])
    }

    @Test
    fun `a field-definitions fetch failure does not fail the contact load`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { contactRepository.getDeviceLookupKey(5) } returns null
        every { authRepository.observeSession() } returns flowOf(SessionState())
        coEvery { apiClient.listFieldDefinitions(any()) } returns Result.failure(ApiError.Server(500, "boom"))
        coEvery { apiClient.listContactFieldValues(5) } returns Result.success(ContactFieldValuesResponse())

        val vm = ContactDetailViewModel(contactRepository, reminderRepository, authRepository, apiClient, SavedStateHandle(mapOf("contactId" to 5)))
        advanceUntilIdle()

        val state = vm.uiState.value
        assertEquals("Dana White", state.contact?.card?.name?.full)
        assertNull(state.error) // the custom-fields failure must not surface as the screen's error
        assertTrue(state.fieldDefinitions.isEmpty())
    }

    @Test
    fun `a field-values fetch failure does not fail the contact load`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { contactRepository.getDeviceLookupKey(5) } returns null
        every { authRepository.observeSession() } returns flowOf(SessionState())
        coEvery { apiClient.listFieldDefinitions(any()) } returns Result.success(
            FieldDefinitionsResponse(fieldDefinitions = listOf(FieldDefinition(id = "d1", label = "Coffee order", type = "string"))),
        )
        coEvery { apiClient.listContactFieldValues(5) } returns Result.failure(ApiError.Network(java.io.IOException("offline")))

        val vm = ContactDetailViewModel(contactRepository, reminderRepository, authRepository, apiClient, SavedStateHandle(mapOf("contactId" to 5)))
        advanceUntilIdle()

        val state = vm.uiState.value
        assertEquals("Dana White", state.contact?.card?.name?.full)
        assertNull(state.error)
        assertEquals(1, state.fieldDefinitions.size) // definitions still loaded independently
        assertTrue(state.fieldValuesByDefinitionId.isEmpty())
    }

    @Test
    fun `a value whose definition no longer exists is stored but never resolves without a matching definition`() = runTest(mainDispatcherRule.testDispatcher) {
        // T84: definitions and values are fetched separately and can disagree (a definition
        // deleted after the value was set). The ViewModel does no filtering — it stores exactly
        // what the server returned; the render loop (which iterates definitions, not values,
        // see ContactDetailScreen's CustomFieldsSection) is what makes an orphaned value
        // unreachable, without needing special-case code or risking a crash.
        val record = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { contactRepository.getDeviceLookupKey(5) } returns null
        every { authRepository.observeSession() } returns flowOf(SessionState())
        coEvery { apiClient.listFieldDefinitions(any()) } returns Result.success(FieldDefinitionsResponse()) // zero definitions
        coEvery { apiClient.listContactFieldValues(5) } returns Result.success(
            ContactFieldValuesResponse(fieldValues = listOf(FieldValue(id = 1, fieldDefinitionId = "deleted-def", value = "orphaned"))),
        )

        val vm = ContactDetailViewModel(contactRepository, reminderRepository, authRepository, apiClient, SavedStateHandle(mapOf("contactId" to 5)))
        advanceUntilIdle()

        val state = vm.uiState.value
        assertNull(state.error)
        assertTrue(state.fieldDefinitions.isEmpty())
        assertEquals("orphaned", state.fieldValuesByDefinitionId["deleted-def"]) // present but unreachable — no def to render it under
    }
}
