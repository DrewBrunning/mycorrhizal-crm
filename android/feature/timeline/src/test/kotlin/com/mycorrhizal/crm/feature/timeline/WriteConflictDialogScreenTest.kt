package com.mycorrhizal.crm.feature.timeline

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.model.network.Reminder
import com.mycorrhizal.crm.ui.components.WriteConflictDialog
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

// Issue #1665 / ADR 0008-0009: the 412 prompt offers exactly two outcomes, no merge.
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class WriteConflictDialogScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun `list screen shows the conflict prompt and routes both buttons`() {
        var loadLatest = 0
        var dismissed = 0
        composeTestRule.setContent {
            MycorrhizalTheme {
                NotesScreenContent(
                    uiState = NotesUiState(contactId = 5, notes = listOf(Note(id = 3, content = "One")), conflict = true),
                    onConflictLoadLatest = { loadLatest++ },
                    onConflictDismiss = { dismissed++ },
                )
            }
        }

        composeTestRule.onNodeWithText("Changed elsewhere").assertIsDisplayed()
        composeTestRule.onNodeWithText("Load latest version").performClick()
        assertEquals(1, loadLatest)
        composeTestRule.onNodeWithText("Not now").performClick()
        assertEquals(1, dismissed)
    }

    @Test
    fun `no prompt without a conflict`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                NotesScreenContent(uiState = NotesUiState(contactId = 5, notes = listOf(Note(id = 3, content = "One"))))
            }
        }
        composeTestRule.onNodeWithText("Load latest version").assertDoesNotExist()
    }

    @Test
    fun `edit wording offers keep editing`() {
        var keep = 0
        composeTestRule.setContent {
            MycorrhizalTheme { WriteConflictDialog(onLoadLatest = {}, onKeep = { keep++ }) }
        }
        composeTestRule.onNodeWithText("Keep editing").performClick()
        assertEquals(1, keep)
    }

    // The wiring each list screen owes the dialog: shown only on `conflict`, the delete wording
    // (nothing was deleted), "Not now" -> dismiss and "Load latest version" -> reload.
    private fun assertConflictRouting(setScreen: (onLoadLatest: () -> Unit, onDismiss: () -> Unit) -> Unit) {
        var loadLatest = 0
        var dismissed = 0
        setScreen({ loadLatest++ }, { dismissed++ })

        composeTestRule.onNodeWithText("Changed elsewhere").assertIsDisplayed()
        composeTestRule.onNodeWithText("so it was not deleted", substring = true).assertIsDisplayed()
        composeTestRule.onNodeWithText("Load latest version").performClick()
        assertEquals(1, loadLatest)
        composeTestRule.onNodeWithText("Not now").performClick()
        assertEquals(1, dismissed)
    }

    @Test
    fun `reminders screen routes the conflict prompt`() = assertConflictRouting { latest, dismiss ->
        composeTestRule.setContent {
            MycorrhizalTheme {
                RemindersScreenContent(
                    uiState = RemindersUiState(contactId = 5, reminders = listOf(Reminder(id = 1, message = "Call")), conflict = true),
                    onConflictLoadLatest = latest,
                    onConflictDismiss = dismiss,
                )
            }
        }
    }

    @Test
    fun `reminders screen has no prompt without a conflict`() {
        composeTestRule.setContent {
            MycorrhizalTheme { RemindersScreenContent(uiState = RemindersUiState(contactId = 5)) }
        }
        composeTestRule.onNodeWithText("Changed elsewhere").assertDoesNotExist()
    }

    @Test
    fun `activities screen routes the conflict prompt`() = assertConflictRouting { latest, dismiss ->
        composeTestRule.setContent {
            MycorrhizalTheme {
                ActivitiesScreenContent(
                    uiState = ActivitiesUiState(contactId = 5, activities = listOf(Activity(id = 1, title = "Coffee")), conflict = true),
                    onConflictLoadLatest = latest,
                    onConflictDismiss = dismiss,
                )
            }
        }
    }

    @Test
    fun `activities screen has no prompt without a conflict`() {
        composeTestRule.setContent {
            MycorrhizalTheme { ActivitiesScreenContent(uiState = ActivitiesUiState(contactId = 5)) }
        }
        composeTestRule.onNodeWithText("Changed elsewhere").assertDoesNotExist()
    }

    @Test
    fun `activities inbox routes the conflict prompt`() = assertConflictRouting { latest, dismiss ->
        composeTestRule.setContent {
            MycorrhizalTheme {
                ActivitiesInboxScreenContent(
                    uiState = ActivitiesInboxUiState(activities = listOf(Activity(id = 1, title = "Coffee")), conflict = true),
                    onConflictLoadLatest = latest,
                    onConflictDismiss = dismiss,
                )
            }
        }
    }

    @Test
    fun `activities inbox has no prompt without a conflict`() {
        composeTestRule.setContent {
            MycorrhizalTheme { ActivitiesInboxScreenContent(uiState = ActivitiesInboxUiState()) }
        }
        composeTestRule.onNodeWithText("Changed elsewhere").assertDoesNotExist()
    }
}
