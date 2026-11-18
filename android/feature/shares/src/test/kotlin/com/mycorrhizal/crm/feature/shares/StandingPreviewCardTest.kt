package com.mycorrhizal.crm.feature.shares

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class StandingPreviewCardTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun `the preview shows an anniversary kind as a human label, never the raw token`() {
        val card = mapOf<String, Any?>(
            "name" to mapOf("full" to "Dana White"),
            "anniversaries" to mapOf(
                "b" to mapOf(
                    "kind" to "birth",
                    "date" to mapOf("year" to 1990, "month" to 12, "day" to 3)
                )
            )
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                StandingPreviewCard(
                    preview = StandingPreview.Ready(card),
                    selectedSections = setOf("anniversaries")
                )
            }
        }

        composeTestRule.onNodeWithText("Birth: 03.12.1990").assertIsDisplayed()
        composeTestRule.onNodeWithText("birth: 03.12.1990").assertDoesNotExist()
    }
}
