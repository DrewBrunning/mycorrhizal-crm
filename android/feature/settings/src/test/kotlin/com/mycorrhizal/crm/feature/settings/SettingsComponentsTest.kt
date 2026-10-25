package com.mycorrhizal.crm.feature.settings

import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.createComposeRule
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

/** Issue #1775: the shared Save button and required-marker helpers. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class SettingsComponentsTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun `requiredFieldLabel appends the marker only when required`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                androidx.compose.foundation.layout.Column {
                    androidx.compose.material3.Text(
                        requiredFieldLabel("Server URL", required = true)
                    )
                    androidx.compose.material3.Text(
                        requiredFieldLabel("Server URL", required = false)
                    )
                }
            }
        }

        composeTestRule.onNodeWithText("Server URL *").assertIsDisplayed()
        composeTestRule.onNodeWithText("Server URL").assertIsDisplayed()
    }

    @Test
    fun `an enabled save button is clickable`() {
        var saved = false
        composeTestRule.setContent {
            MycorrhizalTheme {
                SettingsSaveButton(
                    canSave = true,
                    isSaving = false,
                    onClick = { saved = true },
                    disabledHint = "Fill in the required fields"
                )
            }
        }

        composeTestRule.onNodeWithText("Save").assertIsEnabled().performClick()
        assertEquals(true, saved)
    }

    @Test
    fun `a disabled save button is visible and explains what enables it`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                SettingsSaveButton(
                    canSave = false,
                    isSaving = false,
                    onClick = {},
                    disabledHint = "Fill in the required fields"
                )
            }
        }

        composeTestRule.onNodeWithText("Save").assertIsNotEnabled()
        composeTestRule.onNodeWithText("Fill in the required fields").assertIsDisplayed()
        composeTestRule.onNodeWithText("Save")
            .assert(
                SemanticsMatcher.expectValue(
                    SemanticsProperties.StateDescription,
                    "Fill in the required fields"
                )
            )
    }

    @Test
    fun `a saving save button shows progress and announces its state`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                SettingsSaveButton(
                    canSave = true,
                    isSaving = true,
                    onClick = {},
                    disabledHint = "Fill in the required fields"
                )
            }
        }

        composeTestRule.onNodeWithText("Save").assertIsNotEnabled()
        composeTestRule.onNode(
            SemanticsMatcher.keyIsDefined(SemanticsProperties.ProgressBarRangeInfo)
        )
            .assertIsDisplayed()
        composeTestRule.onNodeWithText("Save")
            .assert(SemanticsMatcher.expectValue(SemanticsProperties.StateDescription, "Saving"))
    }
}
