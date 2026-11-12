package com.mycorrhizal.crm.feature.circles

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.CircleDetail
import com.mycorrhizal.crm.domain.repository.CircleRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.CircleTimelineItem
import com.mycorrhizal.crm.model.network.CircleTimelinePage
import com.mycorrhizal.crm.model.network.ErrorReason
import com.mycorrhizal.crm.model.network.ErrorReasonCode
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
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

/** Issue #2522: circle description/colour editing and the aggregate member timeline. */
class CircleDetailMetadataViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val circleRepository = mockk<CircleRepository>()
    private val contactRepository = mockk<ContactRepository>()

    private fun item(id: String) = CircleTimelineItem(
        type = "note",
        id = id,
        date = "2026-09-01T10:00:00Z"
    )

    @Before
    fun setUp() {
        coEvery { contactRepository.resolveByUid(any()) } returns Result.success(emptyMap())
        coEvery { circleRepository.getTimeline(any(), any()) } returns
            Result.success(CircleTimelinePage())
        coEvery { circleRepository.getWithMembers("c1") } returns Result.success(
            CircleDetail(
                circle = Circle(id = "c1", name = "Friends", description = "Old", color = "red"),
                members = emptyList()
            )
        )
    }

    private fun viewModel() = CircleDetailViewModel(
        circleRepository,
        contactRepository,
        SavedStateHandle(mapOf("circleId" to "c1"))
    )

    @Test
    fun `init loads the first timeline page with its cursor and member count`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { circleRepository.getTimeline("c1", null) } returns Result.success(
                CircleTimelinePage(items = listOf(item("1"), item("2")), nextCursor = "n2", memberCount = 5)
            )

            val vm = viewModel()
            advanceUntilIdle()

            val state = vm.uiState.value
            assertEquals(listOf("1", "2"), state.timeline.map { it.id })
            assertEquals("n2", state.timelineCursor)
            assertEquals(5, state.timelineMemberCount)
            assertTrue(state.timelineLoaded)
            assertFalse(state.isTimelineLoading)
            assertEquals("red", state.circle?.color)
            assertEquals("Old", state.circle?.description)
        }

    @Test
    fun `an empty timeline is loaded with no items and no cursor`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { circleRepository.getTimeline("c1", null) } returns
                Result.success(CircleTimelinePage(memberCount = 2))

            val vm = viewModel()
            advanceUntilIdle()

            assertTrue(vm.uiState.value.timelineLoaded)
            assertTrue(vm.uiState.value.timeline.isEmpty())
            assertNull(vm.uiState.value.timelineCursor)
        }

    @Test
    fun `a timeline failure is kept apart from the circle and member state`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val boom = ApiError.Server(500, "boom")
            coEvery { circleRepository.getTimeline("c1", null) } returns Result.failure(boom)

            val vm = viewModel()
            advanceUntilIdle()

            val state = vm.uiState.value
            assertEquals(boom, state.timelineError)
            assertNull(state.error)
            assertEquals("Friends", state.circle?.name)
            assertFalse(state.timelineLoaded)
        }

    @Test
    fun `loadMoreTimeline appends the next page using the cursor`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { circleRepository.getTimeline("c1", null) } returns Result.success(
                CircleTimelinePage(items = listOf(item("1")), nextCursor = "n2", memberCount = 1)
            )
            coEvery { circleRepository.getTimeline("c1", "n2") } returns Result.success(
                CircleTimelinePage(items = listOf(item("2")), nextCursor = null, memberCount = 1)
            )
            val vm = viewModel()
            advanceUntilIdle()

            vm.loadMoreTimeline()
            advanceUntilIdle()

            assertEquals(listOf("1", "2"), vm.uiState.value.timeline.map { it.id })
            assertNull(vm.uiState.value.timelineCursor)
        }

    @Test
    fun `loadMoreTimeline does nothing on the last page`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { circleRepository.getTimeline("c1", null) } returns
                Result.success(CircleTimelinePage(items = listOf(item("1"))))
            val vm = viewModel()
            advanceUntilIdle()

            vm.loadMoreTimeline()
            advanceUntilIdle()

            coVerify(exactly = 1) { circleRepository.getTimeline(any(), any()) }
        }

    @Test
    fun `a second loadMoreTimeline while one is in flight is ignored`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { circleRepository.getTimeline("c1", null) } returns Result.success(
                CircleTimelinePage(items = listOf(item("1")), nextCursor = "n2")
            )
            val gate = CompletableDeferred<Result<CircleTimelinePage>>()
            coEvery { circleRepository.getTimeline("c1", "n2") } coAnswers { gate.await() }
            val vm = viewModel()
            advanceUntilIdle()

            vm.loadMoreTimeline()
            vm.loadMoreTimeline()
            gate.complete(Result.success(CircleTimelinePage(items = listOf(item("2")))))
            advanceUntilIdle()

            coVerify(exactly = 1) { circleRepository.getTimeline("c1", "n2") }
            assertEquals(listOf("1", "2"), vm.uiState.value.timeline.map { it.id })
        }

    @Test
    fun `a failed load-more keeps the loaded items and the cursor so it can be retried`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { circleRepository.getTimeline("c1", null) } returns Result.success(
                CircleTimelinePage(items = listOf(item("1")), nextCursor = "n2")
            )
            coEvery { circleRepository.getTimeline("c1", "n2") } returns
                Result.failure(ApiError.Server(500, "boom"))
            val vm = viewModel()
            advanceUntilIdle()

            vm.loadMoreTimeline()
            advanceUntilIdle()

            assertEquals(listOf("1"), vm.uiState.value.timeline.map { it.id })
            assertEquals("n2", vm.uiState.value.timelineCursor)
            assertTrue(vm.uiState.value.timelineError != null)
        }

    @Test
    fun `reloading replaces the timeline rather than appending`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { circleRepository.getTimeline("c1", null) } returnsMany listOf(
                Result.success(CircleTimelinePage(items = listOf(item("1")))),
                Result.success(CircleTimelinePage(items = listOf(item("9"))))
            )
            val vm = viewModel()
            advanceUntilIdle()

            vm.load()
            advanceUntilIdle()

            assertEquals(listOf("9"), vm.uiState.value.timeline.map { it.id })
        }

    @Test
    fun `update saves trimmed values, updates the circle in state and calls onDone`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { circleRepository.getTimeline(any(), any()) } returns
                Result.success(CircleTimelinePage())
            coEvery { circleRepository.update("c1", "Family", "Close", "teal") } returns
                Result.success(
                    Circle(id = "c1", name = "Family", description = "Close", color = "teal")
                )
            val vm = viewModel()
            advanceUntilIdle()
            var done = false

            vm.update("  Family ", "  Close\n", "teal") { done = true }
            advanceUntilIdle()

            assertTrue(done)
            assertEquals("Family", vm.uiState.value.circle?.name)
            assertEquals("Close", vm.uiState.value.circle?.description)
            assertEquals("teal", vm.uiState.value.circle?.color)
            assertFalse(vm.uiState.value.isSaving)
            assertFalse(vm.uiState.value.nameTaken)
        }

    @Test
    fun `update with a blank name does not call the server`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = viewModel()
            advanceUntilIdle()

            vm.update("   ", "x", "red") {}
            advanceUntilIdle()

            coVerify(exactly = 0) { circleRepository.update(any(), any(), any(), any()) }
        }

    @Test
    fun `update with a colour outside the palette does not call the server`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = viewModel()
            advanceUntilIdle()

            vm.update("Family", "x", "chartreuse") {}
            advanceUntilIdle()

            coVerify(exactly = 0) { circleRepository.update(any(), any(), any(), any()) }
        }

    @Test
    fun `an empty colour clears it and is sent through`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { circleRepository.update("c1", "Friends", "", "") } returns
                Result.success(Circle(id = "c1", name = "Friends"))
            val vm = viewModel()
            advanceUntilIdle()

            vm.update("Friends", "", "") {}
            advanceUntilIdle()

            assertEquals("", vm.uiState.value.circle?.color)
            coVerify(exactly = 1) { circleRepository.update("c1", "Friends", "", "") }
        }

    @Test
    fun `a duplicate name sets the inline error and not the snackbar error`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { circleRepository.update(any(), any(), any(), any()) } returns Result.failure(
                ApiError.Client(
                    409,
                    "exists",
                    reason = ErrorReason(ErrorReasonCode.CIRCLE_ALREADY_EXISTS, emptyMap())
                )
            )
            val vm = viewModel()
            advanceUntilIdle()
            var done = false

            vm.update("Other", "", "") { done = true }
            advanceUntilIdle()

            assertTrue(vm.uiState.value.nameTaken)
            assertNull(vm.uiState.value.error)
            assertFalse(done)
            assertEquals("Friends", vm.uiState.value.circle?.name)

            vm.clearNameError()
            assertFalse(vm.uiState.value.nameTaken)
        }

    @Test
    fun `any other update failure is reported as the screen error`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val boom = ApiError.Server(500, "boom")
            coEvery { circleRepository.update(any(), any(), any(), any()) } returns
                Result.failure(boom)
            val vm = viewModel()
            advanceUntilIdle()

            vm.update("Friends", "d", "red") {}
            advanceUntilIdle()

            assertEquals(boom, vm.uiState.value.error)
            assertFalse(vm.uiState.value.nameTaken)
            assertFalse(vm.uiState.value.isSaving)
        }

    @Test
    fun `a slow superseded first-page response cannot overwrite a newer reload`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val slow = CompletableDeferred<Result<CircleTimelinePage>>()
            coEvery { circleRepository.getTimeline("c1", null) } coAnswers { slow.await() } andThen
                Result.success(CircleTimelinePage(items = listOf(item("new"))))
            val vm = viewModel()
            advanceUntilIdle()

            vm.load()
            advanceUntilIdle()
            slow.complete(Result.success(CircleTimelinePage(items = listOf(item("old")))))
            advanceUntilIdle()

            assertEquals(listOf("new"), vm.uiState.value.timeline.map { it.id })
        }

    @Test
    fun `retry after a failed first page loads the first page again`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { circleRepository.getTimeline("c1", null) } returnsMany listOf(
                Result.failure(ApiError.Server(500, "boom")),
                Result.success(CircleTimelinePage(items = listOf(item("1"))))
            )
            val vm = viewModel()
            advanceUntilIdle()
            assertTrue(vm.uiState.value.timelineError != null)

            vm.retryTimeline()
            advanceUntilIdle()

            assertNull(vm.uiState.value.timelineError)
            assertEquals(listOf("1"), vm.uiState.value.timeline.map { it.id })
        }
}
