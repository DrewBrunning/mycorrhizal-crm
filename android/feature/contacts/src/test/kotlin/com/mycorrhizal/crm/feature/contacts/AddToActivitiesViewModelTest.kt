package com.mycorrhizal.crm.feature.contacts

import com.mycorrhizal.crm.domain.repository.ActivityRepository
import com.mycorrhizal.crm.model.network.ActivitiesPage
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.AddContactToActivitiesResponse
import com.mycorrhizal.crm.model.network.AddToActivityResult
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

class AddToActivitiesViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val activities = listOf(
        Activity(id = 1, title = "Coffee"),
        Activity(id = 2, title = "Lunch"),
        Activity(id = 3, title = "Call")
    )

    private fun newViewModel(): Pair<AddToActivitiesViewModel, ActivityRepository> {
        val repository = mockk<ActivityRepository>()
        coEvery { repository.listAll(any(), any(), any()) } returns
            Result.success(ActivitiesPage(activitiesRaw = activities))
        return AddToActivitiesViewModel(repository) to repository
    }

    @Test
    fun `loads recent activities and starts with nothing selected`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val (viewModel, _) = newViewModel()
            advanceUntilIdle()

            assertEquals(activities, viewModel.uiState.value.activities)
            assertTrue(viewModel.uiState.value.selected.isEmpty())
            assertFalse(viewModel.uiState.value.canSubmit)
        }

    @Test
    fun `typing a query searches server side`() = runTest(mainDispatcherRule.testDispatcher) {
        val (viewModel, repository) = newViewModel()
        advanceUntilIdle()

        viewModel.onQueryChange("lun")
        advanceUntilIdle()

        coVerify { repository.listAll(null, any(), "lun") }
    }

    @Test
    fun `selection toggles and gates submit`() = runTest(mainDispatcherRule.testDispatcher) {
        val (viewModel, _) = newViewModel()
        advanceUntilIdle()

        viewModel.toggle(1)
        viewModel.toggle(2)
        assertEquals(setOf(1, 2), viewModel.uiState.value.selected)
        assertTrue(viewModel.uiState.value.canSubmit)
        viewModel.toggle(1)
        viewModel.toggle(2)
        assertFalse(viewModel.uiState.value.canSubmit)
    }

    @Test
    fun `submit issues one request and finishes when every result is added or already_member`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val (viewModel, repository) = newViewModel()
            coEvery { repository.addContactToActivities(5, listOf(1, 2)) } returns Result.success(
                AddContactToActivitiesResponse(
                    5,
                    listOf(
                        AddToActivityResult(1, "added"),
                        AddToActivityResult(2, "already_member")
                    )
                )
            )
            advanceUntilIdle()
            viewModel.toggle(1)
            viewModel.toggle(2)

            viewModel.submit(5)
            advanceUntilIdle()

            coVerify(exactly = 1) { repository.addContactToActivities(5, listOf(1, 2)) }
            assertTrue(viewModel.uiState.value.done)
            assertTrue(viewModel.uiState.value.failedTitles.isEmpty())
        }

    @Test
    fun `not_found and unknown results keep the sheet open and list their titles`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val (viewModel, repository) = newViewModel()
            coEvery { repository.addContactToActivities(5, listOf(1, 2, 3)) } returns
                Result.success(
                    AddContactToActivitiesResponse(
                        5,
                        listOf(
                            AddToActivityResult(1, "added"),
                            AddToActivityResult(2, "not_found"),
                            AddToActivityResult(3, "mystery")
                        )
                    )
                )
            advanceUntilIdle()
            listOf(1, 2, 3).forEach(viewModel::toggle)

            viewModel.submit(5)
            advanceUntilIdle()

            assertFalse(viewModel.uiState.value.done)
            assertEquals(listOf("Lunch", "Call"), viewModel.uiState.value.failedTitles)
            // Only the failures stay selected so a retry targets exactly them.
            assertEquals(setOf(2, 3), viewModel.uiState.value.selected)
        }

    @Test
    fun `a request failure surfaces the error and keeps the selection`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val (viewModel, repository) = newViewModel()
            coEvery { repository.addContactToActivities(5, listOf(1)) } returns
                Result.failure(ApiError.Client(404, "Contact not found"))
            advanceUntilIdle()
            viewModel.toggle(1)

            viewModel.submit(5)
            advanceUntilIdle()

            assertEquals(404, (viewModel.uiState.value.error as? ApiError.Client)?.code)
            assertFalse(viewModel.uiState.value.done)
            assertEquals(setOf(1), viewModel.uiState.value.selected)
            assertNull(viewModel.uiState.value.failedTitles.firstOrNull())
        }
}
