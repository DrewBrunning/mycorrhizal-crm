package com.mycorrhizal.crm.feature.timeline

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.ContactTimelineRepository
import com.mycorrhizal.crm.domain.repository.ReminderRepository
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.ContactTimelinePage
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.model.network.ReminderCompletion
import com.mycorrhizal.crm.model.network.TimelineBuckets
import com.mycorrhizal.crm.model.network.TimelineEvent
import com.mycorrhizal.crm.model.network.TimelineTypes
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

/** Issue #1401: the timeline explorer's paging, filters, refresh and error handling. */
class TimelineExplorerViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val timelineRepository = mockk<ContactTimelineRepository>()
    private val reminderRepository = mockk<ReminderRepository>()

    private fun viewModel(contactId: Int = 5) = TimelineExplorerViewModel(
        timelineRepository,
        reminderRepository,
        SavedStateHandle(mapOf("contactId" to contactId)),
    )

    private fun note(id: Int, content: String = "note $id") =
        TimelineEvent(TimelineTypes.NOTE, id.toString(), "2026-09-0${id}T10:00:00Z", note = Note(id = id, content = content))

    private fun activity(id: Int) =
        TimelineEvent(TimelineTypes.ACTIVITY, id.toString(), "2026-08-0${id}T10:00:00Z", activity = Activity(id = id, title = "act $id"))

    private fun page(vararg events: TimelineEvent, next: String? = null) =
        Result.success(ContactTimelinePage(events.toList(), next, 25))

    @Test
    fun `nothing is fetched until refresh is called`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = viewModel()
        advanceUntilIdle()

        assertTrue(vm.uiState.value.items.isEmpty())
        assertFalse(vm.uiState.value.isLoading)
    }

    @Test
    fun `refresh loads the first page with no filters and maps events to rows`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { timelineRepository.page(5, emptySet(), TimelineBuckets.ALL, null, 25) } returns
            page(note(2), activity(1), next = "cur-1")

        val vm = viewModel()
        vm.refresh()
        advanceUntilIdle()

        val state = vm.uiState.value
        assertFalse(state.isLoading)
        assertEquals(listOf("NoteItem:2", "ActivityItem:1"), state.items.map { it.key })
        assertEquals("cur-1", state.nextCursor)
        assertNull(state.error)
    }

    @Test
    fun `load more appends the next page using the cursor and drops duplicates`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { timelineRepository.page(5, emptySet(), TimelineBuckets.ALL, null, 25) } returns
            page(note(3), note(2), next = "cur-1")
        coEvery { timelineRepository.page(5, emptySet(), TimelineBuckets.ALL, "cur-1", 25) } returns
            page(note(2), note(1), next = null)

        val vm = viewModel()
        vm.refresh()
        advanceUntilIdle()
        vm.loadMore()
        advanceUntilIdle()

        val state = vm.uiState.value
        assertEquals(listOf("NoteItem:3", "NoteItem:2", "NoteItem:1"), state.items.map { it.key })
        assertNull(state.nextCursor)
        assertFalse(state.isLoadingMore)
    }

    @Test
    fun `load more is a no-op at the end of the timeline`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { timelineRepository.page(5, emptySet(), TimelineBuckets.ALL, null, 25) } returns page(note(1))

        val vm = viewModel()
        vm.refresh()
        advanceUntilIdle()
        vm.loadMore()
        advanceUntilIdle()

        coVerify(exactly = 1) { timelineRepository.page(any(), any(), any(), any(), any()) }
    }

    @Test
    fun `changing the type filter restarts from the first page with that filter`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { timelineRepository.page(5, emptySet(), TimelineBuckets.ALL, null, 25) } returns
            page(note(2), activity(1), next = "cur-1")
        coEvery { timelineRepository.page(5, setOf(TimelineTypes.NOTE), TimelineBuckets.ALL, null, 25) } returns
            page(note(2))

        val vm = viewModel()
        vm.refresh()
        advanceUntilIdle()
        vm.setTypes(setOf(TimelineTypes.NOTE))
        advanceUntilIdle()

        val state = vm.uiState.value
        assertEquals(setOf(TimelineTypes.NOTE), state.types)
        assertEquals(listOf("NoteItem:2"), state.items.map { it.key })
        assertNull(state.nextCursor)
        coVerify { timelineRepository.page(5, setOf(TimelineTypes.NOTE), TimelineBuckets.ALL, null, 25) }
    }

    @Test
    fun `changing the recency bucket re-queries with that bucket`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { timelineRepository.page(5, emptySet(), any(), null, 25) } returns page(note(1))

        val vm = viewModel()
        vm.refresh()
        advanceUntilIdle()
        vm.setBucket(TimelineBuckets.LAST_7_DAYS)
        advanceUntilIdle()

        assertEquals(TimelineBuckets.LAST_7_DAYS, vm.uiState.value.bucket)
        coVerify { timelineRepository.page(5, emptySet(), TimelineBuckets.LAST_7_DAYS, null, 25) }
    }

    @Test
    fun `re-selecting the current filter does not refetch`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { timelineRepository.page(any(), any(), any(), any(), any()) } returns page(note(1))

        val vm = viewModel()
        vm.refresh()
        advanceUntilIdle()
        vm.setTypes(emptySet())
        vm.setBucket(TimelineBuckets.ALL)
        advanceUntilIdle()

        coVerify(exactly = 1) { timelineRepository.page(any(), any(), any(), any(), any()) }
    }

    @Test
    fun `refresh re-fetches the first page so an edit made elsewhere shows when the screen is revisited`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { timelineRepository.page(5, emptySet(), TimelineBuckets.ALL, null, 25) } returnsMany listOf(
                page(note(1, "before edit")),
                page(note(1, "after edit")),
            )

            val vm = viewModel()
            vm.refresh()
            advanceUntilIdle()
            vm.refresh()
            advanceUntilIdle()

            val shown = (vm.uiState.value.items.single() as TimelineItem.NoteItem).note.content
            assertEquals("after edit", shown)
        }

    @Test
    fun `a failed first load surfaces the error and keeps no rows`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { timelineRepository.page(any(), any(), any(), any(), any()) } returns
            Result.failure(ApiError.Client(404, "Contact not found"))

        val vm = viewModel()
        vm.refresh()
        advanceUntilIdle()

        assertEquals("Not found", vm.uiState.value.error)
        assertTrue(vm.uiState.value.items.isEmpty())
        assertFalse(vm.uiState.value.isLoading)
    }

    @Test
    fun `a failed load more keeps the rows already shown and the cursor so it can be retried`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { timelineRepository.page(5, emptySet(), TimelineBuckets.ALL, null, 25) } returns
                page(note(2), next = "cur-1")
            coEvery { timelineRepository.page(5, emptySet(), TimelineBuckets.ALL, "cur-1", 25) } returns
                Result.failure(ApiError.Client(404, "Contact not found"))

            val vm = viewModel()
            vm.refresh()
            advanceUntilIdle()
            vm.loadMore()
            advanceUntilIdle()

            val state = vm.uiState.value
            assertEquals(1, state.items.size)
            assertEquals("cur-1", state.nextCursor)
            assertEquals("Not found", state.error)
            assertFalse(state.isLoadingMore)

            vm.onErrorShown()
            assertNull(vm.uiState.value.error)
        }

    @Test
    fun `undoing a completion deletes it then reloads the timeline`() = runTest(mainDispatcherRule.testDispatcher) {
        val completion = TimelineEvent(
            TimelineTypes.COMPLETION, "9", "2026-09-01T10:00:00Z",
            completion = ReminderCompletion(id = 9, contactId = 5, message = "Done"),
        )
        coEvery { timelineRepository.page(5, emptySet(), TimelineBuckets.ALL, null, 25) } returnsMany listOf(
            page(completion),
            page(),
        )
        coEvery { reminderRepository.deleteCompletion(9) } returns Result.success(Unit)

        val vm = viewModel()
        vm.refresh()
        advanceUntilIdle()
        assertEquals(1, vm.uiState.value.items.size)
        vm.undoCompletion(9)
        advanceUntilIdle()

        coVerify { reminderRepository.deleteCompletion(9) }
        assertTrue(vm.uiState.value.items.isEmpty())
    }

    @Test
    fun `a failed completion undo reports the error without reloading`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { reminderRepository.deleteCompletion(9) } returns Result.failure(ApiError.Client(404, "gone"))

        val vm = viewModel()
        vm.undoCompletion(9)
        advanceUntilIdle()

        assertEquals("Not found", vm.uiState.value.error)
        coVerify(exactly = 0) { timelineRepository.page(any(), any(), any(), any(), any()) }
    }

    @Test
    fun `a missing contact id never hits the network`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = viewModel(contactId = 0)
        vm.refresh()
        advanceUntilIdle()

        coVerify(exactly = 0) { timelineRepository.page(any(), any(), any(), any(), any()) }
    }
}
