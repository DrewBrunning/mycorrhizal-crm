package com.mycorrhizal.crm.feature.timeline

import com.mycorrhizal.crm.domain.repository.NoteRepository
import com.mycorrhizal.crm.domain.repository.NotesScope
import com.mycorrhizal.crm.domain.repository.UnfiledNotesPage
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.model.network.NoteImage
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
        coEvery { noteRepository.listNotes(NotesScope.ALL) } returns Result.success(
            UnfiledNotesPage(
                notes = listOf(
                    Note(id = 3, content = "Buy milk"),
                    Note(id = 4, content = "Call mom")
                ),
                nextCursor = null,
                total = 2
            )
        )

        val vm = NotesInboxViewModel(noteRepository)
        advanceUntilIdle()

        val state = vm.uiState.value
        assertFalse(state.isLoading)
        assertEquals(2, state.notes.size)
        assertEquals(2, state.total)
        assertNull(state.error)
    }

    // Issue #1757: returning from the new-note form must show the note just created.
    @Test
    fun `onResumed reloads once a load has finished but not while the first one is running`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { noteRepository.listNotes(NotesScope.ALL) } returns Result.success(
                UnfiledNotesPage(
                    notes = listOf(Note(id = 1, content = "One")),
                    nextCursor = null,
                    total = 1
                )
            )

            val vm = NotesInboxViewModel(noteRepository)
            advanceUntilIdle()
            coVerify(exactly = 1) { noteRepository.listNotes(NotesScope.ALL) }

            coEvery { noteRepository.listNotes(NotesScope.ALL) } returns Result.success(
                UnfiledNotesPage(
                    notes = listOf(Note(id = 2, content = "Two"), Note(id = 1, content = "One")),
                    nextCursor = null,
                    total = 2
                )
            )
            vm.onResumed()
            advanceUntilIdle()

            assertEquals(listOf(2, 1), vm.uiState.value.notes.map { it.id })
            coVerify(exactly = 2) { noteRepository.listNotes(NotesScope.ALL) }
        }

    @Test
    fun `onResumed during the initial load does not start a second request`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { noteRepository.listNotes(NotesScope.ALL) } returns Result.success(
                UnfiledNotesPage(notes = emptyList(), nextCursor = null, total = 0)
            )

            val vm = NotesInboxViewModel(noteRepository)
            vm.onResumed()
            advanceUntilIdle()

            coVerify(exactly = 1) { noteRepository.listNotes(NotesScope.ALL) }
        }

    @Test
    fun `failure surfaces the display message`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { noteRepository.listNotes(NotesScope.ALL) } returns Result.failure(
            ApiError.Client(500, "server error")
        )

        val vm = NotesInboxViewModel(noteRepository)
        advanceUntilIdle()

        assertEquals("server error", vm.uiState.value.error?.message)
        assertTrue(vm.uiState.value.notes.isEmpty())
    }

    // Ticket's pagination trap: a test that only asserts "more items appeared" would pass
    // against a re-fetch of page 1 — assert both pages are present with no duplicates.
    @Test
    fun `loadMore appends the next page without duplicating`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { noteRepository.listNotes(NotesScope.ALL) } returns Result.success(
            UnfiledNotesPage(
                notes = listOf(Note(id = 1, content = "One")),
                nextCursor = "cursor-2",
                total = 3
            )
        )
        coEvery {
            noteRepository.listNotes(
                NotesScope.ALL,
                cursor = "cursor-2"
            )
        } returns Result.success(
            UnfiledNotesPage(
                notes = listOf(Note(id = 2, content = "Two")),
                nextCursor = null,
                total = 3
            )
        )

        val vm = NotesInboxViewModel(noteRepository)
        advanceUntilIdle()
        vm.loadMore()
        advanceUntilIdle()

        val ids = vm.uiState.value.notes.map { it.id }
        assertEquals(listOf(1, 2), ids)
        assertNull(vm.uiState.value.nextCursor)
        coVerify(exactly = 1) { noteRepository.listNotes(NotesScope.ALL, cursor = "cursor-2") }
    }

    @Test
    fun `loadMore is a no-op with no next cursor`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { noteRepository.listNotes(NotesScope.ALL) } returns Result.success(
            UnfiledNotesPage(
                notes = listOf(Note(id = 1, content = "One")),
                nextCursor = null,
                total = 1
            )
        )

        val vm = NotesInboxViewModel(noteRepository)
        advanceUntilIdle()
        vm.loadMore()
        advanceUntilIdle()

        // Only the initial load() call — loadMore() short-circuited on a null cursor.
        coVerify(
            exactly = 1
        ) { noteRepository.listNotes(NotesScope.ALL, cursor = null, limit = null) }
    }

    // Issue #2224: the default is every note (like web and Activities); Unfiled is the filter.
    @Test
    fun `defaults to the all scope and switching to unfiled reloads with that scope`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { noteRepository.listNotes(NotesScope.ALL) } returns Result.success(
                UnfiledNotesPage(
                    notes = listOf(
                        Note(id = 1, content = "Filed"),
                        Note(id = 2, content = "Loose")
                    ),
                    nextCursor = null,
                    total = 2
                )
            )
            coEvery { noteRepository.listNotes(NotesScope.UNFILED) } returns Result.success(
                UnfiledNotesPage(
                    notes = listOf(Note(id = 2, content = "Loose")),
                    nextCursor = null,
                    total = 1
                )
            )

            val vm = NotesInboxViewModel(noteRepository)
            advanceUntilIdle()
            assertEquals(NotesScope.ALL, vm.uiState.value.scope)
            assertEquals(2, vm.uiState.value.total)

            vm.setScope(NotesScope.UNFILED)
            advanceUntilIdle()

            assertEquals(NotesScope.UNFILED, vm.uiState.value.scope)
            assertEquals(listOf(2), vm.uiState.value.notes.map { it.id })
            assertEquals(1, vm.uiState.value.total)
            coVerify(exactly = 1) { noteRepository.listNotes(NotesScope.UNFILED) }
        }

    @Test
    fun `selecting the active scope again does not reload`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { noteRepository.listNotes(NotesScope.ALL) } returns Result.success(
                UnfiledNotesPage(notes = emptyList(), nextCursor = null, total = 0)
            )

            val vm = NotesInboxViewModel(noteRepository)
            advanceUntilIdle()
            vm.setScope(NotesScope.ALL)
            advanceUntilIdle()

            coVerify(exactly = 1) { noteRepository.listNotes(NotesScope.ALL) }
        }

    @Test
    fun `loadMore and resume keep the selected scope`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { noteRepository.listNotes(NotesScope.UNFILED) } returns Result.success(
                UnfiledNotesPage(notes = listOf(Note(id = 1)), nextCursor = "c2", total = 2)
            )
            coEvery { noteRepository.listNotes(NotesScope.UNFILED, cursor = "c2") } returns
                Result.success(
                    UnfiledNotesPage(notes = listOf(Note(id = 2)), nextCursor = null, total = 2)
                )
            coEvery { noteRepository.listNotes(NotesScope.ALL) } returns Result.success(
                UnfiledNotesPage(notes = emptyList(), nextCursor = null, total = 0)
            )

            val vm = NotesInboxViewModel(noteRepository)
            advanceUntilIdle()
            vm.setScope(NotesScope.UNFILED)
            advanceUntilIdle()
            vm.loadMore()
            advanceUntilIdle()
            vm.onResumed()
            advanceUntilIdle()

            coVerify(exactly = 1) { noteRepository.listNotes(NotesScope.UNFILED, cursor = "c2") }
            coVerify(exactly = 2) { noteRepository.listNotes(NotesScope.UNFILED) }
        }

    @Test
    fun `a slow response for the previous scope does not overwrite the new one`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val slowAll = kotlinx.coroutines.CompletableDeferred<Result<UnfiledNotesPage>>()
            coEvery { noteRepository.listNotes(NotesScope.ALL) } coAnswers { slowAll.await() }
            coEvery { noteRepository.listNotes(NotesScope.UNFILED) } returns Result.success(
                UnfiledNotesPage(notes = listOf(Note(id = 2)), nextCursor = null, total = 1)
            )

            val vm = NotesInboxViewModel(noteRepository)
            vm.setScope(NotesScope.UNFILED)
            advanceUntilIdle()
            slowAll.complete(
                Result.success(
                    UnfiledNotesPage(
                        notes = listOf(Note(id = 1), Note(id = 2)),
                        nextCursor = null,
                        total = 2
                    )
                )
            )
            advanceUntilIdle()

            assertEquals(listOf(2), vm.uiState.value.notes.map { it.id })
            assertEquals(1, vm.uiState.value.total)
        }

    // v1.6.0 parity with web's /notes page: each row shows its photos. A row
    // whose image fetch fails just shows none; the list itself still loads.
    @Test
    fun `loads each note's photos after the list`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { noteRepository.listNotes(NotesScope.ALL) } returns Result.success(
            UnfiledNotesPage(
                notes = listOf(
                    Note(id = 3, content = "Buy milk"),
                    Note(id = 4, content = "Call mom")
                ),
                nextCursor = "c2",
                total = 3
            )
        )
        coEvery { noteRepository.listImages(3) } returns
            Result.success(listOf(NoteImage(id = 9, noteId = 3, sensitivity = "secret")))
        coEvery { noteRepository.listImages(4) } returns
            Result.failure(ApiError.Server(500, "thumbs down"))
        coEvery { noteRepository.listNotes(NotesScope.ALL, cursor = "c2") } returns Result.success(
            UnfiledNotesPage(
                notes = listOf(Note(id = 5, content = "Gift idea")),
                nextCursor = null,
                total = 3
            )
        )
        coEvery { noteRepository.listImages(5) } returns
            Result.success(listOf(NoteImage(id = 12, noteId = 5)))

        val vm = NotesInboxViewModel(noteRepository)
        advanceUntilIdle()

        assertEquals(listOf(9), vm.uiState.value.imagesByNote[3]?.map { it.id })
        assertNull(vm.uiState.value.imagesByNote[4])
        assertNull(vm.uiState.value.error)

        vm.loadMore()
        advanceUntilIdle()

        assertEquals(listOf(12), vm.uiState.value.imagesByNote[5]?.map { it.id })
    }
}
