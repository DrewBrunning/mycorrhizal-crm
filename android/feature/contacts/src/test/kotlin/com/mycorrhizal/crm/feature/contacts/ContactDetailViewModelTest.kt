package com.mycorrhizal.crm.feature.contacts

import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.CircleRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.ExternalIdentityRepository
import com.mycorrhizal.crm.domain.repository.FieldDefinitionRepository
import com.mycorrhizal.crm.domain.repository.ImmichRepository
import com.mycorrhizal.crm.domain.repository.NextcloudRepository
import com.mycorrhizal.crm.domain.repository.PaperlessRepository
import com.mycorrhizal.crm.domain.repository.ReminderRepository
import com.mycorrhizal.crm.domain.repository.SeafileRepository
import com.mycorrhizal.crm.domain.repository.SessionState
import com.mycorrhizal.crm.domain.repository.TagRepository
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactFieldValuesInput
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.ExternalIdentity
import com.mycorrhizal.crm.model.network.FieldDefinition
import com.mycorrhizal.crm.model.network.FieldValue
import com.mycorrhizal.crm.model.network.FieldValueInput
import com.mycorrhizal.crm.model.network.ImmichPerson
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.model.network.PaperlessDocument
import com.mycorrhizal.crm.model.network.SeafileItem
import com.mycorrhizal.crm.model.network.SeafileLibrary
import com.mycorrhizal.crm.model.network.SeafileLinkRequest
import com.mycorrhizal.crm.model.network.WebDAVItem
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import com.mycorrhizal.crm.ui.R
import androidx.lifecycle.SavedStateHandle
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import io.mockk.slot
import kotlinx.coroutines.CompletableDeferred
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
    private val fieldDefinitionRepository = mockk<FieldDefinitionRepository>()
    private val circleRepository = mockk<CircleRepository>()
    private val tagRepository = mockk<TagRepository>()
    private val externalIdentityRepository = mockk<ExternalIdentityRepository>()
    private val immichRepository = mockk<ImmichRepository>()
    private val paperlessRepository = mockk<PaperlessRepository>()
    private val seafileRepository = mockk<SeafileRepository>()
    private val nextcloudRepository = mockk<NextcloudRepository>()

    private fun viewModel(
        id: Int,
        dateFormat: String? = null,
        selfContactVCardUid: String? = null,
        enabledContactFields: List<String>? = null,
    ): ContactDetailViewModel {
        coEvery { contactRepository.getDeviceLookupKey(any()) } returns null
        every { authRepository.observeSession() } returns
            flowOf(
                SessionState(
                    dateFormat = dateFormat,
                    selfContactVCardUid = selfContactVCardUid,
                    enabledContactFields = enabledContactFields,
                ),
            )
        coEvery { fieldDefinitionRepository.list() } returns Result.success(emptyList())
        coEvery { fieldDefinitionRepository.contactValues(any()) } returns Result.success(emptyList())
        stubMemberships()
        stubCompletions()
        stubExternalLinks()
        return ContactDetailViewModel(
            contactRepository,
            reminderRepository,
            authRepository,
            fieldDefinitionRepository,
            circleRepository,
            tagRepository,
            externalIdentityRepository,
            immichRepository,
            paperlessRepository,
            seafileRepository,
            nextcloudRepository,
            SavedStateHandle(mapOf("contactId" to id)),
        )
    }

    /**
     * Issue #220/#236: default stubs for the External Links panel's loads
     * (run on every load()) — Immich plus the three file-system
     * "configured" gates.
     */
    private fun stubExternalLinks() {
        coEvery { externalIdentityRepository.listForContact(any()) } returns Result.success(emptyList())
        coEvery { immichRepository.isConfigured() } returns Result.success(false)
        coEvery { immichRepository.getContactSummary(any()) } returns Result.success(null)
        coEvery { paperlessRepository.isConfigured() } returns Result.success(false)
        coEvery { seafileRepository.isConfigured() } returns Result.success(false)
        coEvery { nextcloudRepository.isConfigured() } returns Result.success(false)
    }

    /** M24: default stubs for the inline circle/tag editor loads (run on every load()). */
    private fun stubMemberships() {
        coEvery { circleRepository.circlesForContact(any()) } returns Result.success(emptyList())
        coEvery { circleRepository.list(any(), any()) } returns Result.success(emptyList())
        coEvery { tagRepository.tagsForContact(any()) } returns Result.success(emptyList())
        coEvery { tagRepository.list(any(), any()) } returns Result.success(emptyList())
    }

    /** M20: default stub for the completion-timeline load (run on every load()). */
    private fun stubCompletions() {
        coEvery { reminderRepository.listCompletions(any()) } returns Result.success(emptyList())
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
        coEvery { fieldDefinitionRepository.list() } returns Result.success(emptyList())
        coEvery { fieldDefinitionRepository.contactValues(any()) } returns Result.success(emptyList())
        coEvery { reminderRepository.listCompletions(any()) } returns Result.success(emptyList())
        stubExternalLinks()

        val vm = ContactDetailViewModel(
            contactRepository,
            reminderRepository,
            authRepository,
            fieldDefinitionRepository,
            circleRepository,
            tagRepository,
            externalIdentityRepository,
            immichRepository,
            paperlessRepository,
            seafileRepository,
            nextcloudRepository,
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
        coEvery { fieldDefinitionRepository.list() } returns Result.success(
            listOf(FieldDefinition(id = "d1", label = "Coffee order", type = "string")),
        )
        coEvery { fieldDefinitionRepository.contactValues(5) } returns Result.success(
            listOf(FieldValue(id = 1, fieldDefinitionId = "d1", value = "Latte")),
        )
        coEvery { reminderRepository.listCompletions(any()) } returns Result.success(emptyList())

        stubExternalLinks()

        val vm = ContactDetailViewModel(contactRepository, reminderRepository, authRepository, fieldDefinitionRepository, circleRepository, tagRepository, externalIdentityRepository, immichRepository, paperlessRepository, seafileRepository, nextcloudRepository, SavedStateHandle(mapOf("contactId" to 5)))
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
        coEvery { fieldDefinitionRepository.list() } returns Result.failure(ApiError.Server(500, "boom"))
        coEvery { fieldDefinitionRepository.contactValues(5) } returns Result.success(emptyList())
        coEvery { reminderRepository.listCompletions(any()) } returns Result.success(emptyList())

        stubExternalLinks()

        val vm = ContactDetailViewModel(contactRepository, reminderRepository, authRepository, fieldDefinitionRepository, circleRepository, tagRepository, externalIdentityRepository, immichRepository, paperlessRepository, seafileRepository, nextcloudRepository, SavedStateHandle(mapOf("contactId" to 5)))
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
        coEvery { fieldDefinitionRepository.list() } returns Result.success(
            listOf(FieldDefinition(id = "d1", label = "Coffee order", type = "string")),
        )
        coEvery { fieldDefinitionRepository.contactValues(5) } returns Result.failure(ApiError.Network(java.io.IOException("offline")))
        coEvery { reminderRepository.listCompletions(any()) } returns Result.success(emptyList())

        stubExternalLinks()

        val vm = ContactDetailViewModel(contactRepository, reminderRepository, authRepository, fieldDefinitionRepository, circleRepository, tagRepository, externalIdentityRepository, immichRepository, paperlessRepository, seafileRepository, nextcloudRepository, SavedStateHandle(mapOf("contactId" to 5)))
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
        coEvery { fieldDefinitionRepository.list() } returns Result.success(emptyList()) // zero definitions
        coEvery { fieldDefinitionRepository.contactValues(5) } returns Result.success(
            listOf(FieldValue(id = 1, fieldDefinitionId = "deleted-def", value = "orphaned")),
        )
        coEvery { reminderRepository.listCompletions(any()) } returns Result.success(emptyList())

        stubExternalLinks()

        val vm = ContactDetailViewModel(contactRepository, reminderRepository, authRepository, fieldDefinitionRepository, circleRepository, tagRepository, externalIdentityRepository, immichRepository, paperlessRepository, seafileRepository, nextcloudRepository, SavedStateHandle(mapOf("contactId" to 5)))
        advanceUntilIdle()

        val state = vm.uiState.value
        assertNull(state.error)
        assertTrue(state.fieldDefinitions.isEmpty())
        assertEquals("orphaned", state.fieldValuesByDefinitionId["deleted-def"]) // present but unreachable — no def to render it under
    }

    // --- M24: delete / archive / unarchive / export ---

    @Test
    fun `deleteContact emits ContactDeleted on success`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { contactRepository.deleteContact(5) } returns Result.success(Unit)

        val vm = viewModel(5)
        advanceUntilIdle()

        vm.deleteContact()
        advanceUntilIdle()

        assertEquals(ContactDetailEvent.ContactDeleted, vm.events.value)
        assertFalse(vm.uiState.value.isMutating)
        io.mockk.coVerify(exactly = 1) { contactRepository.deleteContact(5) }
    }

    @Test
    fun `deleteContact failure surfaces the error and keeps the screen alive`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { contactRepository.deleteContact(5) } returns Result.failure(ApiError.Server(500, "boom"))

        val vm = viewModel(5)
        advanceUntilIdle()

        vm.deleteContact()
        advanceUntilIdle()

        assertEquals(null, vm.events.value)
        assertEquals("Server error (500)", vm.uiState.value.error)
        assertFalse(vm.uiState.value.isMutating)
    }

    @Test
    fun `setArchived true calls archiveContact and reloads the contact`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { contactRepository.archiveContact(5) } returns Result.success(Unit)

        val vm = viewModel(5)
        advanceUntilIdle()

        vm.setArchived(archived = true)
        advanceUntilIdle()

        io.mockk.coVerify(exactly = 1) { contactRepository.archiveContact(5) }
        // Reloaded after the flip.
        io.mockk.coVerify(atLeast = 2) { contactRepository.getContact(5) }
        assertEquals(null, vm.events.value)
        assertEquals(null, vm.uiState.value.error)
    }

    @Test
    fun `setArchived false calls unarchiveContact`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { contactRepository.unarchiveContact(5) } returns Result.success(Unit)

        val vm = viewModel(5)
        advanceUntilIdle()

        vm.setArchived(archived = false)
        advanceUntilIdle()

        io.mockk.coVerify(exactly = 1) { contactRepository.unarchiveContact(5) }
    }

    @Test
    fun `archive failure surfaces the error and does not reload`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { contactRepository.archiveContact(5) } returns Result.failure(ApiError.Client(404, "Contact not found"))

        val vm = viewModel(5)
        advanceUntilIdle()

        vm.setArchived(archived = true)
        advanceUntilIdle()

        assertEquals("Not found", vm.uiState.value.error)
        io.mockk.coVerify(exactly = 1) { contactRepository.getContact(5) }
    }

    // --- Issue #212: favorite toggle (web #173) ---

    @Test
    fun `toggleFavorite flips the header star optimistically and calls the repository`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")), isFavorite = false)
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { contactRepository.favoriteContact(5) } returns Result.success(Unit)

        val vm = viewModel(5)
        advanceUntilIdle()
        assertFalse(vm.uiState.value.contact?.isFavorite!!)

        vm.toggleFavorite()
        advanceUntilIdle()

        assertTrue(vm.uiState.value.contact?.isFavorite!!)
        io.mockk.coVerify(exactly = 1) { contactRepository.favoriteContact(5) }
        assertEquals(null, vm.uiState.value.error)
    }

    @Test
    fun `toggleFavorite on a favorite calls unfavorite and keeps the flip`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")), isFavorite = true)
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { contactRepository.unfavoriteContact(5) } returns Result.success(Unit)

        val vm = viewModel(5)
        advanceUntilIdle()
        assertTrue(vm.uiState.value.contact?.isFavorite!!)

        vm.toggleFavorite()
        advanceUntilIdle()

        assertFalse(vm.uiState.value.contact?.isFavorite!!)
        io.mockk.coVerify(exactly = 1) { contactRepository.unfavoriteContact(5) }
    }

    @Test
    fun `toggleFavorite rolls back on failure so the star can't disagree with the DB`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")), isFavorite = false)
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { contactRepository.favoriteContact(5) } returns Result.failure(ApiError.Server(500, "boom"))

        val vm = viewModel(5)
        advanceUntilIdle()

        vm.toggleFavorite()
        advanceUntilIdle()

        assertFalse(vm.uiState.value.contact?.isFavorite!!)
        assertEquals("Server error (500)", vm.uiState.value.error)
    }

    // --- Issue #832: Contact field settings (web parity) ---

    @Test
    fun `state resolves a never-configured session value to the default enabled set`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
            coEvery { contactRepository.getContact(5) } returns Result.success(record)

            val vm = viewModel(5, enabledContactFields = null)
            advanceUntilIdle()

            assertEquals(com.mycorrhizal.crm.model.network.DEFAULT_ENABLED_CONTACT_FIELDS, vm.uiState.value.enabledFields)
        }

    @Test
    fun `state resolves an explicit empty session value to no fields enabled`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
            coEvery { contactRepository.getContact(5) } returns Result.success(record)

            val vm = viewModel(5, enabledContactFields = emptyList())
            advanceUntilIdle()

            assertTrue(vm.uiState.value.enabledFields.isEmpty())
        }

    @Test
    fun `state resolves a populated session value to the matching keys`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        val vm = viewModel(5, enabledContactFields = listOf("emails", "keywords"))
        advanceUntilIdle()

        assertEquals(
            setOf(com.mycorrhizal.crm.model.network.ContactFieldKey.EMAILS, com.mycorrhizal.crm.model.network.ContactFieldKey.KEYWORDS),
            vm.uiState.value.enabledFields,
        )
    }

    // --- T90 / issue #831: "Mark as Me" self-contact pointer (web parity) ---

    @Test
    fun `state exposes the self contact vcard uid from the session`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        val vm = viewModel(5, selfContactVCardUid = "u5")
        advanceUntilIdle()

        assertEquals("u5", vm.uiState.value.selfContactVCardUid)
    }

    @Test
    fun `toggleMe marks the contact as me by sending its own uid`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { authRepository.updateSelfContact("u5") } returns Result.success(Unit)

        // No self contact set yet — this contact isn't the pointer.
        val vm = viewModel(5, selfContactVCardUid = null)
        advanceUntilIdle()

        vm.toggleMe()
        advanceUntilIdle()

        coVerify(exactly = 1) { authRepository.updateSelfContact("u5") }
        assertEquals(null, vm.uiState.value.error)
        assertFalse(vm.uiState.value.isMutating)
    }

    @Test
    fun `toggleMe unmarks the contact by sending a null uid`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { authRepository.updateSelfContact(null) } returns Result.success(Unit)

        // This contact IS already the pointer.
        val vm = viewModel(5, selfContactVCardUid = "u5")
        advanceUntilIdle()

        vm.toggleMe()
        advanceUntilIdle()

        coVerify(exactly = 1) { authRepository.updateSelfContact(null) }
    }

    @Test
    fun `toggleMe surfaces an error on failure`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { authRepository.updateSelfContact("u5") } returns Result.failure(ApiError.Server(500, "boom"))

        val vm = viewModel(5, selfContactVCardUid = null)
        advanceUntilIdle()

        vm.toggleMe()
        advanceUntilIdle()

        assertEquals("Server error (500)", vm.uiState.value.error)
        assertFalse(vm.uiState.value.isMutating)
    }

    @Test
    fun `toggleMe is a no-op without a loaded contact`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = viewModel(0)
        advanceUntilIdle()

        vm.toggleMe()
        advanceUntilIdle()

        io.mockk.coVerify(exactly = 0) { authRepository.updateSelfContact(any()) }
    }

    @Test
    fun `exportVcf emits ExportReady with the raw bytes`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        val bytes = "BEGIN:VCARD\r\nEND:VCARD\r\n".toByteArray()
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { contactRepository.exportContactVcf("u5", null) } returns Result.success(bytes)

        val vm = viewModel(5)
        advanceUntilIdle()

        vm.exportVcf()
        advanceUntilIdle()

        val event = vm.events.value
        assertTrue(event is ContactDetailEvent.ExportReady)
        val ready = event as ContactDetailEvent.ExportReady
        assertEquals(null, ready.version)
        assertEquals(bytes.contentToString(), ready.bytes.contentToString())
    }

    @Test
    fun `exportVcf failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { contactRepository.exportContactVcf("u5", 3) } returns Result.failure(ApiError.Client(400, "bad request"))

        val vm = viewModel(5)
        advanceUntilIdle()

        vm.exportVcf(version = 3)
        advanceUntilIdle()

        assertEquals(null, vm.events.value)
        assertEquals("bad request", vm.uiState.value.error)
    }

    @Test
    fun `exportVcf without a contact uid is a no-op`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        val vm = viewModel(5)
        advanceUntilIdle()

        vm.exportVcf()
        advanceUntilIdle()

        assertEquals(null, vm.events.value)
        io.mockk.coVerify(exactly = 0) { contactRepository.exportContactVcf(any(), any()) }
    }

    // --- Issue #219: profile-photo upload ---

    @Test
    fun `uploadPhoto uploads the bytes and reloads the contact on success`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        val vm = viewModel(5)
        advanceUntilIdle()

        val bytes = ByteArray(64) { it.toByte() }
        coEvery { contactRepository.uploadPhoto(5, bytes, "image/jpeg") } returns Result.success(Unit)
        vm.uploadPhoto(bytes, "image/jpeg")
        advanceUntilIdle()

        io.mockk.coVerify(exactly = 1) { contactRepository.uploadPhoto(5, bytes, "image/jpeg") }
        // The contact is refetched so the new photoThumbnail/card.photoUri renders immediately.
        io.mockk.coVerify(atLeast = 2) { contactRepository.getContact(5) }
        assertFalse(vm.uiState.value.isMutating)
        assertEquals(null, vm.uiState.value.error)
    }

    @Test
    fun `uploadPhoto failure surfaces the error and does not reload`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        val vm = viewModel(5)
        advanceUntilIdle()

        val bytes = ByteArray(64) { it.toByte() }
        coEvery { contactRepository.uploadPhoto(5, bytes, "image/jpeg") } returns
            Result.failure(ApiError.Client(400, "File too large. Maximum size is 10MB"))
        vm.uploadPhoto(bytes, "image/jpeg")
        advanceUntilIdle()

        assertEquals("File too large. Maximum size is 10MB", vm.uiState.value.error)
        assertFalse(vm.uiState.value.isMutating)
        io.mockk.coVerify(exactly = 1) { contactRepository.getContact(5) }
    }

    // --- CON-01..04 style: the isMutating double-submit guard shared by
    // delete/archive/export/upload. Coverage-analysis follow-up: nothing
    // proved the early return in each of these actually blocks a second
    // concurrent call while one is in flight. ---

    @Test
    fun `deleteContact ignores a second call while the first is in flight`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        val gate = CompletableDeferred<Unit>()
        coEvery { contactRepository.deleteContact(5) } coAnswers {
            gate.await()
            Result.success(Unit)
        }

        val vm = viewModel(5)
        advanceUntilIdle()

        vm.deleteContact()
        advanceUntilIdle() // isMutating flips true and the coroutine suspends on the gate
        assertTrue(vm.uiState.value.isMutating)

        vm.deleteContact() // a second call while the first is still in flight must be a no-op

        gate.complete(Unit)
        advanceUntilIdle()

        coVerify(exactly = 1) { contactRepository.deleteContact(5) }
        assertEquals(ContactDetailEvent.ContactDeleted, vm.events.value)
    }

    @Test
    fun `setArchived ignores a second call while the first is in flight`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        val gate = CompletableDeferred<Unit>()
        coEvery { contactRepository.archiveContact(5) } coAnswers {
            gate.await()
            Result.success(Unit)
        }

        val vm = viewModel(5)
        advanceUntilIdle()

        vm.setArchived(archived = true)
        advanceUntilIdle()
        assertTrue(vm.uiState.value.isMutating)

        vm.setArchived(archived = true)

        gate.complete(Unit)
        advanceUntilIdle()

        coVerify(exactly = 1) { contactRepository.archiveContact(5) }
    }

    @Test
    fun `uploadPhoto ignores a second call while the first is in flight`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        val bytes = ByteArray(8) { it.toByte() }
        val gate = CompletableDeferred<Unit>()
        coEvery { contactRepository.uploadPhoto(5, bytes, "image/jpeg") } coAnswers {
            gate.await()
            Result.success(Unit)
        }

        val vm = viewModel(5)
        advanceUntilIdle()

        vm.uploadPhoto(bytes, "image/jpeg")
        advanceUntilIdle()
        assertTrue(vm.uiState.value.isMutating)

        vm.uploadPhoto(bytes, "image/jpeg")

        gate.complete(Unit)
        advanceUntilIdle()

        coVerify(exactly = 1) { contactRepository.uploadPhoto(5, bytes, "image/jpeg") }
    }

    @Test
    fun `isMutating is shared across actions -- exportVcf is blocked while deleteContact is in flight`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
            coEvery { contactRepository.getContact(5) } returns Result.success(record)
            val gate = CompletableDeferred<Unit>()
            coEvery { contactRepository.deleteContact(5) } coAnswers {
                gate.await()
                Result.success(Unit)
            }

            val vm = viewModel(5)
            advanceUntilIdle()

            vm.deleteContact()
            advanceUntilIdle()
            assertTrue(vm.uiState.value.isMutating)

            // A completely different mutating action must also be blocked by the
            // same shared isMutating flag.
            vm.exportVcf()

            gate.complete(Unit)
            advanceUntilIdle()

            coVerify(exactly = 0) { contactRepository.exportContactVcf(any(), any()) }
        }

    // --- Issue #220: External Links panel + Immich ---

    @Test
    fun `external links and the immich summary load alongside the contact`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        val vm = viewModel(5)
        // Re-stub after the helper's defaults (mockk last-registered-wins).
        coEvery { externalIdentityRepository.listForContact("u5") } returns Result.success(
            listOf(ExternalIdentity(id = "i1", entityId = "u5", system = "paperless", externalId = "doc-1")),
        )
        coEvery { immichRepository.getContactSummary("u5") } returns Result.success(
            com.mycorrhizal.crm.model.network.ImmichPersonSummary(personName = "Alice", photoCount = 42),
        )
        advanceUntilIdle()

        val state = vm.uiState.value
        assertEquals(1, state.externalIdentities.size)
        assertEquals("paperless", state.externalIdentities.first().system)
        assertEquals("Alice", state.immichSummary?.personName)
        assertEquals(42, state.immichSummary?.photoCount)
    }

    @Test
    fun `a contact without a vcard uid never loads external links`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        val vm = viewModel(5)
        advanceUntilIdle()

        io.mockk.coVerify(exactly = 0) { externalIdentityRepository.listForContact(any()) }
        io.mockk.coVerify(exactly = 0) { immichRepository.getContactSummary(any()) }
    }

    @Test
    fun `immich configured flag reflects the user's connection`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        val vm = viewModel(5)
        coEvery { immichRepository.isConfigured() } returns Result.success(true)
        advanceUntilIdle()

        assertTrue(vm.uiState.value.immichConfigured)
    }

    @Test
    fun `deleteExternalIdentity reloads the panel on success`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { externalIdentityRepository.delete("i1") } returns Result.success(Unit)

        val vm = viewModel(5)
        advanceUntilIdle()

        vm.deleteExternalIdentity("i1")
        advanceUntilIdle()

        io.mockk.coVerify(exactly = 1) { externalIdentityRepository.delete("i1") }
        // The panel is refetched after the delete.
        io.mockk.coVerify(atLeast = 2) { externalIdentityRepository.listForContact("u5") }
        assertFalse(vm.uiState.value.isExternalLinkMutating)
        assertNull(vm.uiState.value.error)
    }

    @Test
    fun `deleteExternalIdentity failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { externalIdentityRepository.delete("i1") } returns Result.failure(ApiError.Client(404, "External identity not found"))

        val vm = viewModel(5)
        advanceUntilIdle()

        vm.deleteExternalIdentity("i1")
        advanceUntilIdle()

        assertEquals("Not found", vm.uiState.value.error)
        assertFalse(vm.uiState.value.isExternalLinkMutating)
    }

    @Test
    fun `unlinkImmichPerson clears the summary and reloads the panel`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { immichRepository.unlinkPerson("u5") } returns Result.success(Unit)

        val vm = viewModel(5)
        advanceUntilIdle()

        vm.unlinkImmichPerson()
        advanceUntilIdle()

        io.mockk.coVerify(exactly = 1) { immichRepository.unlinkPerson("u5") }
        assertNull(vm.uiState.value.immichSummary)
        io.mockk.coVerify(atLeast = 2) { externalIdentityRepository.listForContact("u5") }
    }

    @Test
    fun `loadImmichPeople populates the picker's person list`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        val vm = viewModel(5)
        advanceUntilIdle()

        coEvery { immichRepository.listPeople() } returns Result.success(
            listOf(ImmichPerson(id = "p1", name = "Alice"), ImmichPerson(id = "p2", name = "Bob")),
        )
        vm.loadImmichPeople()
        advanceUntilIdle()

        assertEquals(2, vm.uiState.value.immichPeople.size)
        assertEquals("Alice", vm.uiState.value.immichPeople.first().name)
        assertFalse(vm.uiState.value.immichPeopleLoading)
    }

    @Test
    fun `linkImmichPerson links and loads the person's assets`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        val vm = viewModel(5)
        advanceUntilIdle()

        coEvery { immichRepository.linkPerson("u5", any()) } returns Result.success(Unit)
        coEvery { immichRepository.listContactAssets("u5") } returns Result.success(
            listOf(com.mycorrhizal.crm.model.network.ImmichAssetSummary(id = "a1")),
        )
        vm.linkImmichPerson(ImmichPerson(id = "p1", name = "Alice"))
        advanceUntilIdle()

        io.mockk.coVerify(exactly = 1) { immichRepository.linkPerson("u5", any()) }
        assertEquals(1, vm.uiState.value.immichAssets.size)
        assertEquals("a1", vm.uiState.value.immichAssets.first().id)
    }

    @Test
    fun `pickImmichAsset hands the fetched bytes to the callback`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        val vm = viewModel(5)
        advanceUntilIdle()

        val bytes = ByteArray(128) { it.toByte() }
        coEvery { immichRepository.getAssetImageBytes("u5", "a1") } returns Result.success(bytes)
        var handed: ByteArray? = null
        vm.pickImmichAsset("a1") { handed = it }
        advanceUntilIdle()

        assertEquals(bytes.contentToString(), handed?.contentToString())
        assertNull(vm.uiState.value.error)
    }

    @Test
    fun `pickImmichAsset failure surfaces the error without a callback`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        val vm = viewModel(5)
        advanceUntilIdle()

        coEvery { immichRepository.getThumbnailBytes("u5") } returns Result.failure(ApiError.Server(503, "boom"))
        var handed = false
        vm.pickImmichAsset(null) { handed = true }
        advanceUntilIdle()

        assertFalse(handed)
        assertTrue(vm.uiState.value.error != null)
    }

    // --- isExternalLinkMutating / immichPeopleLoading double-submit guards
    // (coverage-analysis follow-up; same rationale as the isMutating tests
    // above). ---

    @Test
    fun `deleteExternalIdentity ignores a second call while the first is in flight`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
            coEvery { contactRepository.getContact(5) } returns Result.success(record)
            val gate = CompletableDeferred<Unit>()
            coEvery { externalIdentityRepository.delete("i1") } coAnswers {
                gate.await()
                Result.success(Unit)
            }

            val vm = viewModel(5)
            advanceUntilIdle()

            vm.deleteExternalIdentity("i1")
            advanceUntilIdle()
            assertTrue(vm.uiState.value.isExternalLinkMutating)

            vm.deleteExternalIdentity("i1")

            gate.complete(Unit)
            advanceUntilIdle()

            coVerify(exactly = 1) { externalIdentityRepository.delete("i1") }
        }

    @Test
    fun `isExternalLinkMutating is shared -- unlinkImmichPerson is blocked while deleteExternalIdentity is in flight`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
            coEvery { contactRepository.getContact(5) } returns Result.success(record)
            val gate = CompletableDeferred<Unit>()
            coEvery { externalIdentityRepository.delete("i1") } coAnswers {
                gate.await()
                Result.success(Unit)
            }

            val vm = viewModel(5)
            advanceUntilIdle()

            vm.deleteExternalIdentity("i1")
            advanceUntilIdle()
            assertTrue(vm.uiState.value.isExternalLinkMutating)

            vm.unlinkImmichPerson()

            gate.complete(Unit)
            advanceUntilIdle()

            coVerify(exactly = 0) { immichRepository.unlinkPerson(any()) }
        }

    @Test
    fun `loadImmichPeople ignores a second call while the first is in flight`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
            coEvery { contactRepository.getContact(5) } returns Result.success(record)
            val gate = CompletableDeferred<Unit>()
            coEvery { immichRepository.listPeople() } coAnswers {
                gate.await()
                Result.success(emptyList())
            }

            val vm = viewModel(5)
            advanceUntilIdle()

            vm.loadImmichPeople()
            advanceUntilIdle()
            assertTrue(vm.uiState.value.immichPeopleLoading)

            vm.loadImmichPeople()

            gate.complete(Unit)
            advanceUntilIdle()

            coVerify(exactly = 1) { immichRepository.listPeople() }
        }

    // --- M24: inline circle/tag editors ---

    @Test
    fun `circles and tags load into the editors alongside the contact`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        val vm = viewModel(5)
        // Re-stub after the helper's defaults (mockk last-registered-wins) and before the
        // membership-load coroutines are scheduled to run.
        coEvery { circleRepository.circlesForContact("u5") } returns Result.success(
            listOf(com.mycorrhizal.crm.model.network.Circle(id = "c1", name = "friends")),
        )
        coEvery { circleRepository.list(any(), any()) } returns Result.success(
            listOf(com.mycorrhizal.crm.model.network.Circle(id = "c1", name = "friends"), com.mycorrhizal.crm.model.network.Circle(id = "c2", name = "family")),
        )
        coEvery { tagRepository.tagsForContact("u5") } returns Result.success(
            listOf(com.mycorrhizal.crm.model.network.Tag(id = "t1", name = "close")),
        )
        coEvery { tagRepository.list(any(), any()) } returns Result.success(
            listOf(com.mycorrhizal.crm.model.network.Tag(id = "t1", name = "close"), com.mycorrhizal.crm.model.network.Tag(id = "t2", name = "work")),
        )
        advanceUntilIdle()

        val state = vm.uiState.value
        assertEquals(listOf("friends"), state.contactCircles.map { it.name })
        assertEquals(2, state.allCircles.size)
        assertEquals(listOf("close"), state.contactTags.map { it.name })
        assertEquals(2, state.allTags.size)
    }

    @Test
    fun `addCircle writes the membership and refreshes the derivations`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { circleRepository.addMember("c2", "u5") } returns Result.success(
            com.mycorrhizal.crm.model.network.CircleMember(circleId = "c2", memberVCardUid = "u5"),
        )

        val vm = viewModel(5)
        advanceUntilIdle()

        vm.addCircle(com.mycorrhizal.crm.model.network.Circle(id = "c2", name = "family"))
        advanceUntilIdle()

        io.mockk.coVerify(exactly = 1) { circleRepository.addMember("c2", "u5") }
        // The derivation is refetched after the write.
        io.mockk.coVerify(atLeast = 2) { circleRepository.circlesForContact("u5") }
    }

    @Test
    fun `removeTag writes the untagging and refreshes the derivations`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { tagRepository.removeContact("t1", "u5") } returns Result.success(Unit)

        val vm = viewModel(5)
        advanceUntilIdle()

        vm.removeTag(com.mycorrhizal.crm.model.network.Tag(id = "t1", name = "close"))
        advanceUntilIdle()

        io.mockk.coVerify(exactly = 1) { tagRepository.removeContact("t1", "u5") }
    }

    // --- M20: reminder completions on the timeline ---

    @Test
    fun `completions load alongside the contact`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        val vm = viewModel(5)
        // Re-stub after the helper's default (mockk last-registered-wins).
        coEvery { reminderRepository.listCompletions(5) } returns Result.success(
            listOf(
                com.mycorrhizal.crm.model.network.ReminderCompletion(
                    id = 1,
                    contactId = 5,
                    message = "Done with gift",
                    completedAt = "2026-08-12T10:00:00Z",
                ),
            ),
        )
        advanceUntilIdle()

        assertEquals(1, vm.uiState.value.completions.size)
        assertEquals("Done with gift", vm.uiState.value.completions[0].message)
    }

    @Test
    fun `a completions fetch failure does not fail the contact load`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { reminderRepository.listCompletions(5) } returns Result.failure(ApiError.Server(500, "boom"))

        val vm = viewModel(5)
        advanceUntilIdle()

        val state = vm.uiState.value
        assertEquals("Dana White", state.contact?.card?.name?.full)
        assertNull(state.error)
        assertTrue(state.completions.isEmpty())
    }

    @Test
    fun `undoCompletion deletes the completion and reloads the list`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { reminderRepository.deleteCompletion(7) } returns Result.success(Unit)
        coEvery { reminderRepository.listCompletions(5) } returns Result.success(emptyList())

        val vm = viewModel(5)
        advanceUntilIdle()

        vm.undoCompletion(7)
        advanceUntilIdle()

        io.mockk.coVerify(exactly = 1) { reminderRepository.deleteCompletion(7) }
        // M20 test case 4: after the undo the completion list is refetched, so a deleted
        // completion is gone from the timeline (its pre-completion state).
        io.mockk.coVerify(exactly = 2) { reminderRepository.listCompletions(5) }
        assertNull(vm.uiState.value.error)
    }

    @Test
    fun `undoCompletion failure surfaces the error and keeps the completion`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)
        coEvery { reminderRepository.deleteCompletion(7) } returns Result.failure(
            ApiError.Client(404, "Reminder completion not found"),
        )

        val vm = viewModel(5)
        advanceUntilIdle()

        vm.undoCompletion(7)
        advanceUntilIdle()

        assertEquals("Not found", vm.uiState.value.error)
    }

    // --- Issue #236: Paperless/Seafile/Nextcloud create/link pickers ---

    @Test
    fun `loadExternalLinks populates the three configured gates`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        // viewModel() applies its default (all-false) stubExternalLinks() stubs during
        // construction; override them afterward, then let init's launched coroutines run.
        val vm = viewModel(5)
        coEvery { paperlessRepository.isConfigured() } returns Result.success(true)
        coEvery { seafileRepository.isConfigured() } returns Result.success(true)
        coEvery { nextcloudRepository.isConfigured() } returns Result.success(false)
        advanceUntilIdle()

        assertTrue(vm.uiState.value.paperlessConfigured)
        assertTrue(vm.uiState.value.seafileConfigured)
        assertFalse(vm.uiState.value.nextcloudConfigured)
    }

    @Test
    fun `loadPaperlessDocuments populates the picker's document list`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        val vm = viewModel(5)
        advanceUntilIdle()

        coEvery { paperlessRepository.searchDocuments(null) } returns Result.success(
            listOf(PaperlessDocument(id = 1, title = "Lease")),
        )
        vm.loadPaperlessDocuments()
        advanceUntilIdle()

        assertEquals(1, vm.uiState.value.paperlessDocuments.size)
        assertEquals("Lease", vm.uiState.value.paperlessDocuments.first().title)
        assertFalse(vm.uiState.value.paperlessDocumentsLoading)
    }

    @Test
    fun `linkPaperlessDocument links then reloads the panel`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        val vm = viewModel(5)
        advanceUntilIdle()

        coEvery { paperlessRepository.linkDocument("u5", "1") } returns Result.success(Unit)
        vm.linkPaperlessDocument(PaperlessDocument(id = 1, title = "Lease"))
        advanceUntilIdle()

        io.mockk.coVerify(exactly = 1) { paperlessRepository.linkDocument("u5", "1") }
        io.mockk.coVerify(atLeast = 2) { externalIdentityRepository.listForContact("u5") }
    }

    @Test
    fun `linkPaperlessDocument failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        val vm = viewModel(5)
        advanceUntilIdle()

        coEvery { paperlessRepository.linkDocument("u5", "1") } returns Result.failure(
            ApiError.Client(409, "Already linked"),
        )
        vm.linkPaperlessDocument(PaperlessDocument(id = 1, title = "Lease"))
        advanceUntilIdle()

        assertEquals("Already linked", vm.uiState.value.error)
    }

    @Test
    fun `loadSeafileLibraries populates the picker's top level`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        val vm = viewModel(5)
        advanceUntilIdle()

        coEvery { seafileRepository.listLibraries() } returns Result.success(
            listOf(SeafileLibrary(id = "r1", name = "Docs")),
        )
        vm.loadSeafileLibraries()
        advanceUntilIdle()

        assertEquals(1, vm.uiState.value.seafileLibraries.size)
        assertNull(vm.uiState.value.seafileBrowseRepoId)
    }

    @Test
    fun `loadSeafileLibraries failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        val vm = viewModel(5)
        advanceUntilIdle()

        coEvery { seafileRepository.listLibraries() } returns Result.failure(ApiError.Server(500, "boom"))
        vm.loadSeafileLibraries()
        advanceUntilIdle()

        assertEquals("Server error (500)", vm.uiState.value.error)
        assertFalse(vm.uiState.value.seafileBrowseLoading)
    }

    @Test
    fun `enterSeafileDir failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        val vm = viewModel(5)
        advanceUntilIdle()

        coEvery { seafileRepository.listDir("r1", "/") } returns
            Result.failure(ApiError.Client(404, "Directory not found"))
        vm.enterSeafileDir("r1")
        advanceUntilIdle()

        assertEquals("Not found", vm.uiState.value.error)
        assertFalse(vm.uiState.value.seafileBrowseLoading)
        // The failed directory is still recorded (matches the loading-flag reset
        // behaviour of the other browse failures) rather than silently reverting.
        assertEquals("r1", vm.uiState.value.seafileBrowseRepoId)
    }

    @Test
    fun `enterSeafileDir then backSeafileDir returns to the library list`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        val vm = viewModel(5)
        advanceUntilIdle()

        coEvery { seafileRepository.listLibraries() } returns Result.success(
            listOf(SeafileLibrary(id = "r1", name = "Docs")),
        )
        coEvery { seafileRepository.listDir("r1", "/") } returns Result.success(
            listOf(SeafileItem(id = "f1", name = "doc.pdf", type = "file")),
        )
        vm.enterSeafileDir("r1")
        advanceUntilIdle()

        assertEquals("r1", vm.uiState.value.seafileBrowseRepoId)
        assertEquals(1, vm.uiState.value.seafileBrowseItems.size)

        vm.backSeafileDir()
        advanceUntilIdle()

        assertNull(vm.uiState.value.seafileBrowseRepoId)
        io.mockk.coVerify(exactly = 1) { seafileRepository.listLibraries() }
    }

    @Test
    fun `linkSeafileItem joins the browse path and reloads the panel`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        val vm = viewModel(5)
        advanceUntilIdle()

        coEvery { seafileRepository.listDir("r1", "/") } returns Result.success(emptyList())
        vm.enterSeafileDir("r1")
        advanceUntilIdle()

        val expectedRequest = SeafileLinkRequest(repoId = "r1", path = "/doc.pdf", name = "doc.pdf", type = "file", size = 1024)
        coEvery { seafileRepository.linkItem("u5", expectedRequest) } returns Result.success(Unit)
        vm.linkSeafileItem(SeafileItem(id = "f1", name = "doc.pdf", type = "file", size = 1024))
        advanceUntilIdle()

        io.mockk.coVerify(exactly = 1) { seafileRepository.linkItem("u5", expectedRequest) }
        io.mockk.coVerify(atLeast = 2) { externalIdentityRepository.listForContact("u5") }
    }

    @Test
    fun `linkSeafileItem failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        val vm = viewModel(5)
        advanceUntilIdle()

        coEvery { seafileRepository.listDir("r1", "/") } returns Result.success(emptyList())
        vm.enterSeafileDir("r1")
        advanceUntilIdle()

        val expectedRequest = SeafileLinkRequest(repoId = "r1", path = "/doc.pdf", name = "doc.pdf", type = "file", size = 1024)
        coEvery { seafileRepository.linkItem("u5", expectedRequest) } returns
            Result.failure(ApiError.Client(409, "Already linked"))
        vm.linkSeafileItem(SeafileItem(id = "f1", name = "doc.pdf", type = "file", size = 1024))
        advanceUntilIdle()

        assertEquals("Already linked", vm.uiState.value.error)
    }

    @Test
    fun `loadNextcloudDir populates the browse list at the given path`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        val vm = viewModel(5)
        advanceUntilIdle()

        coEvery { nextcloudRepository.listDir("/Photos") } returns Result.success(
            listOf(WebDAVItem(name = "img.jpg", path = "/Photos/img.jpg", type = "file")),
        )
        vm.loadNextcloudDir("/Photos")
        advanceUntilIdle()

        assertEquals("/Photos", vm.uiState.value.nextcloudBrowsePath)
        assertEquals(1, vm.uiState.value.nextcloudBrowseItems.size)
        assertFalse(vm.uiState.value.nextcloudBrowseLoading)
    }

    @Test
    fun `loadNextcloudDir failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        val vm = viewModel(5)
        advanceUntilIdle()

        coEvery { nextcloudRepository.listDir("/Photos") } returns Result.failure(ApiError.Server(500, "boom"))
        vm.loadNextcloudDir("/Photos")
        advanceUntilIdle()

        assertEquals("Server error (500)", vm.uiState.value.error)
        assertFalse(vm.uiState.value.nextcloudBrowseLoading)
    }

    @Test
    fun `linkNextcloudItem links then reloads the panel`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        val vm = viewModel(5)
        advanceUntilIdle()

        val item = WebDAVItem(name = "img.jpg", path = "/Photos/img.jpg", type = "file", size = 2048)
        coEvery { nextcloudRepository.linkItem("u5", item) } returns Result.success(Unit)
        vm.linkNextcloudItem(item)
        advanceUntilIdle()

        io.mockk.coVerify(exactly = 1) { nextcloudRepository.linkItem("u5", item) }
        io.mockk.coVerify(atLeast = 2) { externalIdentityRepository.listForContact("u5") }
    }

    @Test
    fun `linkNextcloudItem failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        val vm = viewModel(5)
        advanceUntilIdle()

        val item = WebDAVItem(name = "img.jpg", path = "/Photos/img.jpg", type = "file", size = 2048)
        coEvery { nextcloudRepository.linkItem("u5", item) } returns
            Result.failure(ApiError.Client(409, "Already linked"))
        vm.linkNextcloudItem(item)
        advanceUntilIdle()

        assertEquals("Already linked", vm.uiState.value.error)
    }

    // --- Issue #830: saveFieldValue (per-contact custom-field value editing) ---

    @Test
    fun `saveFieldValue upserts the definition into the existing set and sends the full replace`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val record = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
            coEvery { contactRepository.getContact(5) } returns Result.success(record)

            val vm = viewModel(5)
            // Overrides the helper's default any()-matched empty-list stub — must be set after
            // viewModel(5) so this more specific stub wins for the id-5 call load() is about to make.
            coEvery { fieldDefinitionRepository.contactValues(5) } returns Result.success(
                listOf(FieldValue(id = 1, fieldDefinitionId = "d1", value = "Latte")),
            )
            advanceUntilIdle()
            assertEquals("Latte", vm.uiState.value.fieldValuesByDefinitionId["d1"])

            val slot = slot<ContactFieldValuesInput>()
            coEvery { fieldDefinitionRepository.replaceContactValues(5, capture(slot)) } returns Result.success(
                listOf(
                    FieldValue(id = 1, fieldDefinitionId = "d1", value = "Latte"),
                    FieldValue(id = 2, fieldDefinitionId = "d2", value = "M"),
                ),
            )

            vm.saveFieldValue("d2", "M")
            advanceUntilIdle()

            val sent = slot.captured.fieldValues.associate { it.fieldDefinitionId to it.value }
            assertEquals(mapOf("d1" to "Latte", "d2" to "M"), sent)
            assertEquals("M", vm.uiState.value.fieldValuesByDefinitionId["d2"])
            assertNull(vm.uiState.value.savingFieldDefinitionId)
        }

    @Test
    fun `saveFieldValue with a null value omits the definition from the payload instead of sending null`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val record = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
            coEvery { contactRepository.getContact(5) } returns Result.success(record)
            coEvery { fieldDefinitionRepository.contactValues(5) } returns Result.success(
                listOf(FieldValue(id = 1, fieldDefinitionId = "d1", value = "Latte")),
            )

            val vm = viewModel(5)
            advanceUntilIdle()

            val slot = slot<ContactFieldValuesInput>()
            coEvery { fieldDefinitionRepository.replaceContactValues(5, capture(slot)) } returns Result.success(emptyList())

            vm.saveFieldValue("d1", null)
            advanceUntilIdle()

            assertTrue(slot.captured.fieldValues.none { it.fieldDefinitionId == "d1" })
            assertTrue(vm.uiState.value.fieldValuesByDefinitionId.isEmpty())
        }

    @Test
    fun `saveFieldValue ignores a second call while the first is in flight`() = runTest(mainDispatcherRule.testDispatcher) {
        val record = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        coEvery { contactRepository.getContact(5) } returns Result.success(record)

        val vm = viewModel(5)
        advanceUntilIdle()

        val gate = CompletableDeferred<Unit>()
        coEvery { fieldDefinitionRepository.replaceContactValues(5, any()) } coAnswers {
            gate.await()
            Result.success(listOf(FieldValue(id = 1, fieldDefinitionId = "d1", value = "Latte")))
        }

        vm.saveFieldValue("d1", "Latte")
        advanceUntilIdle() // savingFieldDefinitionId flips to "d1" and the coroutine suspends on the gate
        assertEquals("d1", vm.uiState.value.savingFieldDefinitionId)

        vm.saveFieldValue("d2", "M") // a second call while the first is still in flight must be a no-op

        gate.complete(Unit)
        advanceUntilIdle()

        coVerify(exactly = 1) { fieldDefinitionRepository.replaceContactValues(5, any()) }
    }

    @Test
    fun `saveFieldValue failure surfaces the error and clears savingFieldDefinitionId without mutating values`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val record = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
            coEvery { contactRepository.getContact(5) } returns Result.success(record)

            val vm = viewModel(5)
            coEvery { fieldDefinitionRepository.contactValues(5) } returns Result.success(
                listOf(FieldValue(id = 1, fieldDefinitionId = "d1", value = "Latte")),
            )
            advanceUntilIdle()

            coEvery { fieldDefinitionRepository.replaceContactValues(5, any()) } returns
                Result.failure(ApiError.Client(500, "boom"))

            vm.saveFieldValue("d1", "Espresso")
            advanceUntilIdle()

            assertEquals("boom", vm.uiState.value.error)
            assertNull(vm.uiState.value.savingFieldDefinitionId)
            assertEquals("Latte", vm.uiState.value.fieldValuesByDefinitionId["d1"])
        }
}
