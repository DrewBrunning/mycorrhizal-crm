package com.mycorrhizal.crm.feature.timeline

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.NoteRepository
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.model.network.NoteInput
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
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

class NotesViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val noteRepository = mockk<NoteRepository>()

    private fun viewModel(contactId: Int = 5): NotesViewModel =
        NotesViewModel(noteRepository, SavedStateHandle(mapOf("contactId" to contactId)))

    @Test
    fun `loads the contact's notes on init`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { noteRepository.listForContact(5) } returns Result.success(
            listOf(Note(id = 3, content = "Loves climbing"), Note(id = 4, content = "Met at conference")),
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

        assertEquals("Missing contact id", vm.uiState.value.error)
    }

    @Test
    fun `failure surfaces the display message`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { noteRepository.listForContact(5) } returns Result.failure(
            ApiError.Client(404, "Contact not found"),
        )

        val vm = viewModel()
        advanceUntilIdle()

        assertEquals("Not found", vm.uiState.value.error)
        assertTrue(vm.uiState.value.notes.isEmpty())
    }
}

class NoteFormViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val noteRepository = mockk<NoteRepository>()

    private fun createViewModel(contactId: Int = 5, noteId: Int? = null): NoteFormViewModel {
        val handle = SavedStateHandle(mapOf("contactId" to contactId).toMutableMap().apply {
            if (noteId != null) put("noteId", noteId)
        })
        return NoteFormViewModel(noteRepository, handle)
    }

    @Test
    fun `create mode starts empty`() {
        val vm = createViewModel()
        val state = vm.uiState.value
        assertFalse(state.isEdit)
        assertEquals(5, state.contactId)
        assertNull(state.noteId)
    }

    @Test
    fun `save without content is blocked`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = createViewModel()
        vm.onDateChange("2026-08-10T14:00:00Z")
        vm.save()
        advanceUntilIdle()

        assertEquals("Note content is required", vm.uiState.value.error)
        coVerify(exactly = 0) { noteRepository.create(any(), any()) }
    }

    @Test
    fun `save without a date is blocked`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = createViewModel()
        vm.onContentChange("Loves climbing")
        vm.save()
        advanceUntilIdle()

        assertEquals("Date is required", vm.uiState.value.error)
        coVerify(exactly = 0) { noteRepository.create(any(), any()) }
    }

    @Test
    fun `save in create mode calls create with the contact id`() = runTest(mainDispatcherRule.testDispatcher) {
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

        assertEquals("Date must be ISO 8601, e.g. 2026-08-10T14:00:00Z", vm.uiState.value.error)
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
}
