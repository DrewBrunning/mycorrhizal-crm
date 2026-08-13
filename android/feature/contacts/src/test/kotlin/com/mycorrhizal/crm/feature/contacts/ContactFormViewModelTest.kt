package com.mycorrhizal.crm.feature.contacts

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.CircleRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.TagRepository
import com.mycorrhizal.crm.model.network.CRMEnvelope
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.ContactRecordInput
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.model.network.Nickname
import com.mycorrhizal.crm.model.network.Email
import com.mycorrhizal.crm.model.network.Phone
import com.mycorrhizal.crm.model.network.Address
import com.mycorrhizal.crm.model.network.AddressComponent
import com.mycorrhizal.crm.model.network.OnlineService
import com.mycorrhizal.crm.model.network.Organization
import com.mycorrhizal.crm.model.network.PersonalInfo
import com.mycorrhizal.crm.model.network.Resource
import com.mycorrhizal.crm.model.network.Title
import com.mycorrhizal.crm.model.network.Tag
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.CONTACT_TYPE_OPTIONS
import com.mycorrhizal.crm.ui.components.CONTEXT_OPTIONS
import com.mycorrhizal.crm.ui.components.EmailSpec
import com.mycorrhizal.crm.ui.components.LinkSpec
import com.mycorrhizal.crm.ui.components.OnlineServiceSpec
import com.mycorrhizal.crm.ui.components.PERSONAL_INFO_KIND_OPTIONS
import com.mycorrhizal.crm.ui.components.PhoneSpec
import com.mycorrhizal.crm.ui.components.TitleSpec
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class ContactFormViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val contactRepository = mockk<ContactRepository>()
    private val circleRepository = mockk<CircleRepository>()
    private val tagRepository = mockk<TagRepository>()

    private fun createViewModel(id: Int? = null): ContactFormViewModel {
        // Default stubs so the init option-loader / membership-derivation coroutines never
        // throw. NOTE: mockk's last-registered-wins means a test wanting a *specific* answer
        // must re-stub AFTER createViewModel() and BEFORE advanceUntilIdle() (the coroutines
        // are only scheduled until then).
        coEvery { circleRepository.list(any(), any()) } returns Result.success(emptyList())
        coEvery { tagRepository.list(any(), any()) } returns Result.success(emptyList())
        coEvery { circleRepository.circlesForContact(any()) } returns Result.success(emptyList())
        coEvery { tagRepository.tagsForContact(any()) } returns Result.success(emptyList())
        return ContactFormViewModel(
            contactRepository,
            circleRepository,
            tagRepository,
            if (id == null) SavedStateHandle() else SavedStateHandle(mapOf("contactId" to id)),
        )
    }

    @Test
    fun `create mode starts with empty form and no id`() {
        val vm = createViewModel()
        val state = vm.uiState.value
        assertFalse(state.isEdit)
        assertNull(state.contactId)
        assertTrue(state.hasName.not())
        // M24: the kind defaults to human and the language to the device locale (web parity).
        assertEquals(ContactFormState.KIND_HUMAN, state.kind)
        assertTrue(state.language.isNotBlank())
    }

    @Test
    fun `edit mode loads the existing contact into the form`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(
            id = 5,
            uid = "u5",
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
        // M24: the circles chips seed from the join-row derivation, not the stale flat
        // `crm.circles` column (which is a legacy mirror, not the authoritative membership).
        // Re-stub after creation (last-registered-wins) and before advanceUntilIdle.
        coEvery { circleRepository.circlesForContact("u5") } returns Result.success(listOf(Circle(id = "c1", name = "friends")))
        coEvery { tagRepository.tagsForContact("u5") } returns Result.success(listOf(Tag(id = "t1", name = "close")))
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
        // Seeded from the join-row derivations.
        assertEquals(listOf("friends"), state.circles)
        assertEquals(listOf("close"), state.tags)
        assertFalse(state.isLoading)
    }

    @Test
    fun `edit mode maps prefix middle suffix kind and language from the record`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(
            id = 5,
            uid = "u5",
            card = Card(
                language = "fr",
                name = Name(
                    full = "Dr. Dana White Jr.",
                    components = listOf(
                        com.mycorrhizal.crm.model.network.NameComponent(kind = "title", value = "Dr."),
                        com.mycorrhizal.crm.model.network.NameComponent(kind = "given", value = "Dana"),
                        com.mycorrhizal.crm.model.network.NameComponent(kind = "given2", value = "Ann"),
                        com.mycorrhizal.crm.model.network.NameComponent(kind = "surname", value = "White"),
                        com.mycorrhizal.crm.model.network.NameComponent(kind = "generation", value = "Jr."),
                    ),
                ),
            ),
            crm = CRMEnvelope(kind = "animal"),
        )
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        val vm = createViewModel(5)
        advanceUntilIdle()

        val state = vm.uiState.value
        assertEquals("Dr.", state.prefix)
        assertEquals("Ann", state.middleName)
        assertEquals("Jr.", state.suffix)
        assertEquals(ContactFormState.KIND_ANIMAL, state.kind)
        assertEquals("fr", state.language)
    }

    @Test
    fun `save in create mode calls createContact and emits Saved`() = runTest(mainDispatcherRule.testDispatcher) {
        val created = ContactRecordResponse(id = 9, card = Card(name = Name(full = "Carol King")))
        coEvery { contactRepository.createContact(any()) } returns Result.success(created)

        val vm = createViewModel()
        vm.onGivenNameChange("Carol")
        vm.onSurnameChange("King")
        vm.onEmailsChange(listOf(Email(address = "carol@example.com")))
        vm.onPhonesChange(listOf(Phone(number = "+1-555-0100", label = "cell")))
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
    fun `prefix middle and suffix map to title given2 and generation components`() = runTest(mainDispatcherRule.testDispatcher) {
        val created = ContactRecordResponse(id = 9, card = Card(name = Name(full = "Carol King")))
        coEvery { contactRepository.createContact(any()) } returns Result.success(created)

        val vm = createViewModel()
        vm.onGivenNameChange("Carol")
        vm.onPrefixChange("Dr.")
        vm.onMiddleNameChange("Ann")
        vm.onSurnameChange("King")
        vm.onSuffixChange("Jr.")
        vm.save()
        advanceUntilIdle()

        coVerify {
            contactRepository.createContact(
                match<ContactRecordInput> { input ->
                    val comps = input.card?.name?.components.orEmpty()
                    comps.firstOrNull { it.kind == "title" }?.value == "Dr." &&
                        comps.firstOrNull { it.kind == "given2" }?.value == "Ann" &&
                        comps.firstOrNull { it.kind == "generation" }?.value == "Jr."
                },
            )
        }
    }

    @Test
    fun `kind and language are sent in create mode`() = runTest(mainDispatcherRule.testDispatcher) {
        val created = ContactRecordResponse(id = 9, card = Card(name = Name(full = "Rex")))
        coEvery { contactRepository.createContact(any()) } returns Result.success(created)

        val vm = createViewModel()
        vm.onGivenNameChange("Rex")
        vm.onKindChange(ContactFormState.KIND_ANIMAL)
        vm.onLanguageChange("de")
        vm.save()
        advanceUntilIdle()

        coVerify {
            contactRepository.createContact(
                match<ContactRecordInput> { input ->
                    input.crm?.kind == ContactFormState.KIND_ANIMAL && input.card?.language == "de"
                },
            )
        }
    }

    @Test
    fun `selected circles and tags become memberships after create`() = runTest(mainDispatcherRule.testDispatcher) {
        // M24: the form applies memberships via the CircleMember/ContactTag sub-resources,
        // like web's AddContactDialog — the PUT's crm.circles only touches the flat column.
        val created = ContactRecordResponse(id = 9, uid = "u9", card = Card(name = Name(full = "Carol King")))
        coEvery { contactRepository.createContact(any()) } returns Result.success(created)

        val vm = createViewModel()
        coEvery { circleRepository.list(any(), any()) } returns Result.success(listOf(Circle(id = "c1", name = "friends"), Circle(id = "c2", name = "family")))
        coEvery { tagRepository.list(any(), any()) } returns Result.success(listOf(Tag(id = "t1", name = "close")))
        coEvery { circleRepository.circlesForContact("u9") } returns Result.success(emptyList())
        coEvery { tagRepository.tagsForContact("u9") } returns Result.success(emptyList())
        coEvery { circleRepository.addMember("c1", "u9") } returns Result.success(
            com.mycorrhizal.crm.model.network.CircleMember(circleId = "c1", memberVCardUid = "u9"),
        )
        coEvery { tagRepository.addContact("t1", "u9") } returns Result.success(
            com.mycorrhizal.crm.model.network.ContactTag(tagId = "t1", contactVCardUid = "u9"),
        )
        vm.onGivenNameChange("Carol")
        vm.onCircleToggle("friends")
        vm.onTagToggle("close")
        vm.save()
        advanceUntilIdle()

        coVerify { circleRepository.addMember("c1", "u9") }
        coVerify { tagRepository.addContact("t1", "u9") }
        assertEquals(ContactFormEvent.Saved, vm.events.value)
    }

    @Test
    fun `deselecting a membership removes it in edit mode`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(
            id = 5,
            uid = "u5",
            card = Card(
                name = Name(
                    full = "Dana White",
                    components = listOf(
                        com.mycorrhizal.crm.model.network.NameComponent(kind = "given", value = "Dana"),
                        com.mycorrhizal.crm.model.network.NameComponent(kind = "surname", value = "White"),
                    ),
                ),
            ),
            crm = CRMEnvelope(circles = listOf("friends")),
        )
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { contactRepository.updateContact(5, any()) } returns Result.success(record)

        val vm = createViewModel(5)
        // Re-stub after creation (last-registered-wins) so the edit-mode membership load
        // sees the pre-edit membership and can detect the deselection.
        coEvery { circleRepository.circlesForContact("u5") } returns Result.success(listOf(Circle(id = "c1", name = "friends")))
        coEvery { tagRepository.tagsForContact("u5") } returns Result.success(emptyList())
        coEvery { circleRepository.removeMember("c1", "u5") } returns Result.success(Unit)
        advanceUntilIdle()
        assertEquals(listOf("friends"), vm.uiState.value.circles)

        vm.onCircleToggle("friends") // deselect
        vm.save()
        advanceUntilIdle()

        coVerify { circleRepository.removeMember("c1", "u5") }
    }

    @Test
    fun `saved phones default to the cell context`() = runTest(mainDispatcherRule.testDispatcher) {
        // Write-parity with the web form's MultiValueField `defaultType="cell"`,
        // which lands in `contexts: ["cell"]` — visible to BOTH platforms' SMS
        // detection (the old `label="cell"` was invisible to the web).
        val created = ContactRecordResponse(id = 9, card = Card(name = Name(full = "Carol King")))
        coEvery { contactRepository.createContact(any()) } returns Result.success(created)

        val vm = createViewModel()
        vm.onGivenNameChange("Carol")
        vm.onSurnameChange("King")
        vm.onPhonesChange(listOf(Phone(number = "+1-555-0100", contexts = listOf("cell"))))
        vm.save()
        advanceUntilIdle()

        coVerify {
            contactRepository.createContact(
                match<ContactRecordInput> { input ->
                    input.card?.phones?.firstOrNull()?.number == "+1-555-0100" &&
                        input.card?.phones?.firstOrNull()?.contexts == listOf("cell")
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
        vm.onEmailsChange(listOf(Email(address = "x@example.com")))
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
        vm.onPhonesChange(listOf(Phone(number = "1234", label = "cell")))
        vm.save()
        advanceUntilIdle()

        assertEquals(ContactFormEvent.Saved, vm.events.value)
        coVerify { contactRepository.createContact(any()) }
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
                phones = listOf(Phone(id = "phone-1", number = "+1-555-0100", contexts = listOf("work"))),
            ),
        )
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { contactRepository.updateContact(5, any()) } returns Result.success(record)

        val vm = createViewModel(5)
        advanceUntilIdle()
        // M7: the add path goes through the editor's `onChange(items + spec.blank())`,
        // which is what mints a genuinely-new phone with the `cell` default.
        vm.onPhonesChange(vm.uiState.value.phones + PhoneSpec.blank())
        vm.onPhonesChange(vm.uiState.value.phones.mapIndexed { i, p -> if (i == 1) p.copy(number = "+1-555-0199") else p })
        vm.save()
        advanceUntilIdle()

        coVerify {
            contactRepository.updateContact(
                5,
                match<ContactRecordInput> { input ->
                    val phones = input.card?.phones.orEmpty()
                    phones.any { it.id == "phone-1" && it.number == "+1-555-0100" && it.contexts == listOf("work") } &&
                        phones.any { it.id == null && it.number == "+1-555-0199" && it.contexts == listOf("cell") }
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

    // --- M7 ---

    @Test
    fun `editing email and phone type and preferred preserves id contexts and pref`() = runTest(mainDispatcherRule.testDispatcher) {
        // The whole point of the withValue/withType -> .copy() contract (M7 test case 1):
        // changing a phone's number and label must not touch id/contexts/pref/features, and
        // setting a phone preferred must clear it on the other phone (list-level pref).
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
                    Phone(id = "phone-1", number = "+1-555-0100", label = "work", contexts = listOf("work"), pref = 1, features = listOf("voice")),
                    Phone(id = "phone-2", number = "+1-555-0199", label = "cell", contexts = listOf("cell")),
                ),
                emails = listOf(
                    Email(id = "email-1", address = "dana@example.com", label = "work", contexts = listOf("work"), pref = 1),
                ),
            ),
        )
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { contactRepository.updateContact(5, any()) } returns Result.success(record)

        val vm = createViewModel(5)
        advanceUntilIdle()

        // Edit phone-1's number and retype it "home" (contexts[0]); make phone-2 preferred.
        // (The pref-exclusivity itself is the editor's job — this simulates its output,
        // where toggling pref on phone-2 cleared it on phone-1.)
        vm.onPhonesChange(
            vm.uiState.value.phones.mapIndexed { i, p ->
                when (i) {
                    0 -> p.copy(number = "+1-555-0000", contexts = listOf("home"), pref = null)
                    1 -> p.copy(pref = 1)
                    else -> p
                }
            },
        )
        vm.onEmailsChange(vm.uiState.value.emails.mapIndexed { i, e -> if (i == 0) e.copy(contexts = listOf("home")) else e })
        vm.save()
        advanceUntilIdle()

        coVerify {
            contactRepository.updateContact(
                5,
                match<ContactRecordInput> { input ->
                    val phones = input.card?.phones.orEmpty()
                    val phone1 = phones.first { it.id == "phone-1" }
                    val phone2 = phones.first { it.id == "phone-2" }
                    phone1.number == "+1-555-0000" && phone1.contexts == listOf("home") &&
                        phone1.id == "phone-1" && phone1.label == "work" &&
                        phone1.pref == null && phone1.features == listOf("voice") &&
                        phone2.pref == 1 && phone2.number == "+1-555-0199" && phone2.contexts == listOf("cell") &&
                        input.card?.emails?.firstOrNull()?.contexts == listOf("home") &&
                        input.card?.emails?.firstOrNull()?.id == "email-1" &&
                        input.card?.emails?.firstOrNull()?.label == "work" &&
                        input.card?.emails?.firstOrNull()?.pref == 1
                },
            )
        }
    }

    @Test
    fun `addresses round-trip with registry kinds and preserve id contexts and pref`() = runTest(mainDispatcherRule.testDispatcher) {
        // M7 test case 5: the editor emits the real registry kinds (`name` for street,
        // `locality` for city — NOT `street`/`city`, the T67 bug), and a save preserves
        // id/contexts/pref on the loaded entry.
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
                    Address(
                        id = "addr-1",
                        contexts = listOf("home", "delivery"),
                        pref = 1,
                        components = listOf(
                            AddressComponent(kind = "name", value = "123 Main St"),
                            AddressComponent(kind = "locality", value = "Springfield"),
                            AddressComponent(kind = "postcode", value = "12345"),
                            AddressComponent(kind = "country", value = "US"),
                        ),
                    ),
                ),
            ),
        )
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { contactRepository.updateContact(5, any()) } returns Result.success(record)

        val vm = createViewModel(5)
        advanceUntilIdle()
        assertEquals(listOf("home", "delivery"), vm.uiState.value.addresses.firstOrNull()?.contexts)

        // Change the city on the loaded address.
        vm.onAddressesChange(
            vm.uiState.value.addresses.map { addr ->
                addr.copy(components = addr.components.orEmpty().map { c ->
                    if (c.kind == "locality") c.copy(value = "Shelbyville") else c
                })
            },
        )
        vm.save()
        advanceUntilIdle()

        coVerify {
            contactRepository.updateContact(
                5,
                match<ContactRecordInput> { input ->
                    val addr = input.card?.addresses?.firstOrNull()
                    val comps = addr?.components.orEmpty()
                    comps.first { it.kind == "name" }.value == "123 Main St" &&
                        comps.first { it.kind == "locality" }.value == "Shelbyville" &&
                        comps.first { it.kind == "postcode" }.value == "12345" &&
                        addr?.id == "addr-1" && addr.pref == 1
                },
            )
        }
    }

    @Test
    fun `editing an address preserves its extra contexts`() = runTest(mainDispatcherRule.testDispatcher) {
        // M7 AddressEditor is deliberately better than web here: contexts[1..] survive a
        // save (web's valuesToCardAddresses collapses to just contexts[0]).
        val record = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White", components = listOf(
                    com.mycorrhizal.crm.model.network.NameComponent(kind = "given", value = "Dana"),
                )),
                addresses = listOf(
                    Address(
                        id = "addr-1",
                        contexts = listOf("home", "delivery"),
                        components = listOf(AddressComponent(kind = "name", value = "1 Elm St")),
                    ),
                ),
            ),
        )
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { contactRepository.updateContact(5, any()) } returns Result.success(record)

        val vm = createViewModel(5)
        advanceUntilIdle()

        // Keep the first context but drop the second — mimics the type dropdown editing
        // contexts[0] while withDraft preserves contexts[1..].
        vm.onAddressesChange(
            vm.uiState.value.addresses.map { addr ->
                addr.copy(contexts = listOf("work") + addr.contexts.orEmpty().drop(1))
            },
        )
        vm.save()
        advanceUntilIdle()

        coVerify {
            contactRepository.updateContact(
                5,
                match<ContactRecordInput> { input ->
                    input.card?.addresses?.firstOrNull()?.contexts == listOf("work", "delivery")
                },
            )
        }
    }

    @Test
    fun `online services links titles and personal info round-trip`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White", components = listOf(
                    com.mycorrhizal.crm.model.network.NameComponent(kind = "given", value = "Dana"),
                )),
                imppAddresses = listOf(OnlineService(id = "impp-1", service = "Signal", uri = "tel:+15550001", contexts = listOf("work"))),
                socialProfiles = listOf(OnlineService(id = "social-1", service = "Mastodon", user = "@dana", contexts = listOf("home"))),
                otherOnlineServices = listOf(OnlineService(id = "other-1", service = "Website", uri = "https://example.com")),
                links = listOf(Resource(id = "link-1", uri = "https://example.com/home", label = "Website")),
                titles = listOf(Title(id = "title-1", name = "Engineer", kind = "title")),
                personalInfo = listOf(PersonalInfo(id = "pi-1", kind = "hobby", value = "climbing", level = "medium")),
            ),
        )
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { contactRepository.updateContact(5, any()) } returns Result.success(record)

        val vm = createViewModel(5)
        advanceUntilIdle()

        // Loaded as real objects, none of the unshown metadata narrowed away.
        assertEquals(listOf(OnlineService(id = "impp-1", service = "Signal", uri = "tel:+15550001", contexts = listOf("work"))), vm.uiState.value.imppAddresses)
        assertEquals(listOf(OnlineService(id = "social-1", service = "Mastodon", user = "@dana", contexts = listOf("home"))), vm.uiState.value.socialProfiles)
        assertEquals(listOf(Resource(id = "link-1", uri = "https://example.com/home", label = "Website")), vm.uiState.value.links)
        assertEquals(listOf(Title(id = "title-1", name = "Engineer", kind = "title")), vm.uiState.value.titles)
        assertEquals(listOf(PersonalInfo(id = "pi-1", kind = "hobby", value = "climbing", level = "medium")), vm.uiState.value.personalInfo)

        // Edit a handle and retype it; save must keep id/service/contexts.
        vm.onImppChange(vm.uiState.value.imppAddresses.mapIndexed { i, s -> if (i == 0) s.copy(uri = "tel:+15559999", contexts = listOf("home")) else s })
        vm.onSocialChange(vm.uiState.value.socialProfiles.mapIndexed { i, s -> if (i == 0) s.copy(user = "@dana2") else s })
        vm.onLinksChange(vm.uiState.value.links.mapIndexed { i, l -> if (i == 0) l.copy(uri = "https://example.com/new") else l })
        vm.onPersonalInfoChange(vm.uiState.value.personalInfo.mapIndexed { i, p -> if (i == 0) p.copy(value = "trail running") else p })
        vm.save()
        advanceUntilIdle()

        coVerify {
            contactRepository.updateContact(
                5,
                match<ContactRecordInput> { input ->
                    val impp = input.card?.imppAddresses?.firstOrNull()
                    val social = input.card?.socialProfiles?.firstOrNull()
                    val link = input.card?.links?.firstOrNull()
                    val pi = input.card?.personalInfo?.firstOrNull()
                    val title = input.card?.titles?.firstOrNull()
                    impp?.uri == "tel:+15559999" && impp.contexts == listOf("home") && impp.service == "Signal" && impp.id == "impp-1" &&
                        social?.user == "@dana2" && social.id == "social-1" && social.service == "Mastodon" &&
                        link?.uri == "https://example.com/new" && link.label == "Website" && link.id == "link-1" &&
                        pi?.value == "trail running" && pi.kind == "hobby" && pi.id == "pi-1" && pi.level == "medium" &&
                        title?.name == "Engineer" && title.kind == "title" && title.id == "title-1"
                },
            )
        }
    }

    @Test
    fun `organization and department map onto the first organization preserving extras`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White", components = listOf(
                    com.mycorrhizal.crm.model.network.NameComponent(kind = "given", value = "Dana"),
                )),
                organizations = listOf(
                    Organization(id = "org-1", name = "Acme", units = listOf(com.mycorrhizal.crm.model.network.OrgUnit(name = "R&D"))),
                    Organization(id = "org-2", name = "Side project"),
                ),
            ),
        )
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { contactRepository.updateContact(5, any()) } returns Result.success(record)

        val vm = createViewModel(5)
        advanceUntilIdle()
        assertEquals("Acme", vm.uiState.value.organizationName)
        assertEquals("R&D", vm.uiState.value.department)

        vm.onOrganizationNameChange("Wayne Enterprises")
        vm.onDepartmentChange("Applied Sciences")
        vm.save()
        advanceUntilIdle()

        coVerify {
            contactRepository.updateContact(
                5,
                match<ContactRecordInput> { input ->
                    val orgs = input.card?.organizations.orEmpty()
                    orgs.firstOrNull()?.name == "Wayne Enterprises" &&
                        orgs.firstOrNull()?.units?.firstOrNull()?.name == "Applied Sciences" &&
                        orgs.firstOrNull()?.id == "org-1" && // id preserved via .copy
                        orgs.getOrNull(1)?.name == "Side project" && // extra org preserved
                        orgs.getOrNull(1)?.id == "org-2"
                },
            )
        }
    }

    @Test
    fun `blank organization name drops the first organization but keeps extras`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White", components = listOf(
                    com.mycorrhizal.crm.model.network.NameComponent(kind = "given", value = "Dana"),
                )),
                organizations = listOf(
                    Organization(id = "org-1", name = "Acme"),
                    Organization(id = "org-2", name = "Side project"),
                ),
            ),
        )
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { contactRepository.updateContact(5, any()) } returns Result.success(record)

        val vm = createViewModel(5)
        advanceUntilIdle()
        vm.onOrganizationNameChange("") // clear
        vm.save()
        advanceUntilIdle()

        coVerify {
            contactRepository.updateContact(
                5,
                match<ContactRecordInput> { input ->
                    val orgs = input.card?.organizations.orEmpty()
                    orgs.size == 1 && orgs.first().id == "org-2"
                },
            )
        }
    }

    @Test
    fun `crm envelope fields are sent on save and blank preserves the loaded value`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(
            id = 5,
            card = Card(name = Name(full = "Dana White", components = listOf(
                com.mycorrhizal.crm.model.network.NameComponent(kind = "given", value = "Dana"),
            ))),
            crm = CRMEnvelope(
                howWeMet = "At the climbing gym",
                workInformation = "Full-stack at Acme",
                contactInformation = "Prefers email",
            ),
        )
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { contactRepository.updateContact(5, any()) } returns Result.success(record)

        val vm = createViewModel(5)
        advanceUntilIdle()
        assertEquals("At the climbing gym", vm.uiState.value.howWeMet)

        vm.onHowWeMetChange("Introduced by Sam")
        vm.onWorkInformationChange("") // blank -> preserve loaded value
        vm.save()
        advanceUntilIdle()

        coVerify {
            contactRepository.updateContact(
                5,
                match<ContactRecordInput> { input ->
                    input.crm?.howWeMet == "Introduced by Sam" &&
                        input.crm?.workInformation == "Full-stack at Acme" &&
                        input.crm?.contactInformation == "Prefers email"
                },
            )
        }
    }

    @Test
    fun `create mode sends addresses online services links titles and crm strings`() = runTest(mainDispatcherRule.testDispatcher) {
        val created = ContactRecordResponse(id = 9, card = Card(name = Name(full = "Carol King")))
        coEvery { contactRepository.createContact(any()) } returns Result.success(created)

        val vm = createViewModel()
        vm.onGivenNameChange("Carol")
        vm.onAddressesChange(listOf(Address(components = listOf(AddressComponent(kind = "name", value = "1 Main St"), AddressComponent(kind = "locality", value = "Metropolis")))))
        vm.onOrganizationNameChange("Daily Planet")
        vm.onTitlesChange(listOf(Title(name = "Reporter", kind = "title")))
        vm.onImppChange(listOf(OnlineService(service = "Signal", uri = "tel:+15550001")))
        vm.onLinksChange(listOf(Resource(uri = "https://example.com")))
        vm.onPersonalInfoChange(listOf(PersonalInfo(kind = "hobby", value = "flying")))
        vm.onHowWeMetChange("At the newsroom")
        vm.save()
        advanceUntilIdle()

        coVerify {
            contactRepository.createContact(
                match<ContactRecordInput> { input ->
                    input.card?.addresses?.firstOrNull()?.components?.firstOrNull { it.kind == "name" }?.value == "1 Main St" &&
                        input.card?.addresses?.firstOrNull()?.components?.firstOrNull { it.kind == "locality" }?.value == "Metropolis" &&
                        input.card?.organizations?.firstOrNull()?.name == "Daily Planet" &&
                        input.card?.titles?.firstOrNull()?.name == "Reporter" &&
                        input.card?.imppAddresses?.firstOrNull()?.uri == "tel:+15550001" &&
                        input.card?.links?.firstOrNull()?.uri == "https://example.com" &&
                        input.card?.personalInfo?.firstOrNull()?.value == "flying" &&
                        input.crm?.howWeMet == "At the newsroom"
                },
            )
        }
    }

    @Test
    fun `type option lists mirror the web contactFields copies`() {
        // M7 test case 6: hardcoded mirrors of frontend/src/contactFields.ts
        // (CONTACT_TYPE_OPTIONS / CONTEXT_OPTIONS / PERSONAL_INFO_KIND_OPTIONS).
        // No dynamic type-list endpoint exists in this codebase, by design.
        assertEquals(listOf("home", "work", "cell", "fax", "other"), CONTACT_TYPE_OPTIONS)
        assertEquals(listOf("private", "work", "school", "billing", "delivery"), CONTEXT_OPTIONS)
        assertEquals(listOf("expertise", "hobby", "interest"), PERSONAL_INFO_KIND_OPTIONS)
        assertEquals(listOf("title", "role"), TitleSpec.typeOptions)
        // Email/Phone/Link share the standard vCard type set; online services use
        // the RFC 9554 context set (web's OnlineServiceEditor).
        assertEquals(EmailSpec.typeOptions, PhoneSpec.typeOptions)
        assertEquals(PhoneSpec.typeOptions, LinkSpec.typeOptions)
        assertEquals(CONTEXT_OPTIONS, OnlineServiceSpec.typeOptions)
    }
}
