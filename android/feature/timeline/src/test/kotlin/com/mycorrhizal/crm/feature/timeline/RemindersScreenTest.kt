package com.mycorrhizal.crm.feature.timeline

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.Scaffold
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.network.Reminder
import com.mycorrhizal.crm.model.network.ReminderRecurrence
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

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
                                            isCompleting = state.completingId == reminder.id,
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
        composeTestRule.onNodeWithText("weekly").assertIsDisplayed()
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
}
