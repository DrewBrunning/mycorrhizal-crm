package com.mycorrhizal.crm.feature.timeline

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.Scaffold
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.network.Reminder
import com.mycorrhizal.crm.model.network.ReminderRecurrence
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
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
        onEditReminder: (Int) -> Unit = {},
        onCompleteReminder: (Int) -> Unit = {},
        onDeleteReminder: (Int) -> Unit = {},
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                Scaffold { padding ->
                    Box(modifier = Modifier.padding(padding)) {
                        when {
                            state.isLoading -> LoadingSkeleton()
                            state.reminders.isEmpty() && state.error == null ->
                                EmptyState("No reminders yet")
                            else -> {
                                LazyColumn {
                                    items(state.reminders) { reminder ->
                                        ReminderListItem(
                                            reminder = reminder,
                                            onClick = { onEditReminder(reminder.id) },
                                            onComplete = { onCompleteReminder(reminder.id) },
                                            onDelete = { onDeleteReminder(reminder.id) },
                                            isCompleting = state.completingId == reminder.id,
                                            isDeleting = state.deletingId == reminder.id,
                                        )
                                    }
                                }
                            }
                        }
                    }
                }
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
    fun `delete button invokes the delete callback`() {
        var deletedId: Int? = null
        setContent(
            RemindersUiState(
                contactId = 5,
                reminders = listOf(Reminder(id = 1, message = "Call Dana")),
            ),
            onDeleteReminder = { deletedId = it },
        )
        composeTestRule.onNodeWithContentDescription("Delete reminder").performClick()
        assertEquals(1, deletedId)
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
}
