package com.mycorrhizal.crm.feature.settings

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import com.mycorrhizal.crm.domain.repository.SessionState
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
class SettingsScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun `shows session info`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                SettingsContent(
                    state = SettingsUiState(
                        session = SessionState(
                            serverUrl = "https://crm.example.com",
                            username = "alice",
                            isAdmin = true,
                            language = "en",
                        ),
                    ),
                    onLogout = {},
                )
            }
        }

        composeTestRule.onNodeWithText("https://crm.example.com").assertIsDisplayed()
        composeTestRule.onNodeWithText("alice").assertIsDisplayed()
        composeTestRule.onNodeWithText("Yes").assertIsDisplayed()
        composeTestRule.onNodeWithText("Log out").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `logout button invokes the callback after confirmation`() {
        var loggedOut = false
        composeTestRule.setContent {
            MycorrhizalTheme {
                SettingsContent(
                    state = SettingsUiState(session = SessionState(serverUrl = "https://crm.example.com", username = "alice")),
                    onLogout = { loggedOut = true },
                )
            }
        }

        composeTestRule.onNodeWithText("Log out").performScrollTo().performClick()
        // Confirmation dialog appears; the dialog's confirm button is the one
        // on top (the scroll-visible "Log out" button is behind the dialog).
        composeTestRule.onNodeWithText("Log out?").assertIsDisplayed()
        composeTestRule.onAllNodesWithText("Log out")[1].performClick()
        assertEquals(true, loggedOut)
    }
}
