package com.mycorrhizal.crm.feature.timeline

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.ContactNotesPage
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.ContactsPage
import com.mycorrhizal.crm.domain.repository.NoteRepository
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactFlat
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.model.network.NoteInput
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import com.mycorrhizal.crm.ui.R
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test

class NotesViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val noteRepository = mockk<NoteRepository>()

    private fun viewModel(contactId: Int = 5): NotesViewModel =
        NotesViewModel(noteRepository, SavedStateHandle(mapOf("contactId" to contactId)))

    @Test
    fun `loads the contact's notes on init`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { noteRepository.listForContact(5, null, null, null, null, null) } returns Result.success(
            ContactNotesPage(
                notes = listOf(Note(id = 3, content = "Loves climbing"), Note(id = 4, content = "Met at conference")),
                nextCursor = null,
            ),
        )

        val vm = viewModel()
        advanceUntilIdle()

        val state = vm.uiState.value
        assertFalse(state.isLoading)
        assertEquals(2, state.notes.size)
        assertEquals("Loves climbing", state.notes[0].content)
        assertNull(state.error)
    }

    @Test
    fun `missing contact id sets an error`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = viewModel(contactId = 0)
        advanceUntilIdle()

        assertEquals(R.string.note_error_missing_id, vm.uiState.value.errorRes)
    }

    @Test
    fun `failure surfaces the display message`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { noteRepository.listForContact(5, null, null, null, null, null) } returns Result.failure(
            ApiError.Client(404, "Contact not found"),
        )

        val vm = viewModel()
        advanceUntilIdle()

        assertEquals("Not found", vm.uiState.value.error)
        assertTrue(vm.uiState.value.notes.isEmpty())
    }

    @Test
    fun `search change debounces and reloads with the query`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { noteRepository.listForContact(5, null, null, null, null, null) } returns Result.success(
            ContactNotesPage(notes = listOf(Note(id = 3, content = "Loves climbing")), nextCursor = null),
        )
        coEvery { noteRepository.listForContact(5, null, null, "climb", null, null) } returns Result.success(
            ContactNotesPage(notes = listOf(Note(id = 3, content = "Loves climbing")), nextCursor = null),
        )

        val vm = viewModel()
        advanceUntilIdle()
        vm.onSearchChange("climb")
        advanceUntilIdle()

        assertEquals("climb", vm.uiState.value.searchQuery)
        coVerify { noteRepository.listForContact(5, null, null, "climb", null, null) }
    }

    @Test
    fun `date filters reload with the bounds`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { noteRepository.listForContact(any(), any(), any(), any(), any(), any()) } returns Result.success(
            ContactNotesPage(notes = emptyList(), nextCursor = null),
        )

        val vm = viewModel()
        advanceUntilIdle()
        vm.onFromDateChange("2026-08-01")
        vm.onToDateChange("2026-08-10")
        advanceUntilIdle()

        coVerify { noteRepository.listForContact(5, null, null, null, "2026-08-01", "2026-08-10") }
        assertNull(vm.uiState.value.error)
    }

    @Test
    fun `load more appends the next page without duplicating`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { noteRepository.listForContact(5, null, null, null, null, null) } returns Result.success(
            ContactNotesPage(
                notes = listOf(Note(id = 3, content = "One"), Note(id = 4, content = "Two")),
                nextCursor = "cursor-1",
            ),
        )
        coEvery { noteRepository.listForContact(5, "cursor-1", null, null, null, null) } returns Result.success(
            ContactNotesPage(notes = listOf(Note(id = 5, content = "Three")), nextCursor = null),
        )

        val vm = viewModel()
        advanceUntilIdle()
        assertEquals(2, vm.uiState.value.notes.size)

        vm.loadMore()
        advanceUntilIdle()

        val state = vm.uiState.value
        assertEquals(3, state.notes.size)
        assertEquals(listOf(3, 4, 5), state.notes.map { it.id })
        assertNull(state.nextCursor)
        assertFalse(state.isLoadingMore)
    }

    @Test
    fun `a superseding load cancels the in-flight request so stale results never win`() = runTest(mainDispatcherRule.testDispatcher) {
        val staleGate = CompletableDeferred<Unit>()
        coEvery { noteRepository.listForContact(5, null, null, null, null, null) } returns Result.success(
            ContactNotesPage(notes = listOf(Note(id = 1, content = "all")), nextCursor = null),
        )
        coEvery { noteRepository.listForContact(5, null, null, "a", null, null) } coAnswers {
            staleGate.await()
            Result.success(ContactNotesPage(notes = listOf(Note(id = 1, content = "stale")), nextCursor = null))
        }
        coEvery { noteRepository.listForContact(5, null, null, "ab", null, null) } returns Result.success(
            ContactNotesPage(notes = listOf(Note(id = 2, content = "fresh")), nextCursor = null),
        )

        val vm = viewModel()
        advanceUntilIdle() // initial load
        vm.onSearchChange("a")
        advanceUntilIdle() // debounce fires → load("a") starts and suspends on the gate
        vm.onSearchChange("ab")
        advanceUntilIdle() // debounce fires → load("ab") supersedes
        staleGate.complete(Unit) // the stale request would land here if it weren't cancelled
        advanceUntilIdle()

        // The stale "a" result must NOT overwrite the fresh "ab" result.
        assertEquals(listOf(2), vm.uiState.value.notes.map { it.id })
        assertEquals("fresh", vm.uiState.value.notes.single().content)
    }

    @Test
    fun `load more is ignored while the list is reloading`() = runTest(mainDispatcherRule.testDispatcher) {
        val reloadGate = CompletableDeferred<Unit>()
        coEvery { noteRepository.listForContact(5, null, null, null, null, null) } returns Result.success(
            ContactNotesPage(notes = listOf(Note(id = 1, content = "One")), nextCursor = "cursor-1"),
        )
        coEvery { noteRepository.listForContact(5, null, null, "x", null, null) } coAnswers {
            reloadGate.await()
            Result.success(ContactNotesPage(notes = emptyList(), nextCursor = null))
        }

        val vm = viewModel()
        advanceUntilIdle()
        vm.onSearchChange("x")
        advanceUntilIdle() // reload in flight, gated
        assertTrue(vm.uiState.value.isLoading)

        vm.loadMore() // must be a no-op while isLoading

        coVerify(exactly = 0) { noteRepository.listForContact(5, "cursor-1", null, "x", null, null) }
        reloadGate.complete(Unit)
        advanceUntilIdle()
    }

    @Test
    fun `delete removes the item on success`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { noteRepository.listForContact(5, null, null, null, null, null) } returns Result.success(
            ContactNotesPage(notes = listOf(Note(id = 3, content = "One"), Note(id = 4, content = "Two")), nextCursor = null),
        )
        coEvery { noteRepository.delete(3) } returns Result.success(Unit)

        val vm = viewModel()
        advanceUntilIdle()
        vm.delete(3)
        advanceUntilIdle()

        assertEquals(listOf(4), vm.uiState.value.notes.map { it.id })
        assertNull(vm.uiState.value.deletingId)
    }

    @Test
    fun `delete failure keeps the item`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { noteRepository.listForContact(5, null, null, null, null, null) } returns Result.success(
            ContactNotesPage(notes = listOf(Note(id = 3, content = "One")), nextCursor = null),
        )
        coEvery { noteRepository.delete(3) } returns Result.failure(ApiError.Client(500, "boom"))

        val vm = viewModel()
        advanceUntilIdle()
        vm.delete(3)
        advanceUntilIdle()

        assertEquals(1, vm.uiState.value.notes.size)
        assertEquals("boom", vm.uiState.value.error)
        assertNull(vm.uiState.value.deletingId)
    }
}

class NoteFormViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val noteRepository = mockk<NoteRepository>()
    private val contactRepository = mockk<ContactRepository>()

    @Before
    fun setUp() {
        // Create mode resolves the route contact's name (best-effort); the
        // failure fallback is enough for tests that don't assert the label.
        coEvery { contactRepository.getContact(any()) } returns Result.failure(RuntimeException("offline"))
    }

    private fun createViewModel(contactId: Int = 5, noteId: Int? = null): NoteFormViewModel {
        val handle = SavedStateHandle(mapOf("contactId" to contactId).toMutableMap().apply {
            if (noteId != null) put("noteId", noteId)
        })
        return NoteFormViewModel(noteRepository, contactRepository, handle)
    }

    @Test
    fun `create mode starts empty`() {
        val vm = createViewModel()
        val state = vm.uiState.value
        assertFalse(state.isEdit)
        assertEquals(5, state.contactId)
        assertNull(state.noteId)
        // M19: the route contact is the default assignment.
        assertEquals(5, state.targetContactId)
    }

    @Test
    fun `save without content is blocked`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = createViewModel()
        vm.onDateChange("2026-08-10T14:00:00Z")
        vm.save()
        advanceUntilIdle()

        assertEquals(R.string.note_error_content, vm.uiState.value.errorRes)
        coVerify(exactly = 0) { noteRepository.create(any(), any()) }
    }

    @Test
    fun `save without a date is blocked`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = createViewModel()
        vm.onContentChange("Loves climbing")
        vm.save()
        advanceUntilIdle()

        assertEquals(R.string.note_error_date_required, vm.uiState.value.errorRes)
        coVerify(exactly = 0) { noteRepository.create(any(), any()) }
    }

    @Test
    fun `save in create mode calls create with the target contact id`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { noteRepository.create(5, any()) } returns Result.success(Note(id = 3, content = "Loves climbing"))

        val vm = createViewModel()
        vm.onContentChange("Loves climbing")
        vm.onDateChange("2026-08-10T14:00:00Z")
        vm.save()
        advanceUntilIdle()

        coVerify {
            noteRepository.create(
                5,
                match<NoteInput> { input ->
                    input.content == "Loves climbing" &&
                        input.date == "2026-08-10T14:00:00Z" &&
                        input.contactId == 5
                },
            )
        }
        assertEquals(NoteFormEvent.Saved, vm.events.value)
    }

    @Test
    fun `save in edit mode hydrates and calls update`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { noteRepository.get(3) } returns Result.success(
            Note(id = 3, content = "Loves climbing", date = "2026-08-10T14:00:00Z"),
        )
        coEvery { noteRepository.update(3, any()) } returns Result.success(Note(id = 3, content = "Loves climbing"))

        val vm = createViewModel(noteId = 3)
        advanceUntilIdle()

        assertEquals("Loves climbing", vm.uiState.value.content)

        vm.onContentChange("Loves rock climbing")
        vm.save()
        advanceUntilIdle()

        coVerify {
            noteRepository.update(
                3,
                match<NoteInput> { input -> input.content == "Loves rock climbing" },
            )
        }
        assertEquals(NoteFormEvent.Saved, vm.events.value)
    }

    @Test
    fun `invalid date format blocks save`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = createViewModel()
        vm.onContentChange("Loves climbing")
        vm.onDateChange("2026-08-10")
        vm.save()
        advanceUntilIdle()

        assertEquals(R.string.note_error_date, vm.uiState.value.errorRes)
        coVerify(exactly = 0) { noteRepository.create(any(), any()) }
    }

    @Test
    fun `failed save surfaces the error and stays`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { noteRepository.create(any(), any()) } returns Result.failure(
            ApiError.Client(409, "duplicate"),
        )

        val vm = createViewModel()
        vm.onContentChange("Loves climbing")
        vm.onDateChange("2026-08-10T14:00:00Z")
        vm.save()
        advanceUntilIdle()

        assertEquals("duplicate", vm.uiState.value.error)
        assertFalse(vm.uiState.value.isSaving)
        assertNull(vm.events.value)
    }

    // M19: contact reassignment.
    @Test
    fun `selecting a contact reassigns the note and save targets it`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { noteRepository.create(9, any()) } returns Result.success(Note(id = 3, content = "Loves climbing"))

        val vm = createViewModel()
        vm.selectContact(ContactSummary(id = 9, firstname = "Dana", lastname = "Lee"))
        assertEquals(9, vm.uiState.value.targetContactId)
        assertEquals("Dana Lee", vm.uiState.value.targetContactName)

        vm.onContentChange("Loves climbing")
        vm.onDateChange("2026-08-10T14:00:00Z")
        vm.save()
        advanceUntilIdle()

        coVerify {
            noteRepository.create(
                9,
                match<NoteInput> { input -> input.contactId == 9 },
            )
        }
    }

    @Test
    fun `clearing the contact creates an unassigned note`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { noteRepository.createUnassigned(any()) } returns Result.success(Note(id = 3, content = "Loves climbing"))

        val vm = createViewModel()
        vm.clearContact()
        assertNull(vm.uiState.value.targetContactId)

        vm.onContentChange("Loves climbing")
        vm.onDateChange("2026-08-10T14:00:00Z")
        vm.save()
        advanceUntilIdle()

        coVerify {
            noteRepository.createUnassigned(
                match<NoteInput> { input -> input.contactId == null },
            )
        }
        assertEquals(NoteFormEvent.Saved, vm.events.value)
    }

    @Test
    fun `edit hydrates the loaded note's contact for reassignment`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { noteRepository.get(3) } returns Result.success(
            Note(
                id = 3,
                content = "Loves climbing",
                date = "2026-08-10T14:00:00Z",
                contactId = 9,
            ),
        )
        coEvery { contactRepository.getContact(9) } returns Result.success(
            ContactRecordResponse(id = 9, card = Card(name = Name(full = "Dana Lee"))),
        )

        val vm = createViewModel(noteId = 3)
        advanceUntilIdle()

        assertEquals(9, vm.uiState.value.targetContactId)
        assertEquals("Dana Lee", vm.uiState.value.targetContactName)
    }

    // Regression: the note API never populates the nested `contact` (GetNote
    // has no Preload — it serializes a zero-valued struct), so reading the
    // assigned contact's name off the note gives "#0". The name must come from
    // the repository instead; the flat contact_id is the only reliable field.
    @Test
    fun `edit contact name is resolved via the repository, never the empty nested contact`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { noteRepository.get(3) } returns Result.success(
            Note(
                id = 3,
                content = "Loves climbing",
                date = "2026-08-10T14:00:00Z",
                contactId = 9,
                // The real API sends a zero-valued contact here (id 0, blank names).
                contact = ContactFlat(id = 0, firstname = null, lastname = null),
            ),
        )
        coEvery { contactRepository.getContact(9) } returns Result.success(
            ContactRecordResponse(id = 9, card = Card(name = Name(full = "Dana Lee"))),
        )

        val vm = createViewModel(noteId = 3)
        advanceUntilIdle()

        assertEquals(9, vm.uiState.value.targetContactId)
        assertEquals("Dana Lee", vm.uiState.value.targetContactName)
    }

    @Test
    fun `edit clearing the contact sends a null contact_id to unfile the note`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { noteRepository.get(3) } returns Result.success(
            Note(
                id = 3,
                content = "Loves climbing",
                date = "2026-08-10T14:00:00Z",
                contactId = 9,
                contact = ContactFlat(id = 9, firstname = "Dana", lastname = "Lee"),
            ),
        )
        coEvery { noteRepository.update(3, any()) } returns Result.success(Note(id = 3, content = "Loves climbing"))

        val vm = createViewModel(noteId = 3)
        advanceUntilIdle()
        vm.clearContact()
        assertNull(vm.uiState.value.targetContactId)

        vm.save()
        advanceUntilIdle()

        coVerify {
            noteRepository.update(
                3,
                match<NoteInput> { input -> input.contactId == null },
            )
        }
        assertEquals(NoteFormEvent.Saved, vm.events.value)
    }

    @Test
    fun `edit reassigning to another contact sends the new contact_id`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { noteRepository.get(3) } returns Result.success(
            Note(
                id = 3,
                content = "Loves climbing",
                date = "2026-08-10T14:00:00Z",
                contactId = 9,
                contact = ContactFlat(id = 9, firstname = "Dana", lastname = "Lee"),
            ),
        )
        coEvery { noteRepository.update(3, any()) } returns Result.success(Note(id = 3, content = "Loves climbing"))

        val vm = createViewModel(noteId = 3)
        advanceUntilIdle()
        vm.selectContact(ContactSummary(id = 12, firstname = "Bob", lastname = "Smith"))

        vm.save()
        advanceUntilIdle()

        coVerify {
            noteRepository.update(
                3,
                match<NoteInput> { input -> input.contactId == 12 },
            )
        }
    }

    @Test
    fun `contact search debounces and only the latest query reaches the repository`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { contactRepository.listContacts(search = "al", limit = 25) } returns Result.success(
            ContactsPage(contacts = listOf(ContactSummary(id = 9, firstname = "Dana", lastname = "Lee")), nextCursor = null, limit = 25, sync = null),
        )

        val vm = createViewModel()
        vm.searchContacts("a")
        vm.searchContacts("al") // supersedes the "a" search before its debounce fires
        advanceUntilIdle()

        coVerify(exactly = 1) { contactRepository.listContacts(search = "al", limit = 25) }
        coVerify(exactly = 0) { contactRepository.listContacts(search = "a", limit = 25) }
        assertEquals(1, vm.uiState.value.contactSearchResults.size)
    }
}
