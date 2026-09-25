package com.mycorrhizal.crm.feature.occasions

import com.mycorrhizal.crm.domain.repository.OccasionEventRepository
import com.mycorrhizal.crm.model.network.OccasionEvent
import com.mycorrhizal.crm.model.network.OccasionEventInput
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class OccasionsViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val repository = mockk<OccasionEventRepository>()

    private fun event(id: String = "e1", title: String = "Summer BBQ") =
        OccasionEvent(id = id, title = title, startsAt = "2026-07-04T15:00:00Z")

    @Test
    fun `load populates the event list`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list() } returns Result.success(listOf(event()))

        val viewModel = OccasionsViewModel(repository)
        advanceUntilIdle()

        assertEquals(1, viewModel.uiState.value.events.size)
        assertNull(viewModel.uiState.value.error)
    }

    @Test
    fun `load surfaces a fetch failure`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list() } returns Result.failure(RuntimeException("boom"))

        val viewModel = OccasionsViewModel(repository)
        advanceUntilIdle()

        assertEquals("Something went wrong", viewModel.uiState.value.error)
        assertEquals(0, viewModel.uiState.value.events.size)
    }

    @Test
    fun `create sends the input and reloads`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list() } returns Result.success(emptyList())
        coEvery { repository.create(any()) } returns Result.success(event("e9"))
        val viewModel = OccasionsViewModel(repository)
        advanceUntilIdle()

        viewModel.create("Party", "2026-07-04T15:00:00Z", null, "Park", "normal", "notes")
        advanceUntilIdle()

        coVerify { repository.create(OccasionEventInput("Party", "2026-07-04T15:00:00Z", null, "Park", "normal", "notes")) }
        coVerify(atLeast = 2) { repository.list() }
    }

    @Test
    fun `update sends the id and input and reloads`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list() } returns Result.success(listOf(event()))
        coEvery { repository.update(any(), any()) } returns Result.success(event())
        val viewModel = OccasionsViewModel(repository)
        advanceUntilIdle()

        viewModel.update("e1", "Renamed", "2026-07-04T15:00:00Z", null, null, "normal", null)
        advanceUntilIdle()

        coVerify { repository.update("e1", OccasionEventInput("Renamed", "2026-07-04T15:00:00Z", null, null, "normal", null)) }
    }

    @Test
    fun `delete calls the repository and reloads`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list() } returns Result.success(listOf(event()))
        coEvery { repository.delete("e1") } returns Result.success(Unit)
        val viewModel = OccasionsViewModel(repository)
        advanceUntilIdle()

        viewModel.delete("e1")
        advanceUntilIdle()

        coVerify { repository.delete("e1") }
    }

    @Test
    fun `a failed delete surfaces an error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list() } returns Result.success(listOf(event()))
        coEvery { repository.delete("e1") } returns Result.failure(RuntimeException("boom"))
        val viewModel = OccasionsViewModel(repository)
        advanceUntilIdle()

        viewModel.delete("e1")
        advanceUntilIdle()

        assertEquals("Something went wrong", viewModel.uiState.value.error)
    }

    @Test
    fun `onErrorShown clears the error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list() } returns Result.failure(RuntimeException("boom"))
        val viewModel = OccasionsViewModel(repository)
        advanceUntilIdle()

        viewModel.onErrorShown()

        assertNull(viewModel.uiState.value.error)
    }
}
