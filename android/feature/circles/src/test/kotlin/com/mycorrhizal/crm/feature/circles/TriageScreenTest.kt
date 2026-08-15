package com.mycorrhizal.crm.feature.circles

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextReplacement
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

// The screen wires to hiltViewModel, which a plain Robolectric test cannot
// construct — so these tests exercise the stateful pieces through the
// stateless ClassifyContent/DoneContent composables the screen renders.
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class TriageScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun item(name: String, classification: TriageClassification = TriageClassification.CIRCLE, count: Int = 1) =
        TriageItem(original = name, name = name, classification = classification, contactCount = count)

    @Test
    fun `classify rows render the legacy name, count and classification chips`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ClassifyContent(
                    state = TriageUiState(items = listOf(item("Friends", count = 3))),
                    onSetClassification = { _, _ -> },
                    onSetName = { _, _ -> },
                    onApply = {},
                )
            }
        }

        composeTestRule.onNodeWithText("Friends").assertIsDisplayed()
        composeTestRule.onNodeWithText("3 contacts").assertIsDisplayed()
        composeTestRule.onNodeWithText("Circle").assertIsDisplayed()
        composeTestRule.onNodeWithText("Tag").assertIsDisplayed()
        composeTestRule.onNodeWithText("Skip").assertIsDisplayed()
        composeTestRule.onNodeWithText("1 circles, 0 tags, 0 skipped").assertIsDisplayed()
    }

    @Test
    fun `the apply button is disabled when everything is skipped`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ClassifyContent(
                    state = TriageUiState(items = listOf(item("Obsolete", TriageClassification.SKIP))),
                    onSetClassification = { _, _ -> },
                    onSetName = { _, _ -> },
                    onApply = {},
                )
            }
        }

        composeTestRule.onNodeWithText("0 circles, 0 tags, 1 skipped").assertIsDisplayed()
        composeTestRule.onNodeWithText("Apply").assertIsNotEnabled()
    }

    @Test
    fun `renaming a legacy string updates the item name`() {
        var items = listOf(item("Friends"))
        composeTestRule.setContent {
            MycorrhizalTheme {
                ClassifyContent(
                    state = TriageUiState(items = items),
                    onSetClassification = { _, _ -> },
                    onSetName = { index, name ->
                        items = items.mapIndexed { i, it -> if (i == index) it.copy(name = name) else it }
                    },
                    onApply = {},
                )
            }
        }

        composeTestRule.onNodeWithText("Friends").performTextReplacement("Old friends")

        assertEquals("Old friends", items[0].name)
    }

    @Test
    fun `the done state reports how many circles and tags were created`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                DoneContent(
                    state = TriageUiState(done = true, appliedCircles = 1, appliedTags = 2),
                    onBack = {},
                )
            }
        }

        composeTestRule.onNodeWithText("Triage complete").assertIsDisplayed()
        composeTestRule.onNodeWithText("Created 1 circle(s) and 2 tag(s).").assertIsDisplayed()
    }
}
