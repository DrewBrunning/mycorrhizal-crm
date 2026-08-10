package com.mycorrhizal.crm.feature.contacts

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.model.network.CRMEnvelope
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordInput
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.model.network.Nickname
import com.mycorrhizal.crm.model.network.Email
import com.mycorrhizal.crm.model.network.Phone
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

class ContactFormViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val contactRepository = mockk<ContactRepository>()

    private fun createViewModel(id: Int? = null): ContactFormViewModel =
        ContactFormViewModel(
            contactRepository,
            if (id == null) SavedStateHandle() else SavedStateHandle(mapOf("contactId" to id)),
        )

    @Test
    fun `create mode starts with empty form and no id`() {
        val vm = createViewModel()
        val state = vm.uiState.value
        assertFalse(state.isEdit)
        assertNull(state.contactId)
        assertTrue(state.hasName.not())
    }

    @Test
    fun `edit mode loads the existing contact into the form`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(
                    full = "Dana White",
                    components = listOf(
                        com.mycorrhizal.crm.model.network.NameComponent(kind = "given", value = "Dana"),
                        com.mycorrhizal.crm.model.network.NameComponent(kind = "surname", value = "White"),
                    ),
                ),
                nicknames = listOf(Nickname(name = "D")),
                emails = listOf(Email(address = "dana@example.com")),
                phones = listOf(Phone(number = "+1-555-0100")),
            ),
            crm = CRMEnvelope(circles = listOf("friends")),
        )
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        val vm = createViewModel(5)
        advanceUntilIdle()

        val state = vm.uiState.value
        assertTrue(state.isEdit)
        assertEquals(5, state.contactId)
        assertEquals("Dana", state.givenName)
        assertEquals("White", state.surname)
        assertEquals("D", state.nickname)
        assertEquals(listOf("dana@example.com"), state.emails)
        assertEquals(listOf("+1-555-0100"), state.phones)
        assertEquals(listOf("friends"), state.circles)
        assertFalse(state.isLoading)
    }

    @Test
    fun `save in create mode calls createContact and emits Saved`() = runTest(mainDispatcherRule.testDispatcher) {
        val created = ContactRecordResponse(id = 9, card = Card(name = Name(full = "Carol King")))
        coEvery { contactRepository.createContact(any()) } returns Result.success(created)

        val vm = createViewModel()
        vm.onGivenNameChange("Carol")
        vm.onSurnameChange("King")
        vm.onEmailsChange(listOf("carol@example.com"))
        vm.save()
        advanceUntilIdle()

        coVerify {
            contactRepository.createContact(
                match<ContactRecordInput> { input ->
                    input.card?.name?.components?.firstOrNull { it.kind == "given" }?.value == "Carol" &&
                        input.card?.name?.components?.firstOrNull { it.kind == "surname" }?.value == "King" &&
                        input.card?.emails?.firstOrNull()?.address == "carol@example.com"
                },
            )
        }
        assertEquals(ContactFormEvent.Saved, vm.events.value)
        assertFalse(vm.uiState.value.isSaving)
    }

    @Test
    fun `save in edit mode calls updateContact with the id`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(
                    full = "Dana White",
                    components = listOf(
                        com.mycorrhizal.crm.model.network.NameComponent(kind = "given", value = "Dana"),
                        com.mycorrhizal.crm.model.network.NameComponent(kind = "surname", value = "White"),
                    ),
                ),
            ),
        )
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { contactRepository.updateContact(5, any()) } returns Result.success(record)

        val vm = createViewModel(5)
        advanceUntilIdle()
        vm.onSurnameChange("Whitehall")
        vm.save()
        advanceUntilIdle()

        coVerify { contactRepository.updateContact(5, any()) }
        assertEquals(ContactFormEvent.Saved, vm.events.value)
    }

    @Test
    fun `save without a given name is blocked by validation`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = createViewModel()
        vm.onEmailsChange(listOf("x@example.com"))
        vm.save()
        advanceUntilIdle()

        assertEquals("At least one given name is required", vm.uiState.value.error)
        coVerify(exactly = 0) { contactRepository.createContact(any()) }
    }

    @Test
    fun `invalid phone blocks save`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = createViewModel()
        vm.onGivenNameChange("Carol")
        vm.onPhonesChange(listOf("abc"))
        vm.save()
        advanceUntilIdle()

        assertEquals("Enter a valid phone number", vm.uiState.value.error)
        coVerify(exactly = 0) { contactRepository.createContact(any()) }
    }

    @Test
    fun `invalid birthday blocks save`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = createViewModel()
        vm.onGivenNameChange("Carol")
        vm.onBirthdayChange("15.06.1990")
        vm.save()
        advanceUntilIdle()

        assertEquals("Birthday must be YYYY-MM-DD or --MM-DD", vm.uiState.value.error)
        coVerify(exactly = 0) { contactRepository.createContact(any()) }
    }

    @Test
    fun `birthday is converted to a partial date in the input`() = runTest(mainDispatcherRule.testDispatcher) {
        val created = ContactRecordResponse(id = 9, card = Card(name = Name(full = "Carol King")))
        coEvery { contactRepository.createContact(any()) } returns Result.success(created)

        val vm = createViewModel()
        vm.onGivenNameChange("Carol")
        vm.onSurnameChange("King")
        vm.onBirthdayChange("1990-06-15")
        vm.save()
        advanceUntilIdle()

        coVerify {
            contactRepository.createContact(
                match<ContactRecordInput> { input ->
                    val partial = input.card?.anniversaries?.firstOrNull()?.date?.partial
                    partial?.year == 1990 && partial?.month == 6 && partial?.day == 15
                },
            )
        }
    }

    @Test
    fun `failed save surfaces the error and stays on the form`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { contactRepository.createContact(any()) } returns Result.failure(
            ApiError.Client(409, "A contact with this email already exists"),
        )

        val vm = createViewModel()
        vm.onGivenNameChange("Carol")
        vm.save()
        advanceUntilIdle()

        assertEquals("A contact with this email already exists", vm.uiState.value.error)
        assertFalse(vm.uiState.value.isSaving)
        assertNull(vm.events.value)
    }
}
