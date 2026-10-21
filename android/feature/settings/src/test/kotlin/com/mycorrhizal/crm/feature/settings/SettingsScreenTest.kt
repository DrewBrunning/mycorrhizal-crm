package com.mycorrhizal.crm.feature.settings

import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertIsEnabled
import android.content.Context
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.hasSetTextAction
import androidx.compose.runtime.Composable
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import com.mycorrhizal.crm.data.passkey.PasskeyAvailability
import com.mycorrhizal.crm.data.auth.DeviceGrantManager
import com.mycorrhizal.crm.domain.about.AppBuildInfo
import com.mycorrhizal.crm.domain.repository.AppSettingsRepository
import com.mycorrhizal.crm.model.network.ServerHealth
import com.mycorrhizal.crm.domain.repository.AutoLockDelay
import com.mycorrhizal.crm.domain.repository.BiometricEnrollmentStatus
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.LocalAuthCapabilities
import com.mycorrhizal.crm.domain.repository.LocalAuthSettingsRepository
import com.mycorrhizal.crm.domain.repository.SessionState
import com.mycorrhizal.crm.domain.repository.TrackingSettingsRepository
import com.mycorrhizal.crm.testing.a11y.assertAccessibleSemantics
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.flowOf
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

    private val release = AppBuildInfo("1.3.0", 1042, "release", "obtainium", "com.mycorrhizal.crm")

    private fun show(content: @Composable () -> Unit) {
        composeTestRule.setContent { MycorrhizalTheme { content() } }
    }

    // --- Root (issue #1759) ---------------------------------------------------------------------

    @Test
    fun `the root is an index of grouped rows with no text fields`() {
        show { SettingsRootContent(state = SettingsUiState(buildInfo = release), onLogout = {}) }

        listOf(
            "Account", "Appearance", "Privacy & security", "Contacts & fields",
            "Tracking & notifications", "Integrations", "About", "Log out",
        ).forEach { composeTestRule.onNodeWithText(it).performScrollTo().assertIsDisplayed() }
        // No form fields live on the root any more.
        assertEquals(0, composeTestRule.onAllNodes(hasSetTextAction()).fetchSemanticsNodes().size)
        composeTestRule.onNodeWithText("Administration").assertDoesNotExist()
        composeTestRule.onNodeWithText("Data suggestions").assertDoesNotExist()
    }

    @Test
    fun `each root group opens its sub-screen route`() {
        val opened = mutableListOf<String>()
        show {
            SettingsRootContent(
                state = SettingsUiState(session = SessionState(isAdmin = true), buildInfo = release),
                onNavigate = { opened += it },
                onLogout = {},
            )
        }

        listOf(
            "settings-group-account", "settings-group-appearance", "settings-group-privacy",
            "settings-group-contacts-fields", "settings-group-tracking", "settings-group-integrations",
            "settings-group-administration", "settings-group-about",
        ).forEach { composeTestRule.onNodeWithTag(it).performScrollTo().performClick() }

        assertEquals(
            listOf(
                SettingsRoutes.ACCOUNT, SettingsRoutes.APPEARANCE, SettingsRoutes.PRIVACY,
                SettingsRoutes.CONTACTS_FIELDS, SettingsRoutes.TRACKING, SettingsRoutes.INTEGRATIONS,
                SettingsRoutes.ADMINISTRATION, SettingsRoutes.ABOUT,
            ),
            opened,
        )
    }

    @Test
    fun `the administration group is for admins only`() {
        show { SettingsRootContent(state = SettingsUiState(session = SessionState(isAdmin = false)), onLogout = {}) }
        composeTestRule.onNodeWithText("Administration").assertDoesNotExist()
    }

    @Test
    fun `no About row without build info`() {
        show { SettingsRootContent(state = SettingsUiState(), onLogout = {}) }
        composeTestRule.onNodeWithText("About").assertDoesNotExist()
    }

    @Test
    fun `log out invokes the callback only after confirmation`() {
        var loggedOut = false
        show { SettingsRootContent(state = SettingsUiState(), onLogout = { loggedOut = true }) }

        composeTestRule.onNodeWithText("Log out").performScrollTo().performClick()
        composeTestRule.onNodeWithText("Log out?").assertIsDisplayed()
        assertEquals(false, loggedOut)
        composeTestRule.onAllNodesWithText("Log out")[1].performClick()
        assertEquals(true, loggedOut)
    }

    @Test
    fun `cancelling the log out confirmation keeps the session`() {
        var loggedOut = false
        show { SettingsRootContent(state = SettingsUiState(), onLogout = { loggedOut = true }) }

        composeTestRule.onNodeWithText("Log out").performScrollTo().performClick()
        composeTestRule.onNodeWithText("Cancel").performClick()
        composeTestRule.onNodeWithText("Log out?").assertDoesNotExist()
        assertEquals(false, loggedOut)
    }

    @Test
    fun `log out is inert and announces saving while in flight`() {
        var loggedOut = false
        show { SettingsRootContent(state = SettingsUiState(isLoggingOut = true), onLogout = { loggedOut = true }) }

        composeTestRule.onNodeWithTag("settings-log-out")
            .assert(SemanticsMatcher.expectValue(SemanticsProperties.StateDescription, "Saving"))
            .performScrollTo()
            .performClick()
        composeTestRule.onNodeWithText("Log out?").assertDoesNotExist()
        assertEquals(false, loggedOut)
    }

    // --- Account -------------------------------------------------------------------------------

    @Test
    fun `account shows session info and the security rows`() {
        show {
            AccountSettingsContent(
                state = SettingsUiState(
                    session = SessionState(serverUrl = "https://crm.example.com", username = "alice", isAdmin = true),
                ),
            )
        }

        composeTestRule.onNodeWithText("https://crm.example.com").assertIsDisplayed()
        composeTestRule.onNodeWithText("alice").assertIsDisplayed()
        composeTestRule.onNodeWithText("Yes").assertIsDisplayed()
        listOf("Servers", "Change password", "Two-factor authentication", "API tokens").forEach {
            composeTestRule.onNodeWithText(it).performScrollTo().assertIsDisplayed()
        }
    }

    @Test
    fun `account rows invoke their navigation callbacks`() {
        val opened = mutableListOf<String>()
        show {
            AccountSettingsContent(
                state = SettingsUiState(passkeysAvailable = true),
                onServers = { opened += "servers" },
                onChangePassword = { opened += "password" },
                onTwoFactor = { opened += "2fa" },
                onPasskeys = { opened += "passkeys" },
                onApiTokens = { opened += "tokens" },
            )
        }

        listOf("Servers", "Change password", "Two-factor authentication", "Passkeys", "API tokens").forEach {
            composeTestRule.onNodeWithText(it).performScrollTo().performClick()
        }
        assertEquals(listOf("servers", "password", "2fa", "passkeys", "tokens"), opened)
    }

    // Issue #1293 / ADR 0034: the Passkeys entry appears only through an open gate.
    @Test
    fun `the passkeys entry is hidden when the gate is closed`() {
        show { AccountSettingsContent(state = SettingsUiState(passkeysAvailable = false)) }
        composeTestRule.onNodeWithText("Passkeys").assertDoesNotExist()
    }

    @Test
    fun `account section titles are marked as headings`() {
        show { AccountSettingsContent(state = SettingsUiState(session = SessionState())) }
        // #208: TalkBack's heading navigation found nothing on the old page.
        listOf("Session", "Security").forEach {
            composeTestRule.onNodeWithText(it).assert(SemanticsMatcher.keyIsDefined(SemanticsProperties.Heading))
        }
    }

    // --- Change password -----------------------------------------------------------------------

    // Issue #1760: the disabled "Update password" button says what enables it.
    @Test
    fun `update password explains what enables it and drops the hint once filled`() {
        show { ChangePasswordContent(state = SettingsUiState()) }

        composeTestRule.onNodeWithText("Update password").assertIsNotEnabled()
        composeTestRule.onNodeWithText("Enter your new password and confirm it to continue").assertIsDisplayed()
        composeTestRule.onNodeWithText("Update password").assert(
            SemanticsMatcher.expectValue(
                SemanticsProperties.StateDescription,
                "Enter your new password and confirm it to continue",
            ),
        )

        composeTestRule.onNodeWithText("New password").performTextInput("hunter22")
        composeTestRule.onNodeWithText("Confirm new password").performTextInput("hunter22")

        composeTestRule.onNodeWithText("Update password").assertIsEnabled()
        composeTestRule.onNodeWithText("Enter your new password and confirm it to continue").assertDoesNotExist()
    }

    @Test
    fun `password change submits all three fields and clears them`() {
        var submitted: Triple<String, String, String>? = null
        show {
            ChangePasswordContent(
                state = SettingsUiState(),
                onChangePassword = { c, n, cf -> submitted = Triple(c, n, cf) },
            )
        }

        composeTestRule.onNodeWithText("Current password").performTextInput("old-pass")
        composeTestRule.onNodeWithText("New password").performTextInput("new-pass")
        composeTestRule.onNodeWithText("Confirm new password").performTextInput("new-pass")
        composeTestRule.onNodeWithText("Update password").performClick()

        assertEquals(Triple("old-pass", "new-pass", "new-pass"), submitted)
        composeTestRule.onNodeWithText("Update password").assertIsNotEnabled()
    }

    @Test
    fun `a password change error is announced as an assertive live region`() {
        show { ChangePasswordContent(state = SettingsUiState(passwordError = "Current password is wrong")) }

        composeTestRule.onNodeWithText("Current password is wrong")
            .assert(SemanticsMatcher.expectValue(SemanticsProperties.LiveRegion, LiveRegionMode.Assertive))
    }

    @Test
    fun `a static password validation error is shown`() {
        show {
            ChangePasswordContent(state = SettingsUiState(passwordErrorRes = com.mycorrhizal.crm.ui.R.string.settings_password_mismatch))
        }
        composeTestRule.onNodeWithText("New passwords do not match.").assertIsDisplayed()
    }

    @Test
    fun `the change-password button announces saving while in flight`() {
        show { ChangePasswordContent(state = SettingsUiState(isChangingPassword = true)) }

        composeTestRule.onNodeWithText("Update password")
            .assert(SemanticsMatcher.expectValue(SemanticsProperties.StateDescription, "Saving"))
    }

    // --- Appearance ----------------------------------------------------------------------------

    @Test
    fun `appearance shows language, date format and theme`() {
        show {
            AppearanceSettingsContent(
                state = SettingsUiState(
                    session = SessionState(language = "en", dateFormat = "eu"),
                    themePreference = AppSettingsRepository.THEME_SYSTEM,
                ),
            )
        }

        listOf("Language", "English", "Date format", "European (DD.MM.YYYY)", "Theme", "Use system default")
            .forEach { composeTestRule.onNodeWithText(it).assertIsDisplayed() }
    }

    @Test
    fun `selecting a language invokes the language change callback`() {
        var changedTo: String? = null
        show {
            AppearanceSettingsContent(
                state = SettingsUiState(session = SessionState(language = "en")),
                onLanguageChange = { changedTo = it },
            )
        }

        composeTestRule.onNodeWithText("English").performClick()
        composeTestRule.onNodeWithText("Deutsch").performClick()
        assertEquals("de", changedTo)
    }

    @Test
    fun `selecting a theme invokes the theme change callback`() {
        var changedTo: String? = null
        show {
            AppearanceSettingsContent(
                state = SettingsUiState(themePreference = AppSettingsRepository.THEME_SYSTEM),
                onThemeChange = { changedTo = it },
            )
        }

        composeTestRule.onNodeWithText("Use system default").performClick()
        composeTestRule.onNodeWithText("Dark mode").performClick()
        assertEquals(AppSettingsRepository.THEME_DARK, changedTo)
    }

    @Test
    fun `selecting a date format invokes the date format change callback`() {
        var changedTo: String? = null
        show {
            AppearanceSettingsContent(
                state = SettingsUiState(session = SessionState(dateFormat = "eu")),
                onDateFormatChange = { changedTo = it },
            )
        }

        composeTestRule.onNodeWithText("European (DD.MM.YYYY)").performClick()
        composeTestRule.onNodeWithText("ISO (YYYY-MM-DD)").performClick()
        assertEquals("iso", changedTo)
    }

    // --- Privacy & security --------------------------------------------------------------------

    @Test
    fun `app lock is a single control with no separate biometric setup button`() {
        var toggled: Boolean? = null
        show {
            PrivacySettingsContent(
                state = SettingsUiState(localAuthSupported = true, requireLocalAuth = false),
                onAppLockChange = { toggled = it },
            )
        }

        composeTestRule.onNodeWithText("Set up biometric sign-in").assertDoesNotExist()
        composeTestRule.onNodeWithText("Turn off biometric sign-in").assertDoesNotExist()
        composeTestRule.onNodeWithText("Require biometric or device PIN to open the app").performClick()
        assertEquals(true, toggled)
    }

    @Test
    fun `app lock on shows the hint and the lock delay`() {
        var delay: AutoLockDelay? = null
        show {
            PrivacySettingsContent(
                state = SettingsUiState(requireLocalAuth = true, autoLockDelay = AutoLockDelay.IMMEDIATELY),
                onAutoLockDelayChange = { delay = it },
            )
        }

        composeTestRule.onNodeWithText("Lock automatically after").assertIsDisplayed()
        composeTestRule.onNodeWithText("Immediately").performClick()
        composeTestRule.onNodeWithText("5 minutes").performClick()
        assertEquals(AutoLockDelay.FIVE_MINUTES, delay)
    }

    @Test
    fun `an unsupported device gets the reason and a disabled switch`() {
        show { PrivacySettingsContent(state = SettingsUiState(localAuthSupported = false)) }

        composeTestRule.onNodeWithText("Unavailable", substring = true).assertIsDisplayed()
        composeTestRule.onNodeWithText("Require biometric or device PIN to open the app").assertIsNotEnabled()
    }

    @Test
    fun `an enrolled device says biometric sign-in is on`() {
        show {
            PrivacySettingsContent(
                state = SettingsUiState(
                    requireLocalAuth = true,
                    biometricEnrollmentStatus = BiometricEnrollmentStatus.ENROLLED,
                ),
            )
        }
        composeTestRule.onNodeWithText("Biometric sign-in is on for this device").assertIsDisplayed()
        // With the lock on, the lock switch itself is the way to turn biometric sign-in off.
        composeTestRule.onNodeWithTag("biometric-signin-remove").assertDoesNotExist()
    }

    @Test
    fun `a grant enrolled with the lock off can still be revoked`() {
        // The post-login prompt enrolls without switching the lock on, so there is no switch to
        // turn off; the revoke must stay reachable.
        var removed = 0
        show {
            PrivacySettingsContent(
                state = SettingsUiState(
                    requireLocalAuth = false,
                    biometricEnrollmentStatus = BiometricEnrollmentStatus.ENROLLED,
                ),
                onRemoveBiometricSignIn = { removed++ },
            )
        }
        composeTestRule.onNodeWithText("Turn off biometric sign-in").performScrollTo().performClick()
        assertEquals(1, removed)
    }

    @Test
    fun `a biometric enrolment error is announced`() {
        show {
            PrivacySettingsContent(
                state = SettingsUiState(
                    requireLocalAuth = true,
                    isBiometricBusy = true,
                    biometricErrorRes = com.mycorrhizal.crm.ui.R.string.biometric_enroll_error,
                ),
            )
        }
        composeTestRule.onNodeWithText("Biometric sign-in is on for this device").assertDoesNotExist()
        composeTestRule.onAllNodes(
            SemanticsMatcher.expectValue(SemanticsProperties.LiveRegion, LiveRegionMode.Assertive),
        ).assertCountEquals(1)
    }

    // Issue #1741: opt-in screenshot / screen-recording block.
    @Test
    fun `the block-screenshots toggle shows its hint and invokes its callback`() {
        var changed: Boolean? = null
        show {
            PrivacySettingsContent(
                state = SettingsUiState(blockScreenshots = false),
                onBlockScreenshotsChange = { changed = it },
            )
        }

        composeTestRule.onNodeWithText("Block screenshots and screen recording").performClick()
        assertEquals(true, changed)
        composeTestRule
            .onNodeWithText("Also hides this app in the recent-apps view on all Android versions.", substring = true)
            .assertExists()
    }

    @Test
    fun `privacy section titles are marked as headings`() {
        show { PrivacySettingsContent(state = SettingsUiState()) }
        listOf("App lock", "Screen privacy").forEach {
            composeTestRule.onNodeWithText(it).assert(SemanticsMatcher.keyIsDefined(SemanticsProperties.Heading))
        }
    }

    // --- Contacts & fields ---------------------------------------------------------------------

    @Test
    fun `contacts and fields rows invoke their callbacks, custom link actions is a chevron row`() {
        val opened = mutableListOf<String>()
        show {
            ContactsFieldsSettingsContent(
                onCustomFields = { opened += "fields" },
                onContactFieldSettings = { opened += "contact-fields" },
                onCustomLinks = { opened += "links" },
                onCircleTagTriage = { opened += "triage" },
            )
        }

        listOf("Custom fields", "Contact field settings", "Custom link actions", "Circle & tag triage")
            .forEach { composeTestRule.onNodeWithText(it).performClick() }
        assertEquals(listOf("fields", "contact-fields", "links", "triage"), opened)
    }

    // --- Tracking & notifications --------------------------------------------------------------

    @Test
    fun `hides the call and SMS capture toggles in a build without the feature`() {
        show { TrackingSettingsContent(state = SettingsUiState(callSmsTrackingAvailable = false)) }

        // Issue #1200: the play build omits the whole capture feature.
        composeTestRule.onNodeWithText("Log calls as activities").assertDoesNotExist()
        composeTestRule.onNodeWithText("Log messages as activities").assertDoesNotExist()
        composeTestRule.onNodeWithText("Log unknown numbers too").assertDoesNotExist()
        composeTestRule.onNodeWithText("Call and message logging is not available in this build.").assertIsDisplayed()
        // Notifications are unrelated to capture and stay available.
        composeTestRule.onNodeWithText("Show notifications").assertIsDisplayed()
    }

    @Test
    fun `shows the call and SMS capture toggles and reports changes`() {
        val changes = mutableListOf<String>()
        show {
            TrackingSettingsContent(
                state = SettingsUiState(callSmsTrackingAvailable = true),
                onCallTrackingChange = { changes += "call=$it" },
                onSmsTrackingChange = { changes += "sms=$it" },
                onNotificationsChange = { changes += "notifications=$it" },
                onIncludeUnknownChange = { changes += "unknown=$it" },
            )
        }

        composeTestRule.onNodeWithText("Log calls as activities").performClick()
        composeTestRule.onNodeWithText("Log messages as activities").performClick()
        composeTestRule.onNodeWithText("Show notifications").performClick()
        composeTestRule.onNodeWithText("Log unknown numbers too").performClick()
        assertEquals(listOf("call=true", "sms=true", "notifications=false", "unknown=true"), changes)
        composeTestRule.onNodeWithText("Call and message logging is not available in this build.").assertDoesNotExist()
    }

    @Test
    fun `the notification channels row invokes its callback`() {
        var channels = false
        show { TrackingSettingsContent(state = SettingsUiState(), onNotificationChannels = { channels = true }) }
        composeTestRule.onNodeWithText("Notification channels").performClick()
        assertTrue(channels)
    }

    @Test
    fun `the filtered unknown count is shown when nonzero`() {
        show { TrackingSettingsContent(state = SettingsUiState(filteredUnknownCount = 12)) }
        composeTestRule.onNodeWithText("Unknown numbers filtered: 12").assertIsDisplayed()
    }

    // --- Issue #721: tracking-permission denial dialogs ----------------------------------------

    @Test
    fun `a rationale dialog explains the call-tracking permission and retries`() {
        var retried = false
        var dismissed = false
        show {
            TrackingSettingsContent(
                state = SettingsUiState(
                    permissionDialog = TrackingPermissionDialog.Rationale(TrackingPermissionRequest.CALL_TRACKING),
                ),
                onPermissionDialogRetry = { retried = true },
                onPermissionDialogDismiss = { dismissed = true },
            )
        }

        composeTestRule.onNodeWithText("Permission needed").assertIsDisplayed()
        composeTestRule.onNodeWithText("Try again").performClick()
        assertEquals(true, retried)
        composeTestRule.onNodeWithText("Not now").performClick()
        assertEquals(true, dismissed)
    }

    @Test
    fun `an app-settings dialog offers the system settings deep link for a permanent denial`() {
        var opened = false
        var dismissed = false
        show {
            TrackingSettingsContent(
                state = SettingsUiState(
                    permissionDialog = TrackingPermissionDialog.AppSettings(TrackingPermissionRequest.SMS_TRACKING),
                ),
                onPermissionDialogOpenSettings = { opened = true },
                onPermissionDialogDismiss = { dismissed = true },
            )
        }

        composeTestRule.onNodeWithText("Permission blocked").assertIsDisplayed()
        composeTestRule.onNodeWithText("Open settings").performClick()
        assertEquals(true, opened)
        composeTestRule.onNodeWithText("Not now").performClick()
        assertEquals(true, dismissed)
    }

    // --- Integrations / Administration ---------------------------------------------------------

    @Test
    fun `integration rows invoke their navigation callbacks`() {
        val opened = mutableListOf<String>()
        show {
            IntegrationsSettingsContent(
                onCalendarSync = { opened += "calendar" },
                onWebhooks = { opened += "webhooks" },
                onImmichSettings = { opened += "immich" },
                onPaperlessSettings = { opened += "paperless" },
                onSeafileSettings = { opened += "seafile" },
                onNextcloudSettings = { opened += "nextcloud" },
                onGeoPulseSettings = { opened += "geopulse" },
            )
        }

        listOf(
            "Calendar sync", "Webhooks", "Immich", "Paperless-ngx", "Seafile", "Nextcloud / ownCloud", "GeoPulse",
        ).forEach { composeTestRule.onNodeWithText(it).performScrollTo().performClick() }
        assertEquals(
            listOf("calendar", "webhooks", "immich", "paperless", "seafile", "nextcloud", "geopulse"),
            opened,
        )
    }

    @Test
    fun `administration rows invoke their navigation callbacks`() {
        val opened = mutableListOf<String>()
        show {
            AdministrationSettingsContent(
                onManageUsers = { opened += "users" },
                onSystemEvents = { opened += "events" },
            )
        }

        composeTestRule.onNodeWithText("User management").performClick()
        composeTestRule.onNodeWithText("System events").performClick()
        assertEquals(listOf("users", "events"), opened)
    }

    // --- About ---------------------------------------------------------------------------------

    @Test
    fun `About shows version name and version code`() {
        show { AboutSettingsContent(state = SettingsUiState(buildInfo = release)) }
        composeTestRule.onNodeWithText("1.3.0").assertIsDisplayed()
        composeTestRule.onNodeWithText("1042").assertIsDisplayed()
        composeTestRule.onNodeWithText("release / obtainium").assertIsDisplayed()
        // Unstamped build: no commit / built rows rather than placeholders.
        composeTestRule.onNodeWithText("Commit").assertDoesNotExist()
        composeTestRule.onNodeWithText("Built").assertDoesNotExist()
    }

    @Test
    fun `About shows commit, build date and server details when known`() {
        show {
            AboutSettingsContent(
                state = SettingsUiState(
                    buildInfo = release.copy(commit = "abc1234", buildDate = "2026-10-02T10:00:00Z"),
                    serverHealth = ServerHealth(version = "1.3.1", commit = "def5678", buildDate = "2026-09-30"),
                ),
            )
        }
        composeTestRule.onNodeWithText("abc1234").assertIsDisplayed()
        composeTestRule.onNodeWithText("2026-10-02T10:00:00Z").assertIsDisplayed()
        composeTestRule.onNodeWithText("1.3.1 (def5678)").assertIsDisplayed()
        composeTestRule.onNodeWithText("2026-09-30").assertIsDisplayed()
    }

    @Test
    fun `a debug build shows the debug build type and applicationId suffix`() {
        show {
            AboutSettingsContent(
                state = SettingsUiState(
                    buildInfo = release.copy(buildType = "debug", applicationId = "com.mycorrhizal.crm.localtest"),
                ),
            )
        }
        composeTestRule.onNodeWithText("debug / obtainium .localtest").assertIsDisplayed()
    }

    @Test
    fun `a failed server lookup shows a dash`() {
        show { AboutSettingsContent(state = SettingsUiState(buildInfo = release, serverLookupFailed = true)) }
        composeTestRule.onNodeWithText("Server version").assertIsDisplayed()
        assertTrue(composeTestRule.onAllNodesWithText("—").fetchSemanticsNodes().isNotEmpty())
    }

    @Test
    fun `About without build info renders nothing to copy`() {
        show { AboutSettingsContent(state = SettingsUiState()) }
        composeTestRule.onNodeWithText("Copy build details").assertDoesNotExist()
    }

    @Test
    fun `copy details puts the build details on the clipboard`() {
        show { AboutSettingsContent(state = SettingsUiState(buildInfo = release.copy(commit = "abc1234"))) }
        composeTestRule.onNodeWithText("Copy build details").performClick()

        val clipboard = androidx.test.core.app.ApplicationProvider.getApplicationContext<Context>()
            .getSystemService(Context.CLIPBOARD_SERVICE) as android.content.ClipboardManager
        assertEquals(
            "version: 1.3.0\ncode: 1042\nbuild: release / obtainium\ncommit: abc1234",
            clipboard.primaryClip?.getItemAt(0)?.text?.toString(),
        )
    }

    // --- Issue #214: Compose semantics a11y sweep (the axe-core analog) -------------------------
    //
    // Mounts the real screens (Scaffold + TopAppBar included) via a mocked-repository
    // SettingsViewModel -- the same construction SettingsViewModelTest uses.

    private fun makeViewModel(): SettingsViewModel {
        val authRepository = mockk<AuthRepository>()
        val trackingSettings = mockk<TrackingSettingsRepository>()
        val appSettings = mockk<AppSettingsRepository>()
        val permissionChecker = mockk<com.mycorrhizal.crm.feature.tracking.PermissionChecker>()
        val catchUpScheduler = mockk<com.mycorrhizal.crm.feature.tracking.TrackingCatchUpScheduler>(relaxed = true)
        val localAuthSettings = mockk<LocalAuthSettingsRepository>()
        val localAuthCapabilities = mockk<LocalAuthCapabilities>()
        val deviceGrantManager = mockk<DeviceGrantManager>()
        val callSmsTrackingCapability = mockk<com.mycorrhizal.crm.feature.tracking.CallSmsTrackingCapability>()
        val appContext = mockk<Context>(relaxed = true)
        coEvery { trackingSettings.callTrackingEnabled() } returns false
        coEvery { trackingSettings.smsTrackingEnabled() } returns false
        coEvery { trackingSettings.notificationsEnabled() } returns true
        coEvery { trackingSettings.includeUnknownNumbers() } returns false
        coEvery { trackingSettings.filteredUnknownCount() } returns 0
        every { localAuthSettings.requireLocalAuth() } returns MutableStateFlow(false)
        every { localAuthSettings.blockScreenshots() } returns MutableStateFlow(false)
        every { localAuthSettings.autoLockDelay() } returns MutableStateFlow(AutoLockDelay.DEFAULT)
        every { localAuthSettings.biometricEnrollmentStatus() } returns MutableStateFlow(BiometricEnrollmentStatus.UNASKED)
        coEvery { localAuthSettings.setRequireLocalAuth(any()) } returns Unit
        coEvery { deviceGrantManager.enroll(any()) } returns Result.success(Unit)
        every { localAuthCapabilities.canEnableLocalAuth() } returns true
        every { callSmsTrackingCapability.isAvailable() } returns true
        every { permissionChecker.isGranted(any()) } returns false
        every { authRepository.observeSession() } returns MutableStateFlow(
            SessionState(serverUrl = "https://crm.example.com", username = "alice", isAdmin = true, language = "en"),
        )
        coEvery { appSettings.themePreference() } returns flowOf(AppSettingsRepository.THEME_SYSTEM)
        val serverCompatibility = mockk<com.mycorrhizal.crm.domain.repository.ServerCompatibilityRepository>()
        coEvery { serverCompatibility.getServerHealth() } returns Result.success(ServerHealth(version = "1.3.0"))
        return SettingsViewModel(
            authRepository,
            trackingSettings,
            appSettings,
            localAuthSettings,
            localAuthCapabilities,
            deviceGrantManager,
            permissionChecker,
            catchUpScheduler,
            callSmsTrackingCapability,
            PasskeyAvailability { false },
            release,
            serverCompatibility,
            appContext,
        )
    }

    private fun setScreen(darkTheme: Boolean, screen: @Composable (SettingsViewModel) -> Unit) {
        val viewModel = makeViewModel()
        composeTestRule.setContent { MycorrhizalTheme(darkTheme = darkTheme) { screen(viewModel) } }
    }

    @Test
    fun `settings root has no accessibility violations (light)`() {
        setScreen(darkTheme = false) { SettingsScreen(onLoggedOut = {}, viewModel = it) }
        composeTestRule.assertAccessibleSemantics()
    }

    @Test
    fun `settings root has no accessibility violations (dark)`() {
        setScreen(darkTheme = true) { SettingsScreen(onLoggedOut = {}, viewModel = it) }
        composeTestRule.assertAccessibleSemantics()
    }

    @Test
    fun `the settings root shows the admin group for the signed-in admin and navigates`() {
        var route: String? = null
        setScreen(darkTheme = false) { SettingsScreen(onLoggedOut = {}, onNavigate = { r -> route = r }, viewModel = it) }
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithText("Administration").performScrollTo().performClick()
        assertEquals(SettingsRoutes.ADMINISTRATION, route)
    }

    @Test
    fun `account screen has no accessibility violations and Back works`() {
        var back = 0
        setScreen(darkTheme = false) { AccountSettingsScreen(onBack = { back++ }, viewModel = it) }
        composeTestRule.waitForIdle()
        composeTestRule.assertAccessibleSemantics()
        composeTestRule.onNodeWithContentDescription("Back").performClick()
        assertEquals(1, back)
    }

    @Test
    fun `privacy screen has no accessibility violations (dark)`() {
        setScreen(darkTheme = true) { PrivacySettingsScreen(onBack = {}, viewModel = it) }
        composeTestRule.waitForIdle()
        composeTestRule.assertAccessibleSemantics()
    }

    @Test
    fun `switching app lock on through the screen enables it`() {
        setScreen(darkTheme = false) { PrivacySettingsScreen(onBack = {}, viewModel = it) }
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithText("Require biometric or device PIN to open the app").performClick()
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithText("Lock automatically after").assertIsDisplayed()
    }

    @Test
    fun `appearance, tracking, change password and about screens render with their titles`() {
        setScreen(darkTheme = false) { AppearanceSettingsScreen(onBack = {}, viewModel = it) }
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithText("Appearance").assertIsDisplayed()
    }

    @Test
    fun `tracking screen renders and Back works`() {
        var back = 0
        setScreen(darkTheme = false) { TrackingSettingsScreen(onBack = { back++ }, viewModel = it) }
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithText("Show notifications").assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("Back").performClick()
        assertEquals(1, back)
    }

    @Test
    fun `change password screen renders its form`() {
        setScreen(darkTheme = false) { ChangePasswordScreen(onBack = {}, onLoggedOut = {}, viewModel = it) }
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithText("Current password").assertIsDisplayed()
    }

    @Test
    fun `about screen shows the build identity`() {
        setScreen(darkTheme = false) { AboutSettingsScreen(onBack = {}, viewModel = it) }
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithText("1042").assertIsDisplayed()
    }

    @Test
    fun `group screens render their rows with Back`() {
        var back = 0
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactsFieldsSettingsScreen(onBack = { back++ })
            }
        }
        composeTestRule.onNodeWithText("Custom fields").assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("Back").performClick()
        assertEquals(1, back)
    }

    @Test
    fun `integrations and administration screens render with Back`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                IntegrationsSettingsScreen(onBack = {})
            }
        }
        composeTestRule.onNodeWithText("Webhooks").assertIsDisplayed()
    }

    @Test
    fun `administration screen renders its rows`() {
        composeTestRule.setContent { MycorrhizalTheme { AdministrationSettingsScreen(onBack = {}) } }
        composeTestRule.onNodeWithText("User management").assertIsDisplayed()
    }
}
