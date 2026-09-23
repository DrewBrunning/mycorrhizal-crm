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
class NextcloudSettingsScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun `shows the base url, username and app password fields`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                NextcloudSettingsContent(state = NextcloudSettingsUiState(isLoading = false))
            }
        }

        composeTestRule.onNodeWithText("Server URL").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Username").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("App Password").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `shows the stored-password hint when a password is already configured`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                NextcloudSettingsContent(state = NextcloudSettingsUiState(isLoading = false, hasAppPassword = true))
            }
        }

        composeTestRule.onNodeWithText("An app password is already stored. Leave the field empty to keep it.")
            .performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `the test button only appears once a password is stored`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                NextcloudSettingsContent(state = NextcloudSettingsUiState(isLoading = false, hasAppPassword = false))
            }
        }

        composeTestRule.onNodeWithText("Test connection").assertDoesNotExist()
    }

    @Test
    fun `tapping test invokes its callback`() {
        var tested = false
        composeTestRule.setContent {
            MycorrhizalTheme {
                NextcloudSettingsContent(
                    state = NextcloudSettingsUiState(
                        isLoading = false,
                        hasAppPassword = true,
                        baseUrl = "https://nextcloud.example.com",
                        username = "drew",
                    ),
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
                NextcloudSettingsContent(
                    state = NextcloudSettingsUiState(
                        isLoading = false,
                        hasAppPassword = true,
                        testResult = NextcloudTestOutcome(ok = false, message = "invalid credentials"),
                    ),
                )
            }
        }

        composeTestRule.onNodeWithText("Test failed: invalid credentials").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `remove connection only appears once a password is stored`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                NextcloudSettingsContent(state = NextcloudSettingsUiState(isLoading = false, hasAppPassword = false))
            }
        }

        composeTestRule.onNodeWithText("Remove connection").assertDoesNotExist()
    }

    @Test
    fun `tapping remove connection invokes its callback`() {
        var removed = false
        composeTestRule.setContent {
            MycorrhizalTheme {
                NextcloudSettingsContent(
                    state = NextcloudSettingsUiState(isLoading = false, hasAppPassword = true),
                    onRemove = { removed = true },
                )
            }
        }

        composeTestRule.onNodeWithText("Remove connection").performScrollTo().performClick()
        assertEquals(true, removed)
    }

    @Test
    fun `save is disabled until both base url and username are entered`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                NextcloudSettingsContent(
                    state = NextcloudSettingsUiState(isLoading = false, baseUrl = "https://nextcloud.example.com", username = ""),
                )
            }
        }

        composeTestRule.onNodeWithText("Save").performScrollTo().assertIsNotEnabled()
    }

    @Test
    fun `save error is announced as an assertive live region`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                NextcloudSettingsContent(state = NextcloudSettingsUiState(isLoading = false, saveError = "Could not save"))
            }
        }

        composeTestRule.onNodeWithText("Could not save")
            .assert(SemanticsMatcher.expectValue(SemanticsProperties.LiveRegion, LiveRegionMode.Assertive))
    }
}
