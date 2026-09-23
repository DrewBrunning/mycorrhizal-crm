package com.mycorrhizal.crm.feature.settings

import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class SeafileSettingsScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun `shows the base url and api token fields`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                SeafileSettingsContent(state = SeafileSettingsUiState(isLoading = false))
            }
        }

        composeTestRule.onNodeWithText("Server URL").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("API Token").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `shows the stored-token hint when a token is already configured`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                SeafileSettingsContent(state = SeafileSettingsUiState(isLoading = false, hasApiToken = true))
            }
        }

        composeTestRule.onNodeWithText("A token is already stored. Leave the field empty to keep it.")
            .performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `the test button only appears once a token is stored`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                SeafileSettingsContent(state = SeafileSettingsUiState(isLoading = false, hasApiToken = false))
            }
        }

        composeTestRule.onNodeWithText("Test connection").assertDoesNotExist()
    }

    @Test
    fun `tapping test invokes its callback`() {
        var tested = false
        composeTestRule.setContent {
            MycorrhizalTheme {
                SeafileSettingsContent(
                    state = SeafileSettingsUiState(isLoading = false, hasApiToken = true, baseUrl = "http://seafile:8000"),
                    onTest = { tested = true },
                )
            }
        }

        composeTestRule.onNodeWithText("Test connection").performScrollTo().performClick()
        assertEquals(true, tested)
    }

    @Test
    fun `shows a test failure distinctly`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                SeafileSettingsContent(
                    state = SeafileSettingsUiState(
                        isLoading = false,
                        hasApiToken = true,
                        testResult = SeafileTestOutcome(ok = false, message = "invalid API token"),
                    ),
                )
            }
        }

        composeTestRule.onNodeWithText("Test failed: invalid API token").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `remove connection only appears once a token is stored`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                SeafileSettingsContent(state = SeafileSettingsUiState(isLoading = false, hasApiToken = false))
            }
        }

        composeTestRule.onNodeWithText("Remove connection").assertDoesNotExist()
    }

    @Test
    fun `tapping remove connection invokes its callback`() {
        var removed = false
        composeTestRule.setContent {
            MycorrhizalTheme {
                SeafileSettingsContent(
                    state = SeafileSettingsUiState(isLoading = false, hasApiToken = true),
                    onRemove = { removed = true },
                )
            }
        }

        composeTestRule.onNodeWithText("Remove connection").performScrollTo().performClick()
        assertEquals(true, removed)
    }

    @Test
    fun `save is disabled until a base url is entered`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                SeafileSettingsContent(state = SeafileSettingsUiState(isLoading = false, baseUrl = ""))
            }
        }

        composeTestRule.onNodeWithText("Save").performScrollTo().assertIsNotEnabled()
    }

    @Test
    fun `save error is announced as an assertive live region`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                SeafileSettingsContent(state = SeafileSettingsUiState(isLoading = false, saveError = "Could not save"))
            }
        }

        composeTestRule.onNodeWithText("Could not save")
            .assert(SemanticsMatcher.expectValue(SemanticsProperties.LiveRegion, LiveRegionMode.Assertive))
    }
}
