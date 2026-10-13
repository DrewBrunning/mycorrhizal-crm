package com.mycorrhizal.crm.feature.timeline

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.network.Note
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
}
