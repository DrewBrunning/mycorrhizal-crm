package com.mycorrhizal.crm.feature.settings

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performScrollTo
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
class NotificationChannelsScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun `shows both channel sections with their url fields`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                NotificationChannelsContent(
                    state = NotificationChannelsUiState(isLoading = false),
                    onNtfyUrlChange = {},
                    onNtfyTopicChange = {},
                    onNotifyNtfyChange = {},
                    onGotifyUrlChange = {},
                    onGotifyTokenChange = {},
                    onNotifyGotifyChange = {},
                    onTest = {},
                    onSave = {},
                )
            }
        }

        composeTestRule.onNodeWithText("ntfy").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Gotify").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("ntfy server URL").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Gotify server URL").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("App token").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `shows the stored-token hint when a token is already configured`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                NotificationChannelsContent(
                    state = NotificationChannelsUiState(isLoading = false, gotifyHasToken = true),
                    onNtfyUrlChange = {},
                    onNtfyTopicChange = {},
                    onNotifyNtfyChange = {},
                    onGotifyUrlChange = {},
                    onGotifyTokenChange = {},
                    onNotifyGotifyChange = {},
                    onTest = {},
                    onSave = {},
                )
            }
        }

        composeTestRule.onNodeWithText("A token is already stored. Leave the field empty to keep it.")
            .performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `shows a test failure distinctly`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                NotificationChannelsContent(
                    state = NotificationChannelsUiState(
                        isLoading = false,
                        testResult = mapOf("ntfy" to NotificationTestOutcome(ok = false, error = "ntfy is not configured")),
                    ),
                    onNtfyUrlChange = {},
                    onNtfyTopicChange = {},
                    onNotifyNtfyChange = {},
                    onGotifyUrlChange = {},
                    onGotifyTokenChange = {},
                    onNotifyGotifyChange = {},
                    onTest = {},
                    onSave = {},
                )
            }
        }

        composeTestRule.onNodeWithText("Test failed: ntfy is not configured").performScrollTo().assertIsDisplayed()
    }
}
