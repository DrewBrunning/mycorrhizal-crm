package com.mycorrhizal.crm.ui.components

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/** Issue #2629: the one fatal-error state every network-only screen shows, with Retry. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ErrorStateTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun `names an unreachable server and Retry fires once`() {
        var retries = 0
        composeTestRule.setContent {
            MycorrhizalTheme {
                ErrorState(
                    error = ApiError.Network(java.io.IOException("offline")),
                    onRetry = { retries += 1 }
                )
            }
        }

        composeTestRule.onNodeWithText(
            "Can't reach your server. Check your connection and try again."
        ).assertIsDisplayed()
        composeTestRule.onNodeWithText("Retry").performClick()

        assertEquals(1, retries)
    }

    @Test
    @Config(qualifiers = "de")
    fun `is worded in the UI language`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ErrorState(error = ApiError.Client(404, "gone"), onRetry = {})
            }
        }

        composeTestRule.onNodeWithText("Nicht gefunden").assertIsDisplayed()
    }
}
