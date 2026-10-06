package com.mycorrhizal.crm.feature.settings

import androidx.compose.ui.test.assertIsDisplayed
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
class GeoPulseSettingsScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(
        state: GeoPulseSettingsUiState,
        onSave: () -> Unit = {},
        onTest: () -> Unit = {},
        onRemove: () -> Unit = {},
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                GeoPulseSettingsContent(state = state, onSave = onSave, onTest = onTest, onRemove = onRemove)
            }
        }
    }

    @Test
    fun `first connect marks the token required and offers neither test nor disconnect`() {
        setContent(GeoPulseSettingsUiState(isLoading = false, baseUrl = "https://geo.example.com"))

        composeTestRule.onNodeWithText("API token *").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Create a token in GeoPulse (Profile → Security) and paste it here.")
            .performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Test connection").assertDoesNotExist()
        composeTestRule.onNodeWithText("Disconnect").assertDoesNotExist()
    }

    @Test
    fun `a stored token on the same origin is optional and says so`() {
        setContent(
            GeoPulseSettingsUiState(
                isLoading = false,
                baseUrl = "https://geo.example.com",
                storedBaseUrl = "https://geo.example.com",
                hasApiKey = true,
            ),
        )

        composeTestRule.onNodeWithText("API token").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("A token is already stored. Leave the field empty to keep it.")
            .performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `an origin change marks the token required with the explanation (issue 1501)`() {
        setContent(
            GeoPulseSettingsUiState(
                isLoading = false,
                baseUrl = "https://other.example.com",
                storedBaseUrl = "https://geo.example.com",
                hasApiKey = true,
            ),
        )

        composeTestRule.onNodeWithText("API token *").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText(
            "The server address changed, so re-enter the API token. The stored token is never sent to a different server.",
        ).performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `connected state shows test result stage and message and wires the buttons`() {
        var saved = 0
        var tested = 0
        var removed = 0
        setContent(
            GeoPulseSettingsUiState(
                isLoading = false,
                baseUrl = "https://geo.example.com",
                storedBaseUrl = "https://geo.example.com",
                hasApiKey = true,
                testResult = GeoPulseTestOutcome(ok = false, stage = "reachability", message = "connection refused"),
            ),
            onSave = { saved++ },
            onTest = { tested++ },
            onRemove = { removed++ },
        )

        composeTestRule.onNodeWithText("Test failed (reachability): connection refused")
            .performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Test connection").performScrollTo().performClick()
        composeTestRule.onNodeWithText("Save").performScrollTo().performClick()
        composeTestRule.onNodeWithText("Disconnect").performScrollTo().performClick()
        assertEquals(listOf(1, 1, 1), listOf(tested, saved, removed))
    }

    @Test
    fun `validation error is displayed from its string resource`() {
        setContent(
            GeoPulseSettingsUiState(
                isLoading = false,
                baseUrl = "https://geo.example.com",
                saveErrorRes = com.mycorrhizal.crm.ui.R.string.geopulse_settings_api_key_required,
            ),
        )

        composeTestRule.onNodeWithText("The API token is required to connect.").performScrollTo().assertIsDisplayed()
    }
}
