package com.mycorrhizal.crm.feature.timeline

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.ActivityRepository
import com.mycorrhizal.crm.domain.repository.ContactActivitiesPage
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.ActivityInput
import com.mycorrhizal.crm.model.network.ContactFlat
import com.mycorrhizal.crm.model.network.ContactSummary
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

class ActivitiesViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val activityRepository = mockk<ActivityRepository>()

    private fun viewModel(contactId: Int = 5): ActivitiesViewModel =
        ActivitiesViewModel(activityRepository, SavedStateHandle(mapOf("contactId" to contactId)))

    @Test
    fun `loads the contact's activities on init`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { activityRepository.listForContact(5, null, null, null, null, null) } returns Result.success(
            ContactActivitiesPage(
                activities = listOf(Activity(id = 1, title = "Coffee"), Activity(id = 2, title = "Call")),
                nextCursor = null,
            ),
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

        assertEquals(R.string.activity_error_missing_id, vm.uiState.value.errorRes)
    }

    @Test
    fun `failure surfaces the display message`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { activityRepository.listForContact(5, null, null, null, null, null) } returns Result.failure(
            ApiError.Client(404, "Contact not found"),
        )

        val vm = viewModel()
        advanceUntilIdle()

        assertEquals("Not found", vm.uiState.value.error)
        assertTrue(vm.uiState.value.activities.isEmpty())
    }

    @Test
    fun `search change debounces and reloads with the query`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { activityRepository.listForContact(any(), any(), any(), any(), any(), any()) } returns Result.success(
            ContactActivitiesPage(activities = emptyList(), nextCursor = null),
        )

        val vm = viewModel()
        advanceUntilIdle()
        vm.onSearchChange("coffee")
        advanceUntilIdle()

        coVerify { activityRepository.listForContact(5, null, null, "coffee", null, null) }
    }

    @Test
    fun `date filters reload with the bounds`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { activityRepository.listForContact(any(), any(), any(), any(), any(), any()) } returns Result.success(
            ContactActivitiesPage(activities = emptyList(), nextCursor = null),
        )

        val vm = viewModel()
        advanceUntilIdle()
        vm.onFromDateChange("2026-08-01")
        vm.onToDateChange("2026-08-10")
        advanceUntilIdle()

        coVerify { activityRepository.listForContact(5, null, null, null, "2026-08-01", "2026-08-10") }
    }

    @Test
    fun `a superseding load cancels the in-flight request so stale results never win`() = runTest(mainDispatcherRule.testDispatcher) {
        val staleGate = CompletableDeferred<Unit>()
        coEvery { activityRepository.listForContact(5, null, null, null, null, null) } returns Result.success(
            ContactActivitiesPage(activities = listOf(Activity(id = 1, title = "all")), nextCursor = null),
        )
        coEvery { activityRepository.listForContact(5, null, null, "a", null, null) } coAnswers {
            staleGate.await()
            Result.success(ContactActivitiesPage(activities = listOf(Activity(id = 1, title = "stale")), nextCursor = null))
        }
        coEvery { activityRepository.listForContact(5, null, null, "ab", null, null) } returns Result.success(
            ContactActivitiesPage(activities = listOf(Activity(id = 2, title = "fresh")), nextCursor = null),
        )

        val vm = viewModel()
        advanceUntilIdle()
        vm.onSearchChange("a")
        advanceUntilIdle() // load("a") starts and suspends on the gate
        vm.onSearchChange("ab")
        advanceUntilIdle() // load("ab") supersedes
        staleGate.complete(Unit) // the stale request would land here if it weren't cancelled
        advanceUntilIdle()

        assertEquals(listOf(2), vm.uiState.value.activities.map { it.id })
        assertEquals("fresh", vm.uiState.value.activities.single().title)
    }

    @Test
    fun `load more appends the next page without duplicating`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { activityRepository.listForContact(5, null, null, null, null, null) } returns Result.success(
            ContactActivitiesPage(
                activities = listOf(Activity(id = 1, title = "One"), Activity(id = 2, title = "Two")),
                nextCursor = "cursor-1",
            ),
        )
        coEvery { activityRepository.listForContact(5, "cursor-1", null, null, null, null) } returns Result.success(
            ContactActivitiesPage(activities = listOf(Activity(id = 3, title = "Three")), nextCursor = null),
        )

        val vm = viewModel()
        advanceUntilIdle()
        vm.loadMore()
        advanceUntilIdle()

        val state = vm.uiState.value
        assertEquals(listOf(1, 2, 3), state.activities.map { it.id })
        assertNull(state.nextCursor)
    }

    @Test
    fun `delete removes the item on success and keeps it on failure`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { activityRepository.listForContact(5, null, null, null, null, null) } returns Result.success(
            ContactActivitiesPage(
                activities = listOf(Activity(id = 1, title = "One"), Activity(id = 2, title = "Two")),
                nextCursor = null,
            ),
        )
        coEvery { activityRepository.delete(1) } returns Result.success(Unit)
        coEvery { activityRepository.delete(2) } returns Result.failure(ApiError.Client(500, "boom"))

        val vm = viewModel()
        advanceUntilIdle()

        vm.delete(1)
        advanceUntilIdle()
        assertEquals(listOf(2), vm.uiState.value.activities.map { it.id })

        vm.delete(2)
        advanceUntilIdle()
        assertEquals(listOf(2), vm.uiState.value.activities.map { it.id })
        assertEquals("boom", vm.uiState.value.error)
    }
}

class ActivityFormViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val activityRepository = mockk<ActivityRepository>()
    private val contactRepository = mockk<ContactRepository>()

    @Before
    fun setUp() {
        // Create mode seeds the participant list with the route contact
        // (best-effort name resolution); the failure fallback is a bare
        // "#id" chip, which is all these tests need.
        coEvery { contactRepository.getContact(any()) } returns Result.failure(RuntimeException("offline"))
    }

    private fun createViewModel(contactId: Int = 5, activityId: Int? = null): ActivityFormViewModel {
        val handle = SavedStateHandle(mapOf("contactId" to contactId).toMutableMap().apply {
            if (activityId != null) put("activityId", activityId)
        })
        return ActivityFormViewModel(activityRepository, contactRepository, handle)
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

        assertEquals(R.string.activity_error_title, vm.uiState.value.errorRes)
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

    // M19: removing every participant in edit mode must CLEAR the set
    // (`contact_ids: []`), not silently re-add the route contact — the web
    // dialog allows an activity with no participants, and the backend honors
    // `[]` as `Association.Replace(nil)`.
    @Test
    fun `edit removing all participants clears the set`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { activityRepository.get(7) } returns Result.success(
            Activity(
                id = 7,
                title = "Lunch",
                contacts = listOf(ContactFlat(id = 5, firstname = "Dana"), ContactFlat(id = 9, firstname = "Bob")),
            ),
        )
        coEvery { activityRepository.update(7, any()) } returns Result.success(Activity(id = 7, title = "Lunch"))

        val vm = createViewModel(activityId = 7)
        advanceUntilIdle()
        assertEquals(listOf(5, 9), vm.uiState.value.participants.map { it.id })

        vm.onRemoveParticipant(5)
        vm.onRemoveParticipant(9)
        assertTrue(vm.uiState.value.participants.isEmpty())

        vm.save()
        advanceUntilIdle()

        coVerify {
            activityRepository.update(
                7,
                match<ActivityInput> { input -> input.contactIds == emptyList<Int>() },
            )
        }
    }

    // M19: the multi-participant fix — an activity can span more than one contact.
    @Test
    fun `adding participants sends both on save and reloads with both`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { activityRepository.create(any()) } returns Result.success(
            Activity(id = 7, title = "Lunch", contacts = listOf(ContactFlat(id = 5), ContactFlat(id = 9))),
        )

        val vm = createViewModel()
        advanceUntilIdle() // let the route-contact participant seed finish
        vm.onAddParticipant(ContactSummary(id = 9, firstname = "Dana", lastname = "Lee"))
        assertEquals(listOf(5, 9), vm.uiState.value.participants.map { it.id })

        vm.onTitleChange("Lunch")
        vm.save()
        advanceUntilIdle()

        coVerify {
            activityRepository.create(
                match<ActivityInput> { input -> input.contactIds == listOf(5, 9) },
            )
        }
        // The count is the point: one participant cannot detect the discard.
        assertEquals(2, vm.uiState.value.participants.size)
        assertEquals(ActivityFormEvent.Saved, vm.events.value)
    }

    @Test
    fun `removing a participant drops it from the payload`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { activityRepository.create(any()) } returns Result.success(
            Activity(id = 7, title = "Lunch", contacts = listOf(ContactFlat(id = 5))),
        )

        val vm = createViewModel()
        advanceUntilIdle() // let the route-contact participant seed finish
        vm.onAddParticipant(ContactSummary(id = 9, firstname = "Dana", lastname = "Lee"))
        vm.onRemoveParticipant(9)
        assertEquals(listOf(5), vm.uiState.value.participants.map { it.id })

        vm.onTitleChange("Lunch")
        vm.save()
        advanceUntilIdle()

        coVerify {
            activityRepository.create(
                match<ActivityInput> { input -> input.contactIds == listOf(5) },
            )
        }
    }

    @Test
    fun `adding the same participant twice is a no-op`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = createViewModel()
        advanceUntilIdle() // let the route-contact participant seed finish
        vm.onAddParticipant(ContactSummary(id = 9, firstname = "Dana", lastname = "Lee"))
        vm.onAddParticipant(ContactSummary(id = 9, firstname = "Dana", lastname = "Lee"))
        assertEquals(listOf(5, 9), vm.uiState.value.participants.map { it.id })
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

        assertEquals(R.string.activity_error_date, vm.uiState.value.errorRes)
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
