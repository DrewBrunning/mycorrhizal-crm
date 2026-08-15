package com.mycorrhizal.crm.feature.timeline

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.ReminderRepository
import com.mycorrhizal.crm.domain.repository.SessionState
import com.mycorrhizal.crm.model.network.Reminder
import com.mycorrhizal.crm.model.network.ReminderRecurrence
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import com.mycorrhizal.crm.ui.R
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import java.time.LocalDate

class RemindersViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val reminderRepository = mockk<ReminderRepository>()
    private val authRepository = mockk<AuthRepository>()

    private fun viewModel(contactId: Int = 5): RemindersViewModel {
        every { authRepository.observeSession() } returns flowOf(SessionState())
        return RemindersViewModel(reminderRepository, authRepository, SavedStateHandle(mapOf("contactId" to contactId)))
    }

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
    fun `completing a recurring reminder shows the server-returned next occurrence, not a locally computed date`() = runTest(mainDispatcherRule.testDispatcher) {
        // M20 test case 3: the next occurrence's date must come from the server's
        // rescheduled reminder, never from a client-side computation.
        coEvery { reminderRepository.listForContact(5) } returns Result.success(
            listOf(Reminder(id = 1, message = "Call Dana", recurrence = ReminderRecurrence.WEEKLY)),
        )
        coEvery { reminderRepository.complete(1) } returns Result.success(
            Reminder(
                id = 1,
                message = "Call Dana",
                recurrence = ReminderRecurrence.WEEKLY,
                completed = false,
                remindAt = "2026-09-01T00:00:00Z",
            ),
        )

        val vm = viewModel()
        advanceUntilIdle()
        vm.complete(1)
        advanceUntilIdle()

        assertEquals("2026-09-01T00:00:00Z", vm.uiState.value.reminders[0].remindAt)
    }

    @Test
    fun `delete removes the reminder from the list on success`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { reminderRepository.listForContact(5) } returns Result.success(
            listOf(Reminder(id = 1, message = "Call Dana"), Reminder(id = 2, message = "Gift")),
        )
        coEvery { reminderRepository.delete(1) } returns Result.success(Unit)

        val vm = viewModel()
        advanceUntilIdle()
        vm.delete(1)
        advanceUntilIdle()

        assertEquals(1, vm.uiState.value.reminders.size)
        assertEquals("Gift", vm.uiState.value.reminders[0].message)
        assertNull(vm.uiState.value.deletingId)
    }

    @Test
    fun `delete failure keeps the reminder in the list`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { reminderRepository.listForContact(5) } returns Result.success(
            listOf(Reminder(id = 1, message = "Call Dana")),
        )
        coEvery { reminderRepository.delete(1) } returns Result.failure(
            ApiError.Client(404, "Reminder not found"),
        )

        val vm = viewModel()
        advanceUntilIdle()
        vm.delete(1)
        advanceUntilIdle()

        assertEquals(1, vm.uiState.value.reminders.size)
        assertEquals("Not found", vm.uiState.value.error)
        assertNull(vm.uiState.value.deletingId)
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
    fun `create mode starts with once recurrence and today's pre-filled date`() {
        val vm = createViewModel()
        val state = vm.uiState.value
        assertFalse(state.isEdit)
        assertEquals(ReminderRecurrence.ONCE, state.recurrence)
        assertEquals(5, state.contactId)
        // Web prefills getDateForRecurrence on create; M20 mirrors it.
        val today = LocalDate.now().toString()
        assertEquals("${today}T00:00:00Z", state.remindAt)
    }

    @Test
    fun `create mode starts with reoccur from completion enabled`() {
        val vm = createViewModel()
        assertTrue(vm.uiState.value.reoccurFromCompletion)
    }

    @Test
    fun `changing recurrence in create mode auto-fills the due date from the recurrence`() = runTest(mainDispatcherRule.testDispatcher) {
        // M20 test case (auto-date-from-recurrence): mirrors web's getDateForRecurrence.
        val vm = createViewModel()
        vm.onRecurrenceChange(ReminderRecurrence.WEEKLY)
        val state = vm.uiState.value
        assertEquals(ReminderRecurrence.WEEKLY, state.recurrence)
        val expected = ReminderFormState.dateForRecurrence(ReminderRecurrence.WEEKLY)
        assertEquals("${expected}T00:00:00Z", state.remindAt)
    }

    @Test
    fun `changing recurrence to once in create mode auto-fills today`() = runTest(mainDispatcherRule.testDispatcher) {
        // Web's handleRecurrenceChange sets the date for *every* recurrence, including
        // once (which resets to today). M20 matches that rather than leaving the date stale.
        val vm = createViewModel()
        vm.onRecurrenceChange(ReminderRecurrence.WEEKLY)
        vm.onRecurrenceChange(ReminderRecurrence.ONCE)
        val today = LocalDate.now().toString()
        assertEquals("${today}T00:00:00Z", vm.uiState.value.remindAt)
    }

    @Test
    fun `changing recurrence in edit mode never overwrites the existing date`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { reminderRepository.get(1) } returns Result.success(
            Reminder(id = 1, message = "Call Dana", recurrence = ReminderRecurrence.ONCE, remindAt = "2026-08-10T14:00:00Z"),
        )

        val vm = createViewModel(reminderId = 1)
        advanceUntilIdle()
        vm.onRecurrenceChange(ReminderRecurrence.MONTHLY)
        advanceUntilIdle()

        assertEquals("2026-08-10T14:00:00Z", vm.uiState.value.remindAt)
    }

    @Test
    fun `save sends reoccur from completion on the create payload`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { reminderRepository.create(5, any()) } returns Result.success(
            Reminder(id = 1, message = "Call Dana"),
        )

        val vm = createViewModel()
        vm.onMessageChange("Call Dana")
        vm.onRemindAtChange("2026-08-10T14:00:00Z")
        vm.onRecurrenceChange(ReminderRecurrence.WEEKLY)
        vm.onReoccurFromCompletionChange(false)
        vm.save()
        advanceUntilIdle()

        coVerify {
            reminderRepository.create(
                5,
                match<Reminder> { reminder -> reminder.reoccurFromCompletion == false },
            )
        }
        assertEquals(ReminderFormEvent.Saved, vm.events.value)
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
        vm.onRecurrenceChange(ReminderRecurrence.WEEKLY)
        vm.onRemindAtChange("2026-08-10T14:00:00Z")
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
