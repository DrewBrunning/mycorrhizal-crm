package com.mycorrhizal.crm.feature.timeline

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.ReminderRepository
import com.mycorrhizal.crm.model.network.Reminder
import com.mycorrhizal.crm.model.network.ReminderRecurrence
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import com.mycorrhizal.crm.ui.R
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

class RemindersViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val reminderRepository = mockk<ReminderRepository>()

    private fun viewModel(contactId: Int = 5): RemindersViewModel =
        RemindersViewModel(reminderRepository, SavedStateHandle(mapOf("contactId" to contactId)))

    @Test
    fun `loads the contact's reminders on init`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { reminderRepository.listForContact(5) } returns Result.success(
            listOf(
                Reminder(id = 1, message = "Call Dana", recurrence = ReminderRecurrence.WEEKLY),
                Reminder(id = 2, message = "Gift", recurrence = ReminderRecurrence.ONCE),
            ),
        )

        val vm = viewModel()
        advanceUntilIdle()

        val state = vm.uiState.value
        assertFalse(state.isLoading)
        assertEquals(2, state.reminders.size)
        assertEquals("Call Dana", state.reminders[0].message)
        assertNull(state.error)
    }

    @Test
    fun `missing contact id sets an error`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = viewModel(contactId = 0)
        advanceUntilIdle()

        assertEquals(R.string.reminder_error_missing_id, vm.uiState.value.errorRes)
    }

    @Test
    fun `completing a once reminder removes it from the list`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { reminderRepository.listForContact(5) } returns Result.success(
            listOf(Reminder(id = 2, message = "Gift", recurrence = ReminderRecurrence.ONCE)),
        )
        coEvery { reminderRepository.complete(2) } returns Result.success(null)

        val vm = viewModel()
        advanceUntilIdle()
        vm.complete(2)
        advanceUntilIdle()

        assertTrue(vm.uiState.value.reminders.isEmpty())
        assertNull(vm.uiState.value.completingId)
    }

    @Test
    fun `completing a recurring reminder keeps the rescheduled occurrence`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { reminderRepository.listForContact(5) } returns Result.success(
            listOf(Reminder(id = 1, message = "Call Dana", recurrence = ReminderRecurrence.WEEKLY)),
        )
        coEvery { reminderRepository.complete(1) } returns Result.success(
            Reminder(id = 1, message = "Call Dana", recurrence = ReminderRecurrence.WEEKLY, completed = false),
        )

        val vm = viewModel()
        advanceUntilIdle()
        vm.complete(1)
        advanceUntilIdle()

        assertEquals(1, vm.uiState.value.reminders.size)
        assertEquals("Call Dana", vm.uiState.value.reminders[0].message)
    }

    @Test
    fun `complete failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { reminderRepository.listForContact(5) } returns Result.success(
            listOf(Reminder(id = 1, message = "Call Dana")),
        )
        coEvery { reminderRepository.complete(1) } returns Result.failure(
            ApiError.Client(404, "Reminder not found"),
        )

        val vm = viewModel()
        advanceUntilIdle()
        vm.complete(1)
        advanceUntilIdle()

        assertEquals("Not found", vm.uiState.value.error)
        assertNull(vm.uiState.value.completingId)
        assertEquals(1, vm.uiState.value.reminders.size)
    }
}

class ReminderFormViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val reminderRepository = mockk<ReminderRepository>()

    private fun createViewModel(contactId: Int = 5, reminderId: Int? = null): ReminderFormViewModel {
        val handle = SavedStateHandle(mapOf("contactId" to contactId).toMutableMap().apply {
            if (reminderId != null) put("reminderId", reminderId)
        })
        return ReminderFormViewModel(reminderRepository, handle)
    }

    @Test
    fun `create mode starts with once recurrence`() {
        val vm = createViewModel()
        val state = vm.uiState.value
        assertFalse(state.isEdit)
        assertEquals(ReminderRecurrence.ONCE, state.recurrence)
        assertEquals(5, state.contactId)
    }

    @Test
    fun `save without message or date is blocked`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = createViewModel()
        vm.save()
        advanceUntilIdle()

        assertEquals(R.string.reminder_error_message, vm.uiState.value.errorRes)
        coVerify(exactly = 0) { reminderRepository.create(any(), any()) }
    }

    @Test
    fun `save in create mode calls create with the reminder fields`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { reminderRepository.create(5, any()) } returns Result.success(
            Reminder(id = 1, message = "Call Dana"),
        )

        val vm = createViewModel()
        vm.onMessageChange("Call Dana")
        vm.onRemindAtChange("2026-08-10T14:00:00Z")
        vm.onRecurrenceChange(ReminderRecurrence.WEEKLY)
        vm.save()
        advanceUntilIdle()

        coVerify {
            reminderRepository.create(
                5,
                match<Reminder> { reminder ->
                    reminder.message == "Call Dana" &&
                        reminder.remindAt == "2026-08-10T14:00:00Z" &&
                        reminder.recurrence == ReminderRecurrence.WEEKLY &&
                        reminder.contactId == 5
                },
            )
        }
        assertEquals(ReminderFormEvent.Saved, vm.events.value)
    }

    @Test
    fun `save in edit mode hydrates and calls update`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { reminderRepository.get(1) } returns Result.success(
            Reminder(id = 1, message = "Call Dana", recurrence = ReminderRecurrence.WEEKLY, remindAt = "2026-08-10T14:00:00Z"),
        )
        coEvery { reminderRepository.update(1, any()) } returns Result.success(Reminder(id = 1, message = "Call Dana"))

        val vm = createViewModel(reminderId = 1)
        advanceUntilIdle()

        assertEquals("Call Dana", vm.uiState.value.message)
        assertEquals(ReminderRecurrence.WEEKLY, vm.uiState.value.recurrence)

        vm.onMessageChange("Call Dana today")
        vm.save()
        advanceUntilIdle()

        coVerify {
            reminderRepository.update(
                1,
                match<Reminder> { reminder -> reminder.message == "Call Dana today" },
            )
        }
        assertEquals(ReminderFormEvent.Saved, vm.events.value)
    }

    @Test
    fun `invalid date format blocks save`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = createViewModel()
        vm.onMessageChange("Call Dana")
        vm.onRemindAtChange("2026-08-10")
        vm.save()
        advanceUntilIdle()

        assertEquals(R.string.reminder_error_remind_at, vm.uiState.value.errorRes)
        coVerify(exactly = 0) { reminderRepository.create(any(), any()) }
    }

    @Test
    fun `edit hydration failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { reminderRepository.get(1) } returns Result.failure(
            ApiError.Client(404, "Reminder not found"),
        )

        val vm = createViewModel(reminderId = 1)
        advanceUntilIdle()

        assertEquals("Not found", vm.uiState.value.error)
        assertFalse(vm.uiState.value.isLoading)
    }
}
