package com.mycorrhizal.crm.feature.households

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextReplacement
import com.mycorrhizal.crm.model.network.HouseholdTypes
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/** Issue #1751: the household create/rename dialog guards its free-text name. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class HouseholdFormDialogDirtyTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(initialName: String = "", onDismiss: () -> Unit = {}) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                HouseholdFormDialog(
                    title = "New household",
                    initialName = initialName,
                    initialType = HouseholdTypes.FAMILY_UNIT,
                    confirmLabel = "Create",
                    onConfirm = { _, _ -> },
                    onDismiss = onDismiss,
                )
            }
        }
    }

    @Test
    fun `an untouched household dialog cancels immediately`() {
        var dismissed = false
        setContent(onDismiss = { dismissed = true })

        composeTestRule.onNodeWithText("Cancel").performClick()

        assertTrue(dismissed)
        composeTestRule.onNodeWithText("Discard changes?").assertDoesNotExist()
    }

    @Test
    fun `a changed name asks before cancel discards it`() {
        var dismissed = false
        setContent(onDismiss = { dismissed = true })

        composeTestRule.onNodeWithText("Name").performTextReplacement("The Smiths")
        composeTestRule.onNodeWithText("Cancel").performClick()

        composeTestRule.onNodeWithText("Discard changes?").assertIsDisplayed()
        assertEquals(false, dismissed)

        composeTestRule.onNodeWithText("Keep editing").performClick()
        composeTestRule.onNodeWithText("The Smiths").assertIsDisplayed()
        assertEquals(false, dismissed)

        composeTestRule.onNodeWithText("Cancel").performClick()
        composeTestRule.onNodeWithText("Discard").performClick()
        assertTrue(dismissed)
    }

    @Test
    fun `renaming starts clean and only a real change is dirty`() {
        var dismissed = false
        setContent(initialName = "The Smiths", onDismiss = { dismissed = true })

        composeTestRule.onNodeWithText("Cancel").performClick()
        assertTrue(dismissed)
        composeTestRule.onNodeWithText("Discard changes?").assertDoesNotExist()
    }
}
