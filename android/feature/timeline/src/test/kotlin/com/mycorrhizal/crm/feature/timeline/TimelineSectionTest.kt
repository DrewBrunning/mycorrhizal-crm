package com.mycorrhizal.crm.feature.timeline

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.model.network.Reminder
import com.mycorrhizal.crm.model.network.ReminderRecurrence
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
class TimelineSectionTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun `shows empty message when no items`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                TimelineSection(
                    items = emptyList(),
                    onEditActivity = {},
                    onEditNote = {},
                    onEditReminder = {},
                    onCompleteReminder = {},
                )
            }
        }
        composeTestRule.onNodeWithText("No timeline entries yet").assertIsDisplayed()
    }

    @Test
    fun `renders activities notes and reminders`() {
        val items = listOf(
            TimelineItem.ActivityItem(Activity(id = 1, title = "Coffee with Dana", type = "visit")),
            TimelineItem.NoteItem(Note(id = 2, content = "Loves climbing")),
            TimelineItem.ReminderItem(
                Reminder(id = 3, message = "Call Dana", recurrence = ReminderRecurrence.WEEKLY),
            ),
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                TimelineSection(
                    items = items,
                    onEditActivity = {},
                    onEditNote = {},
                    onEditReminder = {},
                    onCompleteReminder = {},
                )
            }
        }
        composeTestRule.onNodeWithText("Coffee with Dana").assertIsDisplayed()
        composeTestRule.onNodeWithText("Loves climbing").assertIsDisplayed()
        composeTestRule.onNodeWithText("Call Dana").assertIsDisplayed()
    }

    @Test
    fun `tapping an activity routes to edit`() {
        var edited: Int? = null
        val items = listOf(TimelineItem.ActivityItem(Activity(id = 7, title = "Coffee")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                TimelineSection(
                    items = items,
                    onEditActivity = { edited = it },
                    onEditNote = {},
                    onEditReminder = {},
                    onCompleteReminder = {},
                )
            }
        }
        composeTestRule.onNodeWithText("Coffee").performClick()
        assertEquals(7, edited)
    }

    @Test
    fun `reminder complete button invokes the callback`() {
        var completed: Int? = null
        val items = listOf(
            TimelineItem.ReminderItem(
                Reminder(id = 3, message = "Call Dana", recurrence = ReminderRecurrence.ONCE),
            ),
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                TimelineSection(
                    items = items,
                    onEditActivity = {},
                    onEditNote = {},
                    onEditReminder = {},
                    onCompleteReminder = { completed = it },
                )
            }
        }
        composeTestRule.onNodeWithContentDescription("Complete reminder").performClick()
        assertEquals(3, completed)
    }
}
