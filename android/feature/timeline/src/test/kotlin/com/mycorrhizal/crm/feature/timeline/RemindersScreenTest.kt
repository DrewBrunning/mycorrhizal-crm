package com.mycorrhizal.crm.feature.timeline

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.network.Reminder
import com.mycorrhizal.crm.model.network.ReminderRecurrence
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import java.time.LocalDate

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class RemindersScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(
        state: RemindersUiState,
        onBack: () -> Unit = {},
        onCreateReminder: () -> Unit = {},
        onEditReminder: (Int) -> Unit = {},
        onCompleteReminder: (Int) -> Unit = {},
        onDeleteReminder: (Int) -> Unit = {},
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                RemindersScreenContent(
                    uiState = state,
                    onBack = onBack,
                    onCreateReminder = onCreateReminder,
                    onEditReminder = onEditReminder,
                    onComplete = onCompleteReminder,
                    onDelete = onDeleteReminder,
                )
            }
        }
    }

    @Test
    fun `shows empty state when no reminders`() {
        setContent(RemindersUiState(contactId = 5, reminders = emptyList()))
        composeTestRule.onNodeWithText("No reminders yet").assertIsDisplayed()
    }

    @Test
    fun `shows reminder list items with recurrence`() {
        setContent(
            RemindersUiState(
                contactId = 5,
                reminders = listOf(
                    Reminder(id = 1, message = "Call Dana", recurrence = ReminderRecurrence.WEEKLY),
                    Reminder(id = 2, message = "Gift", recurrence = ReminderRecurrence.ONCE),
                ),
            ),
        )
        composeTestRule.onNodeWithText("Call Dana").assertIsDisplayed()
        composeTestRule.onNodeWithText("Gift").assertIsDisplayed()
        composeTestRule.onNodeWithText("Weekly").assertIsDisplayed()
    }

    @Test
    fun `tapping a reminder invokes the edit callback`() {
        var editedId: Int? = null
        setContent(
            RemindersUiState(
                contactId = 5,
                reminders = listOf(Reminder(id = 1, message = "Call Dana")),
            ),
            onEditReminder = { editedId = it },
        )
        composeTestRule.onNodeWithText("Call Dana").performClick()
        assertEquals(1, editedId)
    }

    @Test
    fun `complete button invokes the complete callback`() {
        var completedId: Int? = null
        setContent(
            RemindersUiState(
                contactId = 5,
                reminders = listOf(Reminder(id = 1, message = "Call Dana")),
            ),
            onCompleteReminder = { completedId = it },
        )
        composeTestRule.onNodeWithContentDescription("Complete reminder").performClick()
        assertEquals(1, completedId)
    }

    @Test
    fun `delete asks first -- tapping delete shows a confirmation and does not call onDelete`() {
        var deletedId: Int? = null
        setContent(
            RemindersUiState(
                contactId = 5,
                reminders = listOf(Reminder(id = 7, message = "Call Dana")),
            ),
            onDeleteReminder = { deletedId = it },
        )
        composeTestRule.onNodeWithContentDescription("Delete reminder").performClick()

        composeTestRule.onNodeWithText("Delete reminder?").assertIsDisplayed()
        assertNull(deletedId)
    }

    @Test
    fun `cancel is inert -- dismissing the confirmation issues no call and leaves the item present`() {
        var deletedId: Int? = null
        setContent(
            RemindersUiState(
                contactId = 5,
                reminders = listOf(Reminder(id = 7, message = "Call Dana")),
            ),
            onDeleteReminder = { deletedId = it },
        )
        composeTestRule.onNodeWithContentDescription("Delete reminder").performClick()
        composeTestRule.onNodeWithText("Cancel").performClick()

        assertNull(deletedId)
        composeTestRule.onNodeWithText("Call Dana").assertIsDisplayed()
    }

    @Test
    fun `confirming the dialog calls onDelete with the right id`() {
        var deletedId: Int? = null
        setContent(
            RemindersUiState(
                contactId = 5,
                reminders = listOf(Reminder(id = 7, message = "Call Dana")),
            ),
            onDeleteReminder = { deletedId = it },
        )
        composeTestRule.onNodeWithContentDescription("Delete reminder").performClick()
        composeTestRule.onNodeWithText("Delete").performClick()

        assertEquals(7, deletedId)
    }

    @Test
    fun `a reminder due before today is overdue and shows the overdue chip`() {
        val yesterday = LocalDate.now().minusDays(1).toString()
        setContent(
            RemindersUiState(
                contactId = 5,
                reminders = listOf(
                    Reminder(id = 1, message = "Call Dana", remindAt = "${yesterday}T00:00:00Z"),
                ),
            ),
        )
        composeTestRule.onNodeWithText("Overdue").assertIsDisplayed()
    }

    @Test
    fun `a reminder due today is not overdue`() {
        // M20 test case 2: due today is NOT styled overdue — the off-by-one worth pinning.
        val today = LocalDate.now().toString()
        setContent(
            RemindersUiState(
                contactId = 5,
                reminders = listOf(
                    Reminder(id = 1, message = "Call Dana", remindAt = "${today}T00:00:00Z"),
                ),
            ),
        )
        assertTrue(composeTestRule.onAllNodesWithText("Overdue").fetchSemanticsNodes().isEmpty())
    }

    @Test
    fun `by-mail reminder shows the email chip`() {
        setContent(
            RemindersUiState(
                contactId = 5,
                reminders = listOf(
                    Reminder(id = 1, message = "Call Dana", byMail = true, recurrence = ReminderRecurrence.ONCE),
                ),
            ),
        )
        composeTestRule.onNodeWithText("Email").assertIsDisplayed()
    }

    @Test
    fun `reoccur-from-completion recurring reminder shows the flexible chip`() {
        setContent(
            RemindersUiState(
                contactId = 5,
                reminders = listOf(
                    Reminder(
                        id = 1,
                        message = "Call Dana",
                        recurrence = ReminderRecurrence.WEEKLY,
                        reoccurFromCompletion = true,
                    ),
                ),
            ),
        )
        composeTestRule.onNodeWithText("Flexible").assertIsDisplayed()
    }

    @Test
    fun `once reminder with reoccur-from-completion does not show the flexible chip`() {
        setContent(
            RemindersUiState(
                contactId = 5,
                reminders = listOf(
                    Reminder(
                        id = 1,
                        message = "Call Dana",
                        recurrence = ReminderRecurrence.ONCE,
                        reoccurFromCompletion = true,
                    ),
                ),
            ),
        )
        assertTrue(composeTestRule.onAllNodesWithText("Flexible").fetchSemanticsNodes().isEmpty())
    }

    @Test
    fun `shows the formatted due date chip`() {
        setContent(
            RemindersUiState(
                contactId = 5,
                reminders = listOf(
                    Reminder(id = 1, message = "Call Dana", remindAt = "2026-08-10T00:00:00Z"),
                ),
            ),
            // Default EU format renders "10 August 2026".
        )
        composeTestRule.onNodeWithText("10 August 2026").assertIsDisplayed()
    }
}
