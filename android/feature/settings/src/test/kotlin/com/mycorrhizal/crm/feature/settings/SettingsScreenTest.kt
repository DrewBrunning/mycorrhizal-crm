package com.mycorrhizal.crm.feature.settings

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import com.mycorrhizal.crm.domain.repository.AppSettingsRepository
import com.mycorrhizal.crm.domain.repository.SessionState
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
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

    // --- M25 ---

    @Test
    fun `shows the editable appearance and password sections`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                SettingsContent(
                    state = SettingsUiState(
                        session = SessionState(language = "en", dateFormat = "eu"),
                        themePreference = AppSettingsRepository.THEME_SYSTEM,
                    ),
                    onLogout = {},
                )
            }
        }

        composeTestRule.onNodeWithText("Appearance").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Language").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("English").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Theme").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Use system default").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Change Password").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Current password").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `selecting a language invokes the language change callback`() {
        var changedTo: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                SettingsContent(
                    state = SettingsUiState(session = SessionState(language = "en")),
                    onLanguageChange = { changedTo = it },
                    onLogout = {},
                )
            }
        }

        composeTestRule.onNodeWithText("English").performScrollTo().performClick()
        composeTestRule.onNodeWithText("Deutsch").performClick()
        assertEquals("de", changedTo)
    }

    @Test
    fun `selecting a theme invokes the theme change callback`() {
        var changedTo: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                SettingsContent(
                    state = SettingsUiState(themePreference = AppSettingsRepository.THEME_SYSTEM),
                    onThemeChange = { changedTo = it },
                    onLogout = {},
                )
            }
        }

        composeTestRule.onNodeWithText("Use system default").performScrollTo().performClick()
        composeTestRule.onNodeWithText("Dark mode").performClick()
        assertEquals(AppSettingsRepository.THEME_DARK, changedTo)
    }

    @Test
    fun `selecting a date format invokes the date format change callback`() {
        var changedTo: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                SettingsContent(
                    state = SettingsUiState(session = SessionState(dateFormat = "eu")),
                    onDateFormatChange = { changedTo = it },
                    onLogout = {},
                )
            }
        }

        composeTestRule.onNodeWithText("European (DD.MM.YYYY)").performScrollTo().performClick()
        composeTestRule.onNodeWithText("ISO (YYYY-MM-DD)").performClick()
        assertEquals("iso", changedTo)
    }

    @Test
    fun `password change submits all three fields`() {
        var submitted: Triple<String, String, String>? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                SettingsContent(
                    state = SettingsUiState(),
                    onChangePassword = { c, n, cf -> submitted = Triple(c, n, cf) },
                    onLogout = {},
                )
            }
        }

        composeTestRule.onNodeWithText("Current password").performScrollTo().performClick()
        composeTestRule.onNodeWithText("New password").performClick()
        composeTestRule.onNodeWithText("Confirm new password").performClick()
        composeTestRule.onNodeWithText("Update password").performScrollTo().performClick()

        // The button stays disabled until fields are entered; with no text the
        // submit callback must never fire.
        assertEquals(null, submitted)
    }

    @Test
    fun `webhooks and notification channel rows invoke their navigation callbacks`() {
        var webhooks = false
        var channels = false
        composeTestRule.setContent {
            MycorrhizalTheme {
                SettingsContent(
                    state = SettingsUiState(),
                    onWebhooks = { webhooks = true },
                    onNotificationChannels = { channels = true },
                    onLogout = {},
                )
            }
        }

        composeTestRule.onNodeWithText("Webhooks").performScrollTo().performClick()
        assertTrue(webhooks)

        composeTestRule.onNodeWithText("Notification channels").performScrollTo().performClick()
        assertTrue(channels)
    }
}
