package com.mycorrhizal.crm.feature.timeline

import com.mycorrhizal.crm.domain.repository.NoteRepository
import com.mycorrhizal.crm.domain.repository.UnfiledNotesPage
import com.mycorrhizal.crm.model.network.Note
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

// M9 item 1: the Notes drawer inbox — GET /notes (contact_id IS NULL only), distinct from
// NotesViewModel's per-contact list.
class NotesInboxViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val noteRepository = mockk<NoteRepository>()

    @Test
    fun `loads the unfiled inbox on init`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { noteRepository.listUnfiled() } returns Result.success(
            UnfiledNotesPage(
                notes = listOf(Note(id = 3, content = "Buy milk"), Note(id = 4, content = "Call mom")),
                nextCursor = null,
                total = 2,
            ),
        )

        val vm = NotesInboxViewModel(noteRepository)
        advanceUntilIdle()

        val state = vm.uiState.value
        assertFalse(state.isLoading)
        assertEquals(2, state.notes.size)
        assertEquals(2, state.total)
        assertNull(state.error)
    }

    @Test
    fun `failure surfaces the display message`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { noteRepository.listUnfiled() } returns Result.failure(ApiError.Client(500, "server error"))

        val vm = NotesInboxViewModel(noteRepository)
        advanceUntilIdle()

        assertEquals("server error", vm.uiState.value.error)
        assertTrue(vm.uiState.value.notes.isEmpty())
    }

    // Ticket's pagination trap: a test that only asserts "more items appeared" would pass
    // against a re-fetch of page 1 — assert both pages are present with no duplicates.
    @Test
    fun `loadMore appends the next page without duplicating`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { noteRepository.listUnfiled() } returns Result.success(
            UnfiledNotesPage(notes = listOf(Note(id = 1, content = "One")), nextCursor = "cursor-2", total = 3),
        )
        coEvery { noteRepository.listUnfiled(cursor = "cursor-2") } returns Result.success(
            UnfiledNotesPage(notes = listOf(Note(id = 2, content = "Two")), nextCursor = null, total = 3),
        )

        val vm = NotesInboxViewModel(noteRepository)
        advanceUntilIdle()
        vm.loadMore()
        advanceUntilIdle()

        val ids = vm.uiState.value.notes.map { it.id }
        assertEquals(listOf(1, 2), ids)
        assertNull(vm.uiState.value.nextCursor)
        coVerify(exactly = 1) { noteRepository.listUnfiled(cursor = "cursor-2") }
    }

    @Test
    fun `loadMore is a no-op with no next cursor`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { noteRepository.listUnfiled() } returns Result.success(
            UnfiledNotesPage(notes = listOf(Note(id = 1, content = "One")), nextCursor = null, total = 1),
        )

        val vm = NotesInboxViewModel(noteRepository)
        advanceUntilIdle()
        vm.loadMore()
        advanceUntilIdle()

        // Only the initial load() call — loadMore() short-circuited on a null cursor.
        coVerify(exactly = 1) { noteRepository.listUnfiled(cursor = null, limit = null) }
    }
}
