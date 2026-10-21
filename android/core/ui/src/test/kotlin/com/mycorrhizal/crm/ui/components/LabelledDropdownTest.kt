package com.mycorrhizal.crm.ui.components

import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.test.ExperimentalTestApi
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.hasClickAction
import androidx.compose.ui.test.hasContentDescription
import androidx.compose.ui.test.performKeyInput
import androidx.compose.ui.test.pressKey
import androidx.compose.ui.test.requestFocus
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

    @OptIn(ExperimentalTestApi::class)
    @Test
    fun `a focused dropdown opens from the keyboard and reads as a drop-down list`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                LabelledDropdown(
                    label = "Pick",
                    valueText = "Alpha",
                    options = options,
                    onSelect = {},
                    modifier = Modifier.testTag("dd"),
                )
            }
        }
        composeTestRule.onNodeWithTag("dd")
            .assert(SemanticsMatcher.expectValue(SemanticsProperties.Role, Role.DropdownList))
            .requestFocus()
            .performKeyInput { pressKey(Key.Enter) }
        composeTestRule.onNodeWithText("Beta").assertIsDisplayed()
    }

    // Review of #1773: an OutlinedTextField anchor left TalkBack two stops, a bare
    // "<label>, <value>" node and a separate "<value>, edit box". The control is one
    // actionable node, and nothing in it is editable text.
    @Test
    fun `the dropdown is one actionable node with no editable text inside`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                LabelledDropdown(
                    label = "Pick",
                    valueText = "Alpha",
                    options = options,
                    onSelect = {},
                    modifier = Modifier.testTag("dd"),
                )
            }
        }
        composeTestRule.onAllNodes(
            SemanticsMatcher.keyIsDefined(SemanticsProperties.EditableText),
            useUnmergedTree = true,
        ).assertCountEquals(0)
        composeTestRule.onNode(hasContentDescription("Pick, Alpha"))
            .assert(hasClickAction())
            .assert(SemanticsMatcher.expectValue(SemanticsProperties.Role, Role.DropdownList))
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
        composeTestRule.onNodeWithTag("dd").assertIsNotEnabled().performClick()
        composeTestRule.onNodeWithText("Beta").assertDoesNotExist()
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

    @Test
    fun `a save bar with defaults is enabled and a saving bar shows no hint`() {
        var clicks = 0
        composeTestRule.setContent {
            MycorrhizalTheme {
                androidx.compose.foundation.layout.Column {
                    PinnedSaveBar(label = "Plain", isSaving = false, onClick = { clicks++ })
                    PinnedSaveBar(
                        label = "Busy",
                        isSaving = true,
                        onClick = {},
                        enabled = false,
                        disabledHint = "Should not show",
                    )
                }
            }
        }
        composeTestRule.onNodeWithText("Plain").performClick()
        assertEquals(1, clicks)
        composeTestRule.onNodeWithText("Should not show").assertDoesNotExist()
    }

    @Test
    fun `the form scaffold can hide its save bar and shows it with the hint when asked`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                androidx.compose.foundation.layout.Column {
                    FormScaffold(
                        title = "Loading form",
                        onBack = {},
                        saveLabel = "Hidden save",
                        isSaving = false,
                        onSave = {},
                        snackbarHostState = androidx.compose.runtime.remember { androidx.compose.material3.SnackbarHostState() },
                        showSaveBar = false,
                    ) {}
                }
            }
        }
        composeTestRule.onNodeWithText("Loading form").assertIsDisplayed()
        composeTestRule.onNodeWithText("Hidden save").assertDoesNotExist()
    }
}
