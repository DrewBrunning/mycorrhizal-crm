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
import com.mycorrhizal.crm.ui.R
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
        // Loaded as the real objects, not scalars — and critically, a loaded phone with no
        // label stays label=null, never forced to "cell" (T81).
        assertEquals(listOf(Email(address = "dana@example.com")), state.emails)
        assertEquals(listOf(Phone(number = "+1-555-0100")), state.phones)
        assertEquals("friends", state.circlesText)
        assertFalse(state.isLoading)
    }

    @Test
    fun `save in create mode calls createContact and emits Saved`() = runTest(mainDispatcherRule.testDispatcher) {
        val created = ContactRecordResponse(id = 9, card = Card(name = Name(full = "Carol King")))
        coEvery { contactRepository.createContact(any()) } returns Result.success(created)

        val vm = createViewModel()
        vm.onGivenNameChange("Carol")
        vm.onSurnameChange("King")
        vm.onEmailValueChange(0, "carol@example.com")
        vm.onPhoneValueChange(0, "+1-555-0100")
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
    fun `saved phones default to the cell label`() = runTest(mainDispatcherRule.testDispatcher) {
        // Write-parity with the web form's MultiValueField `defaultType="cell"`:
        // without a label the backend stores ContactPhone.Type="", buildPhones
        // emits a bare Phone{number}, and the detail screen can't detect it as
        // mobile (no SMS action).
        val created = ContactRecordResponse(id = 9, card = Card(name = Name(full = "Carol King")))
        coEvery { contactRepository.createContact(any()) } returns Result.success(created)

        val vm = createViewModel()
        vm.onGivenNameChange("Carol")
        vm.onSurnameChange("King")
        vm.onPhoneValueChange(0, "+1-555-0100")
        vm.save()
        advanceUntilIdle()

        coVerify {
            contactRepository.createContact(
                match<ContactRecordInput> { input ->
                    input.card?.phones?.firstOrNull()?.number == "+1-555-0100" &&
                        input.card?.phones?.firstOrNull()?.label == "cell"
                },
            )
        }
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
        vm.onEmailValueChange(0, "x@example.com")
        vm.save()
        advanceUntilIdle()

        assertEquals(R.string.contact_error_given_name, vm.uiState.value.errorRes)
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
    fun `phone is not blocked when it is short`() = runTest(mainDispatcherRule.testDispatcher) {
        // The nested ContactRecordInput validates only gender — a phone the old
        // flat model would reject (e.g. 4 digits) must pass here so the client
        // never rejects a value the server accepts.
        val created = ContactRecordResponse(id = 9, card = Card(name = Name(full = "Carol King")))
        coEvery { contactRepository.createContact(any()) } returns Result.success(created)

        val vm = createViewModel()
        vm.onGivenNameChange("Carol")
        vm.onPhoneValueChange(0, "1234")
        vm.save()
        advanceUntilIdle()

        assertEquals(ContactFormEvent.Saved, vm.events.value)
        coVerify { contactRepository.createContact(any()) }
    }

    @Test
    fun `circles text is parsed only on save`() = runTest(mainDispatcherRule.testDispatcher) {
        val created = ContactRecordResponse(id = 9, card = Card(name = Name(full = "Carol King")))
        coEvery { contactRepository.createContact(any()) } returns Result.success(created)

        val vm = createViewModel()
        vm.onGivenNameChange("Carol")
        vm.onCirclesTextChange("friends, family")
        vm.save()
        advanceUntilIdle()

        coVerify {
            contactRepository.createContact(
                match<ContactRecordInput> { input ->
                    input.crm?.circles == listOf("friends", "family")
                },
            )
        }
    }

    @Test
    fun `edit save preserves fields the form does not model`() = runTest(mainDispatcherRule.testDispatcher) {
        // Base record carries addresses + personalInfo the form never renders.
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
                addresses = listOf(
                    com.mycorrhizal.crm.model.network.Address(full = "123 Main St"),
                ),
                personalInfo = listOf(
                    com.mycorrhizal.crm.model.network.PersonalInfo(kind = "hobby", value = "climbing"),
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

        coVerify {
            contactRepository.updateContact(
                5,
                match<ContactRecordInput> { input ->
                    input.card?.addresses?.firstOrNull()?.full == "123 Main St" &&
                        input.card?.personalInfo?.firstOrNull()?.value == "climbing" &&
                        input.card?.name?.components?.firstOrNull { it.kind == "surname" }?.value == "Whitehall"
                },
            )
        }
    }

    @Test
    fun `editing an unrelated field preserves phone and email metadata`() = runTest(mainDispatcherRule.testDispatcher) {
        // Regression for T81: the old toInput reconstructed every phone/email from
        // scratch on every save, so editing the *name* silently relabeled every
        // phone "cell" and dropped id/pref/contexts/features.
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
                phones = listOf(
                    Phone(
                        id = "phone-1",
                        number = "+1-555-0100",
                        label = "work",
                        contexts = listOf("work"),
                        pref = 1,
                        features = listOf("voice"),
                    ),
                ),
                emails = listOf(
                    Email(id = "email-1", address = "dana@example.com", contexts = listOf("work"), pref = 1),
                ),
            ),
        )
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { contactRepository.updateContact(5, any()) } returns Result.success(record)

        val vm = createViewModel(5)
        advanceUntilIdle()
        vm.onGivenNameChange("Danielle") // unrelated to phones/emails
        vm.save()
        advanceUntilIdle()

        coVerify {
            contactRepository.updateContact(
                5,
                match<ContactRecordInput> { input ->
                    val phone = input.card?.phones?.firstOrNull()
                    val email = input.card?.emails?.firstOrNull()
                    phone?.id == "phone-1" && phone.label == "work" && phone.contexts == listOf("work") &&
                        phone.pref == 1 && phone.features == listOf("voice") &&
                        email?.id == "email-1" && email.contexts == listOf("work") && email.pref == 1
                },
            )
        }
    }

    @Test
    fun `a newly added phone still defaults to cell, an existing one is untouched`() = runTest(mainDispatcherRule.testDispatcher) {
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
                phones = listOf(Phone(id = "phone-1", number = "+1-555-0100", label = "work")),
            ),
        )
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { contactRepository.updateContact(5, any()) } returns Result.success(record)

        val vm = createViewModel(5)
        advanceUntilIdle()
        vm.onPhoneAdd()
        vm.onPhoneValueChange(1, "+1-555-0199")
        vm.save()
        advanceUntilIdle()

        coVerify {
            contactRepository.updateContact(
                5,
                match<ContactRecordInput> { input ->
                    val phones = input.card?.phones.orEmpty()
                    phones.any { it.id == "phone-1" && it.number == "+1-555-0100" && it.label == "work" } &&
                        phones.any { it.id == null && it.number == "+1-555-0199" && it.label == "cell" }
                },
            )
        }
    }

    @Test
    fun `yearless birthday round-trips through the form`() = runTest(mainDispatcherRule.testDispatcher) {
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
                anniversaries = listOf(
                    com.mycorrhizal.crm.model.network.Anniversary(
                        kind = "birth",
                        date = com.mycorrhizal.crm.model.network.AnniversaryDate(
                            partial = com.mycorrhizal.crm.model.network.PartialDate(year = null, month = 12, day = 25),
                        ),
                    ),
                ),
            ),
        )
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { contactRepository.updateContact(5, any()) } returns Result.success(record)

        val vm = createViewModel(5)
        advanceUntilIdle()

        // Yearless birthday renders as "--MM-DD" in the field.
        assertEquals("--12-25", vm.uiState.value.birthday)

        // Saving with the field untouched must preserve the yearless partial.
        vm.save()
        advanceUntilIdle()

        coVerify {
            contactRepository.updateContact(
                5,
                match<ContactRecordInput> { input ->
                    val partial = input.card?.anniversaries?.firstOrNull()?.date?.partial
                    partial?.year == null && partial?.month == 12 && partial?.day == 25
                },
            )
        }
    }

    @Test
    fun `blank birthday field preserves a year-only anniversary on save`() = runTest(mainDispatcherRule.testDispatcher) {
        // A CardDAV-synced year-only partial (no month/day) cannot be rendered
        // in the form field; saving must NOT delete it.
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
                anniversaries = listOf(
                    com.mycorrhizal.crm.model.network.Anniversary(
                        kind = "birth",
                        date = com.mycorrhizal.crm.model.network.AnniversaryDate(
                            partial = com.mycorrhizal.crm.model.network.PartialDate(year = 2020, month = null, day = null),
                        ),
                    ),
                ),
            ),
        )
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { contactRepository.updateContact(5, any()) } returns Result.success(record)

        val vm = createViewModel(5)
        advanceUntilIdle()
        assertEquals("", vm.uiState.value.birthday)

        vm.onSurnameChange("Whitehall") // some unrelated edit
        vm.save()
        advanceUntilIdle()

        coVerify {
            contactRepository.updateContact(
                5,
                match<ContactRecordInput> { input ->
                    val partial = input.card?.anniversaries?.firstOrNull()?.date?.partial
                    partial?.year == 2020 && partial?.month == null
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
