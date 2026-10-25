package com.mycorrhizal.crm.ui.components

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/** Issue #1734: the ambient reason wording shared by the score, cadence and prep surfaces. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class AmbientLabelsTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun `household and flag map to their own wording and anything else falls back to the flag wording`() {
        assertEquals(R.string.ambient_reason_household, ambientReasonRes("household"))
        assertEquals(R.string.ambient_reason_flag, ambientReasonRes("flag"))
        assertEquals(R.string.ambient_reason_flag, ambientReasonRes(null))
        assertEquals(R.string.ambient_reason_flag, ambientReasonRes("something-new"))
    }

    @Test
    fun `the explanation shows the reason and the explainer by default`() {
        composeTestRule.setContent { MycorrhizalTheme { AmbientExplanation(reason = "household") } }

        composeTestRule.onNodeWithText("Lives with you (household)").assertIsDisplayed()
        composeTestRule.onNodeWithText(
            "Counts as in regular contact without logging activities"
        ).assertIsDisplayed()
    }

    @Test
    fun `the explainer can be left out`() {
        composeTestRule.setContent {
            MycorrhizalTheme { AmbientExplanation(reason = "flag", showExplainer = false) }
        }

        composeTestRule.onNodeWithText("Marked as ambient").assertIsDisplayed()
        composeTestRule.onNodeWithText("Counts as in regular contact without logging activities")
            .assertDoesNotExist()
    }
}
