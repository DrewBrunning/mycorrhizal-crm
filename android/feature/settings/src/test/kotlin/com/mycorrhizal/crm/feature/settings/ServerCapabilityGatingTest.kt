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
                    // Every group's content in one column, so the capability gates of the whole
                    // Settings tree are asserted against one deployment at once.
                    androidx.compose.foundation.layout.Column {
                        SettingsRootContent(state = adminState, onLogout = {})
                        AccountSettingsContent(state = adminState)
                        TrackingSettingsContent(state = adminState)
                        IntegrationsSettingsContent()
                        AdministrationSettingsContent()
                    }
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
        ),
    )

    @Test
    fun `settings hides the network-only rows on an embedded deployment`() {
        setSettingsContent(embeddedCapabilities)

        composeTestRule.onNodeWithText("Two-factor authentication").assertDoesNotExist()
        composeTestRule.onNodeWithText("Webhooks").assertDoesNotExist()
        composeTestRule.onNodeWithText("API tokens").assertDoesNotExist()
        composeTestRule.onNodeWithText("Notification channels").assertDoesNotExist()
        composeTestRule.onNodeWithText("User management").assertDoesNotExist()
        composeTestRule.onNodeWithText("System events").assertDoesNotExist()
        composeTestRule.onNodeWithText("Integrations").assertDoesNotExist()
        composeTestRule.onNodeWithText("Administration").assertDoesNotExist()
        // Issue #1367: storage only — no outbound integrations, no calendar sync.
        composeTestRule.onNodeWithText("Calendar sync").assertDoesNotExist()
        composeTestRule.onNodeWithText("Immich").assertDoesNotExist()
        composeTestRule.onNodeWithText("Paperless-ngx").assertDoesNotExist()
        composeTestRule.onNodeWithText("Seafile").assertDoesNotExist()
        composeTestRule.onNodeWithText("Nextcloud / ownCloud").assertDoesNotExist()
        composeTestRule.onNodeWithText("GeoPulse").assertDoesNotExist()
    }

    @Test
    fun `settings keeps the core product rows on an embedded deployment`() {
        setSettingsContent(embeddedCapabilities)

        composeTestRule.onNodeWithText("Change password").assertExists()
        composeTestRule.onNodeWithText("Show notifications").assertExists()
        composeTestRule.onNodeWithText("Account").assertExists()
    }

    @Test
    fun `settings shows every row when the capability set is unknown`() {
        setSettingsContent(ServerCapabilitiesInfo.Unknown)

        // Fail open: an unreachable /health never strips settings.
        listOf(
            "Two-factor authentication", "Webhooks", "API tokens", "Calendar sync", "Immich", "GeoPulse",
            "Notification channels", "User management", "System events",
        ).forEach { composeTestRule.onNodeWithText(it).assertExists() }
    }
}
