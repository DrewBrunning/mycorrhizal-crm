package com.mycorrhizal.crm.feature.timeline

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.data.share.ShareDraftHolder
import com.mycorrhizal.crm.domain.repository.ActivityRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.NoteRepository
import com.mycorrhizal.crm.domain.repository.ReminderRepository
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.ContactFlat
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.model.network.Reminder
import com.mycorrhizal.crm.model.network.ReminderRecurrence
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

/** Issue #1751: isDirty on the note/activity/reminder form states. */
class FormDirtyStateTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val noteRepository = mockk<NoteRepository>()
    private val contactRepository = mockk<ContactRepository>()
    private val shareDrafts = ShareDraftHolder()

    private fun noteViewModel(noteId: Int? = null, prefill: String? = null): NoteFormViewModel {
        coEvery { contactRepository.getContact(any()) } returns Result.failure(RuntimeException("offline"))
        val handle = SavedStateHandle(mapOf<String, Any?>("contactId" to 5).toMutableMap().apply {
            if (noteId != null) put("noteId", noteId)
            if (prefill != null) put("prefill", prefill)
        })
        return NoteFormViewModel(noteRepository, contactRepository, shareDrafts, handle)
    }

    @Test
    fun `a new note starts clean, becomes dirty when typed, and clean again when reverted`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = noteViewModel()
        advanceUntilIdle()
        assertFalse(vm.uiState.value.isDirty)

        vm.onContentChange("Loves climbing")
        assertTrue(vm.uiState.value.isDirty)

        vm.onContentChange("")
        assertFalse(vm.uiState.value.isDirty)
    }

    @Test
    fun `a shared prefill is part of the starting state, and a later edit is dirty`() = runTest(mainDispatcherRule.testDispatcher) {
        val key = shareDrafts.put("shared snippet")
        val vm = noteViewModel(prefill = key)
        advanceUntilIdle()
        assertFalse(vm.uiState.value.isDirty)

        vm.onContentChange("shared snippet plus more")
        assertTrue(vm.uiState.value.isDirty)
    }

    @Test
    fun `a loaded note edit starts clean`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { noteRepository.get(9) } returns Result.success(
            Note(id = 9, content = "Loves climbing", date = "2026-08-10T14:00:00Z", contactId = 5, revision = 1),
        )
        val vm = noteViewModel(noteId = 9)
        advanceUntilIdle()

        assertFalse(vm.uiState.value.isDirty)

        vm.onContentChange("Loves bouldering")
        assertTrue(vm.uiState.value.isDirty)
    }

    private val activityRepository = mockk<ActivityRepository>()

    private fun activityViewModel(activityId: Int? = null): ActivityFormViewModel {
        coEvery { contactRepository.getContact(any()) } returns Result.failure(RuntimeException("offline"))
        val handle = if (activityId == null) {
            SavedStateHandle(mapOf("contactId" to 5))
        } else {
            SavedStateHandle(mapOf("contactId" to 5, "activityId" to activityId))
        }
        return ActivityFormViewModel(activityRepository, contactRepository, handle)
    }

    @Test
    fun `a new activity starts clean after the route contact resolves`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = activityViewModel()
        advanceUntilIdle()

        assertFalse(vm.uiState.value.isDirty)

        vm.onTitleChange("Coffee")
        assertTrue(vm.uiState.value.isDirty)

        vm.onTitleChange("")
        assertFalse(vm.uiState.value.isDirty)
    }

    @Test
    fun `adding and removing a participant marks an activity dirty and clean`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = activityViewModel()
        advanceUntilIdle()
        assertFalse(vm.uiState.value.isDirty)

        vm.onAddParticipant(ContactSummary(id = 9, firstname = "Dana"))
        assertTrue(vm.uiState.value.isDirty)

        vm.onRemoveParticipant(9)
        assertFalse(vm.uiState.value.isDirty)
    }

    @Test
    fun `a loaded activity edit starts clean`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { activityRepository.get(11) } returns Result.success(
            Activity(
                id = 11,
                title = "Coffee",
                date = "2026-08-10T14:00:00Z",
                contacts = listOf(ContactFlat(id = 5, firstname = "Dana")),
                revision = 1,
            ),
        )
        val vm = activityViewModel(activityId = 11)
        advanceUntilIdle()

        assertFalse(vm.uiState.value.isDirty)

        vm.onLocationChange("Cafe")
        assertTrue(vm.uiState.value.isDirty)
    }

    private val reminderRepository = mockk<ReminderRepository>()

    private fun reminderViewModel(reminderId: Int? = null): ReminderFormViewModel {
        val handle = if (reminderId == null) {
            SavedStateHandle(mapOf("contactId" to 5))
        } else {
            SavedStateHandle(mapOf("contactId" to 5, "reminderId" to reminderId))
        }
        return ReminderFormViewModel(reminderRepository, handle)
    }

    @Test
    fun `a new reminder starts clean and becomes dirty on an edit`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = reminderViewModel()
        assertFalse(vm.uiState.value.isDirty)

        vm.onRecurrenceChange(ReminderRecurrence.WEEKLY)
        assertTrue(vm.uiState.value.isDirty)

        vm.onRecurrenceChange(ReminderRecurrence.ONCE)
        assertFalse(vm.uiState.value.isDirty)
    }

    @Test
    fun `a loaded reminder edit starts clean`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { reminderRepository.get(7) } returns Result.success(
            Reminder(
                id = 7,
                message = "Call",
                remindAt = "2026-08-10T14:00:00Z",
                recurrence = ReminderRecurrence.ONCE,
                byMail = false,
                reoccurFromCompletion = true,
                contactId = 5,
                revision = 1,
            ),
        )
        val vm = reminderViewModel(reminderId = 7)
        advanceUntilIdle()

        assertFalse(vm.uiState.value.isDirty)

        vm.onMessageChange("Call Dana")
        assertTrue(vm.uiState.value.isDirty)
    }
}
