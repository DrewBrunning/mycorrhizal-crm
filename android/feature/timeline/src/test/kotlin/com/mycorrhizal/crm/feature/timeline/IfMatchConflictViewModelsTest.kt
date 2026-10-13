package com.mycorrhizal.crm.feature.timeline

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.ActivityRepository
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.data.share.ShareDraftHolder
import com.mycorrhizal.crm.domain.repository.ContactActivitiesPage
import com.mycorrhizal.crm.domain.repository.ContactNotesPage
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.NoteRepository
import com.mycorrhizal.crm.domain.repository.ReminderRepository
import com.mycorrhizal.crm.domain.repository.SessionState
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.model.network.Reminder
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
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

/*
 * Issue #1665 / ADR 0008-0009: the timeline entities (notes, activities, reminders) send the
 * revision the record carried when the edit/delete began as `If-Match`, and a 412 is a typed
 * conflict state — never an error toast, never an automatic merge or silent retry.
 */

private val STALE = Result.failure<Nothing>(ApiError.PreconditionFailed("stale"))

class NoteIfMatchTest {
    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val noteRepository = mockk<NoteRepository>()
    private val contactRepository = mockk<ContactRepository>()

    private fun form(): NoteFormViewModel {
        coEvery { contactRepository.getContact(any()) } returns Result.failure(RuntimeException("offline"))
        return NoteFormViewModel(
            noteRepository,
            contactRepository,
            ShareDraftHolder(),
            SavedStateHandle(mapOf("contactId" to 5, "noteId" to 3)),
        )
    }

    private fun list() = NotesViewModel(noteRepository, SavedStateHandle(mapOf("contactId" to 5)))

    private fun stubGet(revision: Int) = coEvery { noteRepository.get(3) } returns Result.success(
        Note(id = 3, content = "Loves climbing", date = "2026-08-10T14:00:00Z", revision = revision),
    )

    @Test
    fun `edit save sends the loaded revision`() = runTest(mainDispatcherRule.testDispatcher) {
        stubGet(4)
        coEvery { noteRepository.update(3, any(), 4) } returns Result.success(Note(id = 3, revision = 5))
        val vm = form()
        advanceUntilIdle()
        vm.save()
        advanceUntilIdle()
        coVerify(exactly = 1) { noteRepository.update(3, any(), 4) }
        assertEquals(NoteFormEvent.Saved, vm.events.value)
    }

    @Test
    fun `edit 412 raises conflict, keeps the typed edit, then load latest picks up the new revision`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubGet(4)
            coEvery { noteRepository.update(3, any(), 4) } returns STALE
            coEvery { noteRepository.update(3, any(), 6) } returns Result.success(Note(id = 3, revision = 7))
            val vm = form()
            advanceUntilIdle()
            vm.onContentChange("Edited")
            vm.save()
            advanceUntilIdle()

            assertTrue(vm.uiState.value.conflict)
            assertNull(vm.uiState.value.error)
            assertNull(vm.events.value)
            assertEquals("Edited", vm.uiState.value.content)

            vm.onConflictKeepEditing()
            assertFalse(vm.uiState.value.conflict)
            assertEquals("Edited", vm.uiState.value.content)

            coEvery { noteRepository.get(3) } returns Result.success(
                Note(id = 3, content = "Server copy", date = "2026-08-10T14:00:00Z", revision = 6),
            )
            vm.onConflictLoadLatest()
            advanceUntilIdle()
            assertFalse(vm.uiState.value.conflict)
            assertEquals("Server copy", vm.uiState.value.content)
            vm.save()
            advanceUntilIdle()
            coVerify(exactly = 1) { noteRepository.update(3, any(), 6) }
        }

    @Test
    fun `delete sends the row's revision and a 412 raises conflict leaving the row`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { noteRepository.listForContact(5, null, null, null, null, null) } returns Result.success(
                ContactNotesPage(notes = listOf(Note(id = 3, content = "One", revision = 2)), nextCursor = null),
            )
            coEvery { noteRepository.delete(3, 2) } returns STALE
            val vm = list()
            advanceUntilIdle()
            vm.delete(3)
            advanceUntilIdle()

            coVerify(exactly = 1) { noteRepository.delete(3, 2) }
            assertTrue(vm.uiState.value.conflict)
            assertNull(vm.uiState.value.error)
            assertNull(vm.uiState.value.deletingId)
            assertEquals(1, vm.uiState.value.notes.size)

            coEvery { noteRepository.listForContact(5, null, null, null, null, null) } returns Result.success(
                ContactNotesPage(notes = listOf(Note(id = 3, content = "One", revision = 9)), nextCursor = null),
            )
            vm.onConflictLoadLatest()
            advanceUntilIdle()
            assertFalse(vm.uiState.value.conflict)
            assertEquals(9, vm.uiState.value.notes.single().revision)

            vm.onConflictDismissed()
            assertFalse(vm.uiState.value.conflict)
        }
}

