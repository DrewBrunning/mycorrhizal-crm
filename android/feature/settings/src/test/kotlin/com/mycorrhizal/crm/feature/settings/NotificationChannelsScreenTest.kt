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

    @Test
    fun `ntfy enable switch is named by its label and toggles on tap`() {
        var value: Boolean? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                NotificationChannelsContent(
                    state = NotificationChannelsUiState(isLoading = false, notifyNtfy = false),
                    onNtfyUrlChange = {},
                    onNtfyTopicChange = {},
                    onNotifyNtfyChange = { value = it },
                    onGotifyUrlChange = {},
                    onGotifyTokenChange = {},
                    onNotifyGotifyChange = {},
                    onTest = {},
                    onSave = {},
                )
            }
        }

        // #199: Modifier.toggleable on the row merges the label into the
        // switch's own accessible name, so the label text is directly
        // toggleable -- TalkBack no longer announces an unnamed switch.
        composeTestRule.onNodeWithText("Send reminders via ntfy")
            .performScrollTo()
            .assertIsToggleable()
            .assertIsOff()
            .performClick()
        assertEquals(true, value)
    }

    @Test
    fun `save error is announced as an assertive live region`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                NotificationChannelsContent(
                    state = NotificationChannelsUiState(isLoading = false, saveError = "Could not save"),
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

        composeTestRule.onNodeWithText("Could not save")
            .assert(SemanticsMatcher.expectValue(SemanticsProperties.LiveRegion, LiveRegionMode.Assertive))
    }
}
