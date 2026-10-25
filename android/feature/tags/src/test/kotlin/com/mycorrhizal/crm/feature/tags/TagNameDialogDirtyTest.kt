package com.mycorrhizal.crm.feature.tags

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextReplacement
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/** Issue #1751: the tag create/rename dialog guards its free-text name. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class TagNameDialogDirtyTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(initial: String = "", onDismiss: () -> Unit = {}) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                TagNameDialog(
                    title = "New tag",
                    initial = initial,
                    confirmLabel = "Create",
                    onConfirm = {},
                    onDismiss = onDismiss
                )
            }
        }
    }

    @Test
    fun `an untouched tag dialog cancels immediately`() {
        var dismissed = false
        setContent(onDismiss = { dismissed = true })

        composeTestRule.onNodeWithText("Cancel").performClick()

        assertTrue(dismissed)
        composeTestRule.onNodeWithText("Discard changes?").assertDoesNotExist()
    }

    @Test
    fun `a typed tag name asks before cancel discards it`() {
        var dismissed = false
        setContent(onDismiss = { dismissed = true })

        composeTestRule.onNodeWithText("Name").performTextReplacement("climbing")
        composeTestRule.onNodeWithText("Cancel").performClick()

        composeTestRule.onNodeWithText("Discard changes?").assertIsDisplayed()
        composeTestRule.onNodeWithText(
            "Your edits to this tag haven't been saved."
        ).assertIsDisplayed()
        assertEquals(false, dismissed)

        composeTestRule.onNodeWithText("Keep editing").performClick()
        composeTestRule.onNodeWithText("climbing").assertIsDisplayed()
        assertEquals(false, dismissed)

        composeTestRule.onNodeWithText("Cancel").performClick()
        composeTestRule.onNodeWithText("Discard").performClick()
        assertTrue(dismissed)
    }
}