class ActivityIfMatchTest {
    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val activityRepository = mockk<ActivityRepository>()
    private val contactRepository = mockk<ContactRepository>()

    private fun form() = ActivityFormViewModel(
        activityRepository,
        contactRepository,
        SavedStateHandle(mapOf("contactId" to 5, "activityId" to 7)),
    )

    @Test
    fun `edit save sends the loaded revision and a 412 raises conflict`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { activityRepository.get(7) } returns Result.success(Activity(id = 7, title = "Lunch", revision = 3))
        coEvery { activityRepository.update(7, any(), 3) } returns STALE
        val vm = form()
        advanceUntilIdle()
        vm.onTitleChange("Dinner")
        vm.save()
        advanceUntilIdle()

        coVerify(exactly = 1) { activityRepository.update(7, any(), 3) }
        assertTrue(vm.uiState.value.conflict)
        assertNull(vm.uiState.value.error)
        assertNull(vm.events.value)
        assertEquals("Dinner", vm.uiState.value.title)

        coEvery { activityRepository.get(7) } returns Result.success(Activity(id = 7, title = "Server", revision = 5))
        vm.onConflictLoadLatest()
        advanceUntilIdle()
        assertFalse(vm.uiState.value.conflict)
        assertEquals("Server", vm.uiState.value.title)
        assertEquals(5, vm.uiState.value.revision)
    }

    @Test
    fun `delete from the contact list and the inbox send the row's revision and surface a 412`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val page = ContactActivitiesPage(
                activities = listOf(Activity(id = 1, title = "A", revision = 2)),
                nextCursor = null,
            )
            coEvery { activityRepository.listForContact(any(), any(), any(), any(), any(), any()) } returns Result.success(page)
            coEvery { activityRepository.delete(1, 2) } returns STALE
            val vm = ActivitiesViewModel(activityRepository, SavedStateHandle(mapOf("contactId" to 5)))
            advanceUntilIdle()
            vm.delete(1)
            advanceUntilIdle()
            coVerify(exactly = 1) { activityRepository.delete(1, 2) }
            assertTrue(vm.uiState.value.conflict)
            assertNull(vm.uiState.value.error)
            assertEquals(1, vm.uiState.value.activities.size)
            vm.onConflictDismissed()
            assertFalse(vm.uiState.value.conflict)
        }
}

class ReminderIfMatchTest {
    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val reminderRepository = mockk<ReminderRepository>()
    private val authRepository = mockk<AuthRepository>()

    @Test
    fun `edit save sends the loaded revision and a 412 raises conflict`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { reminderRepository.get(1) } returns Result.success(
            Reminder(id = 1, message = "Call Dana", remindAt = "2026-08-10T14:00:00Z", revision = 6),
        )
        coEvery { reminderRepository.update(1, any(), 6) } returns STALE
        val vm = ReminderFormViewModel(
            reminderRepository,
            SavedStateHandle(mapOf("contactId" to 5, "reminderId" to 1)),
        )
        advanceUntilIdle()
        vm.onMessageChange("Call Dana today")
        vm.save()
        advanceUntilIdle()

        coVerify(exactly = 1) { reminderRepository.update(1, any(), 6) }
        assertTrue(vm.uiState.value.conflict)
        assertNull(vm.uiState.value.error)
        assertEquals("Call Dana today", vm.uiState.value.message)
    }

    @Test
    fun `delete sends the row's revision and a 412 raises conflict leaving the row`() =
        runTest(mainDispatcherRule.testDispatcher) {
            every { authRepository.observeSession() } returns flowOf(SessionState())
            coEvery { reminderRepository.listForContact(5) } returns Result.success(
                listOf(Reminder(id = 1, message = "x", revision = 4)),
            )
            coEvery { reminderRepository.delete(1, 4) } returns STALE
            val vm = RemindersViewModel(reminderRepository, authRepository, SavedStateHandle(mapOf("contactId" to 5)))
            advanceUntilIdle()
            vm.delete(1)
            advanceUntilIdle()

            coVerify(exactly = 1) { reminderRepository.delete(1, 4) }
            assertTrue(vm.uiState.value.conflict)
            assertNull(vm.uiState.value.error)
            assertEquals(1, vm.uiState.value.reminders.size)
        }
}

