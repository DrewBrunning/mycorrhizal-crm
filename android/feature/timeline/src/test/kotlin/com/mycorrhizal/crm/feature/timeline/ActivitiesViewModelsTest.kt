package com.mycorrhizal.crm.feature.timeline

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.ActivityRepository
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.ActivityInput
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

class ActivitiesViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val activityRepository = mockk<ActivityRepository>()

    private fun viewModel(contactId: Int = 5): ActivitiesViewModel =
        ActivitiesViewModel(activityRepository, SavedStateHandle(mapOf("contactId" to contactId)))

    @Test
    fun `loads the contact's activities on init`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { activityRepository.listForContact(5) } returns Result.success(
            listOf(Activity(id = 1, title = "Coffee"), Activity(id = 2, title = "Call")),
        )

        val vm = viewModel()
        advanceUntilIdle()

        val state = vm.uiState.value
        assertFalse(state.isLoading)
        assertEquals(2, state.activities.size)
        assertEquals("Coffee", state.activities[0].title)
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
        coEvery { activityRepository.listForContact(5) } returns Result.failure(
            ApiError.Client(404, "Contact not found"),
        )

        val vm = viewModel()
        advanceUntilIdle()

        assertEquals("Not found", vm.uiState.value.error)
        assertTrue(vm.uiState.value.activities.isEmpty())
    }
}

class ActivityFormViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val activityRepository = mockk<ActivityRepository>()

    private fun createViewModel(contactId: Int = 5, activityId: Int? = null): ActivityFormViewModel {
        val handle = SavedStateHandle(mapOf("contactId" to contactId).toMutableMap().apply {
            if (activityId != null) put("activityId", activityId)
        })
        return ActivityFormViewModel(activityRepository, handle)
    }

    @Test
    fun `create mode starts empty`() {
        val vm = createViewModel()
        val state = vm.uiState.value
        assertFalse(state.isEdit)
        assertEquals(5, state.contactId)
        assertNull(state.activityId)
    }

    @Test
    fun `save without a title is blocked`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = createViewModel()
        vm.onTypeChange("call")
        vm.save()
        advanceUntilIdle()

        assertEquals("Title is required", vm.uiState.value.error)
        coVerify(exactly = 0) { activityRepository.create(any()) }
    }

    @Test
    fun `save in create mode calls create with the contact id`() = runTest(mainDispatcherRule.testDispatcher) {
        val created = Activity(id = 7, title = "Lunch", type = "meal")
        coEvery { activityRepository.create(any()) } returns Result.success(created)

        val vm = createViewModel()
        vm.onTitleChange("Lunch")
        vm.onTypeChange("meal")
        vm.onDateChange("2026-08-10T14:00:00Z")
        vm.save()
        advanceUntilIdle()

        coVerify {
            activityRepository.create(
                match<ActivityInput> { input ->
                    input.title == "Lunch" &&
                        input.type == "meal" &&
                        input.date == "2026-08-10T14:00:00Z" &&
                        input.contactIds == listOf(5)
                },
            )
        }
        assertEquals(ActivityFormEvent.Saved, vm.events.value)
        assertFalse(vm.uiState.value.isSaving)
    }

    @Test
    fun `save in edit mode hydrates and calls update`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { activityRepository.get(7) } returns Result.success(
            Activity(id = 7, title = "Lunch", type = "meal"),
        )
        coEvery { activityRepository.update(7, any()) } returns Result.success(Activity(id = 7, title = "Lunch"))

        val vm = createViewModel(activityId = 7)
        advanceUntilIdle()

        assertEquals("Lunch", vm.uiState.value.title)
        assertEquals("meal", vm.uiState.value.type)

        vm.onTitleChange("Dinner")
        vm.save()
        advanceUntilIdle()

        coVerify {
            activityRepository.update(
                7,
                match<ActivityInput> { input -> input.title == "Dinner" },
            )
        }
        assertEquals(ActivityFormEvent.Saved, vm.events.value)
    }

    @Test
    fun `edit preserves participants and external ref`() = runTest(mainDispatcherRule.testDispatcher) {
        // An activity spanning two contacts, with an external ref (calendar link).
        coEvery { activityRepository.get(7) } returns Result.success(
            Activity(
                id = 7,
                title = "Lunch",
                type = "meal",
                contacts = listOf(
                    com.mycorrhizal.crm.model.network.ContactFlat(id = 5),
                    com.mycorrhizal.crm.model.network.ContactFlat(id = 9),
                ),
                externalRef = "cal-event-42",
            ),
        )
        coEvery { activityRepository.update(7, any()) } returns Result.success(Activity(id = 7, title = "Lunch"))

        val vm = createViewModel(activityId = 7)
        advanceUntilIdle()

        vm.onTitleChange("Dinner")
        vm.save()
        advanceUntilIdle()

        coVerify {
            activityRepository.update(
                7,
                match<ActivityInput> { input ->
                    // Both participants preserved — the other contact isn't dropped.
                    input.contactIds == listOf(5, 9) &&
                        input.externalRef == "cal-event-42"
                },
            )
        }
    }

    @Test
    fun `edit hydration failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { activityRepository.get(7) } returns Result.failure(
            ApiError.Client(404, "Activity not found"),
        )

        val vm = createViewModel(activityId = 7)
        advanceUntilIdle()

        assertEquals("Not found", vm.uiState.value.error)
        assertFalse(vm.uiState.value.isLoading)
    }

    @Test
    fun `invalid date format blocks save`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = createViewModel()
        vm.onTitleChange("Lunch")
        vm.onDateChange("2026-08-10")
        vm.save()
        advanceUntilIdle()

        assertEquals("Date must be ISO 8601, e.g. 2026-08-10T14:00:00Z", vm.uiState.value.error)
        coVerify(exactly = 0) { activityRepository.create(any()) }
    }

    @Test
    fun `iso date time passes validation`() = runTest(mainDispatcherRule.testDispatcher) {
        val created = Activity(id = 7, title = "Lunch")
        coEvery { activityRepository.create(any()) } returns Result.success(created)

        val vm = createViewModel()
        vm.onTitleChange("Lunch")
        vm.onDateChange("2026-08-10T14:00:00Z")
        vm.save()
        advanceUntilIdle()

        assertEquals(ActivityFormEvent.Saved, vm.events.value)
    }

    @Test
    fun `failed save surfaces the error and stays`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { activityRepository.create(any()) } returns Result.failure(
            ApiError.Client(409, "duplicate"),
        )

        val vm = createViewModel()
        vm.onTitleChange("Lunch")
        vm.save()
        advanceUntilIdle()

        assertEquals("duplicate", vm.uiState.value.error)
        assertFalse(vm.uiState.value.isSaving)
        assertNull(vm.events.value)
    }
}
