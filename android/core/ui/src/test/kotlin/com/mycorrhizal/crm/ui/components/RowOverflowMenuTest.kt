package com.mycorrhizal.crm.ui.components

import androidx.compose.foundation.layout.Column
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issue #1760: a row's secondary actions sit behind one overflow button whose
 * TalkBack label names the row, so two rows never announce the same thing.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class RowOverflowMenuTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun `actions stay hidden until the overflow button is tapped`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                RowOverflowMenu(rowName = "Family", actions = listOf(RowAction("Rename", onClick = {})))
            }
        }
        composeTestRule.onNodeWithText("Rename").assertDoesNotExist()
        composeTestRule.onNodeWithContentDescription("More actions for Family").performClick()
        composeTestRule.onNodeWithText("Rename").assertIsDisplayed()
    }

    @Test
    fun `choosing an action runs it once and closes the menu`() {
        val log = mutableListOf<String>()
        composeTestRule.setContent {
            MycorrhizalTheme {
                RowOverflowMenu(
                    rowName = "Family",
                    actions = listOf(
                        RowAction("Rename", onClick = { log += "rename" }),
                        RowAction("Delete", onClick = { log += "delete" }, destructive = true),
                    ),
                )
            }
        }
        composeTestRule.onNodeWithContentDescription("More actions for Family").performClick()
        composeTestRule.onNodeWithText("Delete").performClick()
        assertEquals(listOf("delete"), log)
        composeTestRule.onNodeWithText("Rename").assertDoesNotExist()
    }

    @Test
    fun `each row's overflow button names its own row`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                Column {
                    RowOverflowMenu(rowName = "Family", actions = listOf(RowAction("Rename", onClick = {})))
                    RowOverflowMenu(rowName = "Friends", actions = listOf(RowAction("Rename", onClick = {})))
                }
            }
        }
        composeTestRule.onNodeWithContentDescription("More actions for Family").assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("More actions for Friends").assertIsDisplayed()
    }
}
