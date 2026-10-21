package com.mycorrhizal.crm.feature.settings

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performScrollTo
import com.mycorrhizal.crm.domain.repository.BiometricEnrollmentStatus
import com.mycorrhizal.crm.domain.backup.BundleBackupStatus
import com.mycorrhizal.crm.domain.repository.BundleBackupRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.ExportRepository
import com.mycorrhizal.crm.domain.repository.RelationshipEdgeRepository
import com.mycorrhizal.crm.domain.repository.SessionState
import com.mycorrhizal.crm.model.AppVersion
import com.mycorrhizal.crm.model.network.ApiToken
import com.mycorrhizal.crm.ui.LocalServerVersion
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.mockk
import kotlinx.coroutines.flow.flowOf
import io.mockk.every
import org.junit.Assert.assertTrue
import org.junit.Assert.assertFalse
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

// Issue #692: capability-gated affordances are hidden when the connected server
// (resolved from /health, provided via LocalServerVersion) is too old to
// provide them. A null server version — the fail-open default in previews,
// tests and before the per-session check resolves — leaves everything visible.
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ServerVersionGatingTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun version(major: Int, minor: Int, patch: Int): AppVersion =
        AppVersion(major, minor, patch)

    private fun adminState() = SettingsUiState(
        session = SessionState(serverUrl = "https://crm.example.com", username = "alice", isAdmin = true),
    )

    private fun setAdministration(serverVersion: AppVersion?) {
        composeTestRule.setContent {
            CompositionLocalProvider(LocalServerVersion provides serverVersion) {
                MycorrhizalTheme { AdministrationSettingsContent() }
            }
        }
    }

    private fun setRoot(serverVersion: AppVersion?, state: SettingsUiState) {
        composeTestRule.setContent {
            CompositionLocalProvider(LocalServerVersion provides serverVersion) {
                MycorrhizalTheme { SettingsRootContent(state = state, onLogout = {}) }
            }
        }
    }

    /** What the Privacy screen would pass to `setAppLock(…, offerBiometricSignIn)`. */
    private fun biometricOfferedOn(serverVersion: AppVersion?): Boolean {
        var offered = false
        composeTestRule.setContent {
            CompositionLocalProvider(LocalServerVersion provides serverVersion) {
                offered = rememberSettingsGating().deviceGrantSignInSupported
            }
        }
        composeTestRule.waitForIdle()
        return offered
    }

    // --- Administration: the system-events row needs the v0.6.2 admin endpoints ---

    @Test
    fun `administration shows the system events row on a v0_6_2 server`() {
        setAdministration(serverVersion = version(0, 6, 2))
        composeTestRule.onNodeWithText("System events").assertIsDisplayed()
    }

    @Test
    fun `administration hides the system events row on a v0_6_1 server`() {
        setAdministration(serverVersion = version(0, 6, 1))
        composeTestRule.onNodeWithText("User management").assertIsDisplayed()
        composeTestRule.onNodeWithText("System events").assertDoesNotExist()
    }

    @Test
    fun `administration shows the system events row when the server version is unknown`() {
        // Fail open: an unresolved /health never hides a feature.
        setAdministration(serverVersion = null)
        composeTestRule.onNodeWithText("System events").assertIsDisplayed()
    }

    @Test
    fun `the administration group is absent for non-admins regardless of server`() {
        setRoot(version(0, 6, 2), SettingsUiState(session = SessionState(isAdmin = false)))
        composeTestRule.onNodeWithText("Administration").assertDoesNotExist()
    }

    @Test
    fun `the administration group is offered to admins`() {
        setRoot(version(0, 6, 2), adminState())
        composeTestRule.onNodeWithText("Administration").performScrollTo().assertIsDisplayed()
    }

    // --- App lock: inline biometric sign-in needs the v0.6.10 device grants ---

    @Test
    fun `biometric sign-in is offered on a v0_6_10 server`() {
        assertTrue(biometricOfferedOn(version(0, 6, 10)))
    }

    @Test
    fun `biometric sign-in is not offered when the server predates device grants`() {
        assertFalse(biometricOfferedOn(version(0, 6, 9)))
    }

    @Test
    fun `biometric sign-in is offered when the server version is unknown`() {
        assertTrue(biometricOfferedOn(null))
    }

    // --- Data screen: audit-log export needs the v0.6.1 endpoint ------------

    private fun dataScreen(serverVersion: AppVersion?) {
        val viewModel = DataViewModel(
            mockk<ContactRepository>(),
            mockk<RelationshipEdgeRepository>(),
            mockk<ExportRepository>(),
            mockk<BundleBackupRepository> { every { observeStatus() } returns flowOf(BundleBackupStatus()) },
        )
        composeTestRule.setContent {
            CompositionLocalProvider(LocalServerVersion provides serverVersion) {
                MycorrhizalTheme {
                    DataScreen(onMenuClick = {}, viewModel = viewModel)
                }
            }
        }
    }

    @Test
    fun `data screen hides the audit-log export on a v0_6_0 server`() {
        dataScreen(serverVersion = version(0, 6, 0))

        // Baseline formats remain, the audit export does not exist yet.
        composeTestRule.onNodeWithText("Export CSV backup").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Export JSContact").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Export audit log").assertDoesNotExist()
    }

    @Test
    fun `data screen shows the audit-log export from v0_6_1 on`() {
        dataScreen(serverVersion = version(0, 6, 1))

        composeTestRule.onNodeWithText("Export audit log").performScrollTo().assertIsDisplayed()
    }

    // --- API tokens: rotate needs the v0.6.1 endpoints ----------------------

    private fun token() = ApiToken(
        id = 1,
        name = "CI token",
        createdAt = "2026-01-01T00:00:00Z",
        scope = "full",
    )

    @Test
    fun `an active token on a v0_6_0 server offers revoke but not rotate`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ApiTokenRow(
                    token = token(),
                    rotating = false,
                    revoking = false,
                    rotateSupported = false,
                    onRotate = {},
                    onRevoke = {},
                )
            }
        }

        composeTestRule.onNodeWithContentDescription("Rotate CI token").assertDoesNotExist()
        composeTestRule.onNodeWithContentDescription("Revoke CI token").assertIsDisplayed()
    }

    @Test
    fun `an active token on a v0_6_1 server offers both rotate and revoke`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ApiTokenRow(
                    token = token(),
                    rotating = false,
                    revoking = false,
                    rotateSupported = true,
                    onRotate = {},
                    onRevoke = {},
                )
            }
        }

        composeTestRule.onNodeWithContentDescription("Rotate CI token").assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("Revoke CI token").assertIsDisplayed()
    }
}
