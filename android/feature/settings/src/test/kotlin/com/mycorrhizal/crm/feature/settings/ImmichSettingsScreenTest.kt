package com.mycorrhizal.crm.feature.settings

import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsOff
import androidx.compose.ui.test.assertIsToggleable
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
class ImmichSettingsScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun `shows the base url and api key fields`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ImmichSettingsContent(state = ImmichSettingsUiState(isLoading = false))
            }
        }

        composeTestRule.onNodeWithText("Immich server URL").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("API key").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `shows the stored-key hint when a key is already configured`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ImmichSettingsContent(state = ImmichSettingsUiState(isLoading = false, hasApiKey = true))
            }
        }

        composeTestRule.onNodeWithText("A key is already stored. Leave the field empty to keep it.")
            .performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `the sync toggle and test button only appear once a key is stored`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ImmichSettingsContent(state = ImmichSettingsUiState(isLoading = false, hasApiKey = false))
            }
        }

        composeTestRule.onNodeWithText("Sync automatically").assertDoesNotExist()
        composeTestRule.onNodeWithText("Test connection").assertDoesNotExist()
    }

    @Test
    fun `sync toggle is named by its label and toggles on tap`() {
        var value: Boolean? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                ImmichSettingsContent(
                    state = ImmichSettingsUiState(isLoading = false, hasApiKey = true, syncEnabled = false),
                    onSyncEnabledChange = { value = it },
                )
            }
        }

        composeTestRule.onNodeWithText("Sync automatically")
            .performScrollTo()
            .assertIsToggleable()
            .assertIsOff()
            .performClick()
        assertEquals(true, value)
    }

    @Test
    fun `shows a test failure distinctly`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ImmichSettingsContent(
                    state = ImmichSettingsUiState(
                        isLoading = false,
                        hasApiKey = true,
                        testResult = ImmichTestOutcome(ok = false, message = "invalid API key"),
                    ),
                )
            }
        }

        composeTestRule.onNodeWithText("Test failed: invalid API key").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `shows the last sync error when the most recent sync failed`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ImmichSettingsContent(
                    state = ImmichSettingsUiState(
                        isLoading = false,
                        hasApiKey = true,
                        lastSyncStatus = "error",
                        lastSyncError = "connection refused",
                    ),
                )
            }
        }

        composeTestRule.onNodeWithText("Last sync failed: connection refused").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `remove connection only appears once a key is stored`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ImmichSettingsContent(state = ImmichSettingsUiState(isLoading = false, hasApiKey = false))
            }
        }

        composeTestRule.onNodeWithText("Remove connection").assertDoesNotExist()
    }

    @Test
    fun `tapping remove connection invokes its callback`() {
        var removed = false
        composeTestRule.setContent {
            MycorrhizalTheme {
                ImmichSettingsContent(
                    state = ImmichSettingsUiState(isLoading = false, hasApiKey = true),
                    onRemove = { removed = true },
                )
            }
        }

        composeTestRule.onNodeWithText("Remove connection").performScrollTo().performClick()
        assertEquals(true, removed)
    }

    @Test
    fun `save error is announced as an assertive live region`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ImmichSettingsContent(state = ImmichSettingsUiState(isLoading = false, saveError = "Could not save"))
            }
        }

        composeTestRule.onNodeWithText("Could not save")
            .assert(SemanticsMatcher.expectValue(SemanticsProperties.LiveRegion, LiveRegionMode.Assertive))
    }
}
