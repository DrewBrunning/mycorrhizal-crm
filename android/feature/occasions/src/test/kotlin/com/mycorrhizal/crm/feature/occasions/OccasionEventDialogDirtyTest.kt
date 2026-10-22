package com.mycorrhizal.crm.feature.occasions

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

/** Issue #1751: the occasion create/edit dialog guards free-text input. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class OccasionEventDialogDirtyTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(onDismiss: () -> Unit = {}) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                OccasionEventDialog(event = null, onDismiss = onDismiss, onSave = {})
            }
        }
    }

    @Test
    fun `an untouched occasion dialog cancels immediately`() {
        var dismissed = false
        setContent(onDismiss = { dismissed = true })

        composeTestRule.onNodeWithText("Cancel").performClick()

        assertTrue(dismissed)
        composeTestRule.onNodeWithText("Discard changes?").assertDoesNotExist()
    }

    @Test
    fun `a typed title asks before cancel discards it`() {
        var dismissed = false
        setContent(onDismiss = { dismissed = true })

        composeTestRule.onNodeWithText("Title").performTextReplacement("Birthday")
        composeTestRule.onNodeWithText("Cancel").performClick()

        composeTestRule.onNodeWithText("Discard changes?").assertIsDisplayed()
        composeTestRule.onNodeWithText("Your edits to this occasion haven't been saved.").assertIsDisplayed()
        assertEquals(false, dismissed)

        composeTestRule.onNodeWithText("Keep editing").performClick()
        composeTestRule.onNodeWithText("Birthday").assertIsDisplayed()
        assertEquals(false, dismissed)

        composeTestRule.onNodeWithText("Cancel").performClick()
        composeTestRule.onNodeWithText("Discard").performClick()
        assertTrue(dismissed)
    }
}