class ConflictResolutionViewModelsTest {
    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val activityRepository = mockk<ActivityRepository>()
    private val reminderRepository = mockk<ReminderRepository>()
    private val authRepository = mockk<AuthRepository>()

    @Test
    fun `inbox delete 412 raises conflict, load latest reloads, dismiss clears`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { activityRepository.listAll() } returns Result.success(
                com.mycorrhizal.crm.model.network.ActivitiesPage(
                    activitiesRaw = listOf(Activity(id = 1, title = "A", revision = 2)),
                    nextCursor = null,
                ),
            )
            coEvery { activityRepository.delete(1, 2) } returns STALE
            val vm = ActivitiesInboxViewModel(activityRepository)
            advanceUntilIdle()
            vm.delete(1)
            advanceUntilIdle()

            coVerify(exactly = 1) { activityRepository.delete(1, 2) }
            assertTrue(vm.uiState.value.conflict)
            assertNull(vm.uiState.value.error)
            assertEquals(1, vm.uiState.value.activities.size)

            vm.onConflictLoadLatest()
            advanceUntilIdle()
            assertFalse(vm.uiState.value.conflict)
            coVerify(exactly = 2) { activityRepository.listAll() }

            vm.delete(1)
            advanceUntilIdle()
            vm.onConflictDismissed()
            assertFalse(vm.uiState.value.conflict)
        }

    @Test
    fun `activities list load latest reloads the list`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { activityRepository.listForContact(any(), any(), any(), any(), any(), any()) } returns Result.success(
            ContactActivitiesPage(activities = listOf(Activity(id = 1, title = "A", revision = 2)), nextCursor = null),
        )
        coEvery { activityRepository.delete(1, 2) } returns STALE
        val vm = ActivitiesViewModel(activityRepository, SavedStateHandle(mapOf("contactId" to 5)))
        advanceUntilIdle()
        vm.delete(1)
        advanceUntilIdle()
        vm.onConflictLoadLatest()
        advanceUntilIdle()

        assertFalse(vm.uiState.value.conflict)
        coVerify(exactly = 2) { activityRepository.listForContact(any(), any(), any(), any(), any(), any()) }
    }

    @Test
    fun `reminder form keep editing and load latest resolve the conflict`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { reminderRepository.get(1) } returns Result.success(
            Reminder(id = 1, message = "Call", remindAt = "2026-08-10T14:00:00Z", revision = 6),
        )
        coEvery { reminderRepository.update(1, any(), 6) } returns STALE
        val vm = ReminderFormViewModel(reminderRepository, SavedStateHandle(mapOf("contactId" to 5, "reminderId" to 1)))
        advanceUntilIdle()
        vm.save()
        advanceUntilIdle()
        assertTrue(vm.uiState.value.conflict)

        vm.onConflictKeepEditing()
        assertFalse(vm.uiState.value.conflict)

        vm.save()
        advanceUntilIdle()
        coEvery { reminderRepository.get(1) } returns Result.success(
            Reminder(id = 1, message = "Server", remindAt = "2026-08-10T14:00:00Z", revision = 8),
        )
        vm.onConflictLoadLatest()
        advanceUntilIdle()
        assertFalse(vm.uiState.value.conflict)
        assertEquals("Server", vm.uiState.value.message)
        assertEquals(8, vm.uiState.value.revision)
    }

    @Test
    fun `reminders list load latest reloads and dismiss clears`() = runTest(mainDispatcherRule.testDispatcher) {
        every { authRepository.observeSession() } returns flowOf(SessionState())
        coEvery { reminderRepository.listForContact(5) } returns Result.success(
            listOf(Reminder(id = 1, message = "x", revision = 4)),
        )
        coEvery { reminderRepository.delete(1, 4) } returns STALE
        val vm = RemindersViewModel(reminderRepository, authRepository, SavedStateHandle(mapOf("contactId" to 5)))
        advanceUntilIdle()
        vm.delete(1)
        advanceUntilIdle()
        assertTrue(vm.uiState.value.conflict)

        vm.onConflictLoadLatest()
        advanceUntilIdle()
        assertFalse(vm.uiState.value.conflict)
        coVerify(exactly = 2) { reminderRepository.listForContact(5) }

        vm.delete(1)
        advanceUntilIdle()
        vm.onConflictDismissed()
        assertFalse(vm.uiState.value.conflict)
    }
}
