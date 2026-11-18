package com.mycorrhizal.crm.ui.components

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createComposeRule
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

/** Issue #2629: the shared "Showing saved data" banner cached screens show after a failed refresh. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class SavedDataBannerTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun `shows the saved data message and a retry button that fires once`() {
        var retries = 0
        composeTestRule.setContent {
            MycorrhizalTheme { SavedDataBanner(onRetry = { retries += 1 }) }
        }

        composeTestRule.onNodeWithText("Showing saved data").assertIsDisplayed()
        composeTestRule.onNodeWithText("Retry").performClick()

        assertEquals(1, retries)
    }
}
