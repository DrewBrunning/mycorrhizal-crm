package com.mycorrhizal.crm.feature.timeline

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.data.share.ShareDraftHolder
import com.mycorrhizal.crm.domain.repository.ActivityRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.NoteRepository
import com.mycorrhizal.crm.domain.repository.ReminderRepository
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.model.network.Reminder
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.UnconfinedTestDispatcher
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

// Issue #1665 / ADR 0008-0009: each edit form's 412 prompt, driven through the real screen with a
// real ViewModel (unconfined dispatcher, so the save has already failed by first composition).
@OptIn(ExperimentalCoroutinesApi::class)
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], qualifiers = "w480dp-h2000dp")
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class FormConflictScreenTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule(UnconfinedTestDispatcher())

    @get:Rule
    val composeTestRule = createComposeRule()

    private val stale = Result.failure<Nothing>(ApiError.PreconditionFailed("stale"))

    @Test
    fun `note form shows the prompt and both buttons resolve it`() {
        val noteRepository = mockk<NoteRepository>()
        val contactRepository = mockk<ContactRepository>()
        coEvery {
            contactRepository.getContact(
                any()
            )
        } returns Result.failure(RuntimeException("offline"))
        coEvery { noteRepository.get(3) } returns Result.success(
            Note(id = 3, content = "Mine", date = "2026-08-10T14:00:00Z", revision = 1)
        )
        coEvery { noteRepository.update(3, any(), 1) } returns stale
        val vm = NoteFormViewModel(
            noteRepository,
            contactRepository,
            ShareDraftHolder(),
            SavedStateHandle(mapOf("contactId" to 5, "noteId" to 3))
        )
        vm.save()

        composeTestRule.setContent {
            MycorrhizalTheme {
                NoteFormScreen(
                    onSaved = {},
                    onBack = {},
                    viewModel = vm
                )
            }
        }

        composeTestRule.onNodeWithText("Changed elsewhere").assertIsDisplayed()
        composeTestRule.onNodeWithText("Keep editing").performClick()
        assertFalse(vm.uiState.value.conflict)
    }

    @Test
    fun `note form load latest refetches`() {
        val noteRepository = mockk<NoteRepository>()
        val contactRepository = mockk<ContactRepository>()
        coEvery {
            contactRepository.getContact(
                any()
            )
        } returns Result.failure(RuntimeException("offline"))
        coEvery { noteRepository.get(3) } returns Result.success(
            Note(id = 3, content = "Mine", date = "2026-08-10T14:00:00Z", revision = 1)
        )
        coEvery { noteRepository.update(3, any(), 1) } returns stale
        val vm = NoteFormViewModel(
            noteRepository,
            contactRepository,
            ShareDraftHolder(),
            SavedStateHandle(mapOf("contactId" to 5, "noteId" to 3))
        )
        vm.save()
        coEvery { noteRepository.get(3) } returns Result.success(
            Note(id = 3, content = "Theirs", date = "2026-08-10T14:00:00Z", revision = 2)
        )

        composeTestRule.setContent {
            MycorrhizalTheme {
                NoteFormScreen(
                    onSaved = {},
                    onBack = {},
                    viewModel = vm
                )
            }
        }
        composeTestRule.onNodeWithText("Load latest version").performClick()

        assertFalse(vm.uiState.value.conflict)
        assertTrue(vm.uiState.value.content == "Theirs")
    }

    @Test
    fun `activity form shows the prompt`() {
        val activityRepository = mockk<ActivityRepository>()
        coEvery {
            activityRepository.get(
                7
            )
        } returns Result.success(
            Activity(id = 7, title = "Lunch", date = "2026-08-10T14:00:00Z", revision = 3)
        )
        coEvery { activityRepository.update(7, any(), 3) } returns stale
        val vm = ActivityFormViewModel(
            activityRepository,
            mockk<ContactRepository>(),
            SavedStateHandle(mapOf("contactId" to 5, "activityId" to 7))
        )
        vm.save()

        composeTestRule.setContent {
            MycorrhizalTheme {
                ActivityFormScreen(
                    onSaved = {},
                    onBack = {},
                    viewModel = vm
                )
            }
        }

        composeTestRule.onNodeWithText("Changed elsewhere").assertIsDisplayed()
        composeTestRule.onNodeWithText("Keep editing").performClick()
        assertFalse(vm.uiState.value.conflict)
    }

    @Test
    fun `reminder form shows the prompt`() {
        val reminderRepository = mockk<ReminderRepository>()
        coEvery { reminderRepository.get(1) } returns Result.success(
            Reminder(id = 1, message = "Call", remindAt = "2026-08-10T14:00:00Z", revision = 6)
        )
        coEvery { reminderRepository.update(1, any(), 6) } returns stale
        val vm = ReminderFormViewModel(
            reminderRepository,
            SavedStateHandle(mapOf("contactId" to 5, "reminderId" to 1))
        )
        vm.save()

        composeTestRule.setContent {
            MycorrhizalTheme {
                ReminderFormScreen(
                    onSaved = {},
                    onBack = {},
                    viewModel = vm
                )
            }
        }

        composeTestRule.onNodeWithText("Changed elsewhere").assertIsDisplayed()
        composeTestRule.onNodeWithText("Keep editing").performClick()
        assertFalse(vm.uiState.value.conflict)
    }
}
