package com.mycorrhizal.crm.ui.components

import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/** Issue #1773: the shared labelled select and the pinned save bar's disabled hint. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class LabelledDropdownTest {
    @get:Rule
    val composeTestRule = createComposeRule()

    private val options = listOf(
        DropdownOption("a", "Alpha", sectionHeader = "Letters"),
        DropdownOption("b", "Beta"),
    )

    @Test
    fun `shows its label and value, opens on tap, and reports the chosen option`() {
        var picked: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                LabelledDropdown(
                    label = "Pick",
                    valueText = "Alpha",
                    options = options,
                    onSelect = { picked = it },
                    modifier = Modifier.testTag("dd"),
                )
            }
        }
        composeTestRule.onNodeWithText("Pick").assertIsDisplayed()
        composeTestRule.onNodeWithTag("dd").assert(
            SemanticsMatcher.expectValue(SemanticsProperties.ContentDescription, listOf("Pick, Alpha")),
        )
        composeTestRule.onNodeWithText("Beta").assertDoesNotExist()
        composeTestRule.onNodeWithTag("dd").performClick()
        composeTestRule.onNodeWithText("Letters").assertIsDisplayed()
        composeTestRule.onNodeWithText("Beta").performClick()
        assertEquals("b", picked)
        composeTestRule.onNodeWithText("Letters").assertDoesNotExist()
    }

    @Test
    fun `a disabled dropdown does not open`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                LabelledDropdown(
                    label = "Pick",
                    valueText = "Alpha",
                    options = options,
                    onSelect = {},
                    enabled = false,
                    modifier = Modifier.testTag("dd"),
                )
            }
        }
        composeTestRule.onNodeWithTag("dd").assertIsNotEnabled()
    }

    @Test
    fun `a disabled save bar shows what enables it and does not fire`() {
        var clicks = 0
        composeTestRule.setContent {
            MycorrhizalTheme {
                PinnedSaveBar(
                    label = "Create",
                    isSaving = false,
                    onClick = { clicks++ },
                    enabled = false,
                    disabledHint = "Enter a name to continue",
                )
            }
        }
        composeTestRule.onNodeWithText("Enter a name to continue").assertIsDisplayed()
        composeTestRule.onNodeWithText("Create").assertIsNotEnabled()
        composeTestRule.onNodeWithText("Create").performClick()
        assertEquals(0, clicks)
    }
}
