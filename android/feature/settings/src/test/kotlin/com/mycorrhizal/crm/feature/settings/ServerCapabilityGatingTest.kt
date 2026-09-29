package com.mycorrhizal.crm.feature.settings

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performScrollTo
import com.mycorrhizal.crm.domain.compat.ServerCapabilitiesInfo
import com.mycorrhizal.crm.domain.compat.ServerCapability
import com.mycorrhizal.crm.domain.repository.SessionState
import com.mycorrhizal.crm.ui.LocalServerCapabilities
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issue #1263 / ADR 0028 Decision 2: capability-gated UI. Settings renders with
 * the embedded deployment's capability set (the `/health` fixture's tokens) and
 * the network-only rows must be absent; an unknown set fails open to everything
 * visible.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ServerCapabilityGatingTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val adminState = SettingsUiState(
        session = SessionState(serverUrl = "embedded.invalid", username = "local", isAdmin = true),
    )

    private fun setSettingsContent(capabilities: ServerCapabilitiesInfo) {
        composeTestRule.setContent {
            CompositionLocalProvider(LocalServerCapabilities provides capabilities) {
                MycorrhizalTheme {
                    SettingsContent(state = adminState, onLogout = {})
                }
            }
        }
    }

    /** Exactly the tokens the embedded deployment exposes (health-embedded.json). */
    private val embeddedCapabilities = ServerCapabilitiesInfo(
        deployment = ServerCapabilitiesInfo.DEPLOYMENT_EMBEDDED,
        capabilities = setOf(
            ServerCapability.CONTACTS,
            ServerCapability.DASHBOARD,
            ServerCapability.NOTES,
            ServerCapability.ACTIVITIES,
            ServerCapability.REMINDERS,
            ServerCapability.LIFE_EVENTS,
            ServerCapability.GRAPH,
            ServerCapability.SEARCH,
            ServerCapability.IMPORT,
            ServerCapability.EXPORT,
            ServerCapability.CALENDAR,
            ServerCapability.NOTIFICATIONS,
        ),
    )

    @Test
    fun `settings hides the network-only rows on an embedded deployment`() {
        setSettingsContent(embeddedCapabilities)

        composeTestRule.onNodeWithText("Two-factor authentication").assertDoesNotExist()
        composeTestRule.onNodeWithText("Webhooks").assertDoesNotExist()
        composeTestRule.onNodeWithText("API Tokens").assertDoesNotExist()
        composeTestRule.onNodeWithText("Notification channels").assertDoesNotExist()
        composeTestRule.onNodeWithText("User management").assertDoesNotExist()
        composeTestRule.onNodeWithText("System events").assertDoesNotExist()
        composeTestRule.onNodeWithText("Set up biometric sign-in").assertDoesNotExist()
    }

    @Test
    fun `settings keeps the core product rows on an embedded deployment`() {
        setSettingsContent(embeddedCapabilities)

        // Calendar sync is a client-side calendars surface that embedded mode
        // does keep (`calendar` is not a disabled capability); only DAV
        // *serving* is disabled, and Android has no client surface for that.
        composeTestRule.onNodeWithText("Calendar Sync").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Data suggestions").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `settings shows every row when the capability set is unknown`() {
        setSettingsContent(ServerCapabilitiesInfo.Unknown)

        // Fail open: an unreachable /health never strips settings.
        composeTestRule.onNodeWithText("Two-factor authentication").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Webhooks").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("API Tokens").performScrollTo().assertIsDisplayed()
    }
}
