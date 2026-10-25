package com.mycorrhizal.crm.applock

import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.data.session.AppLockController
import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.LocalAuthCapabilities
import com.mycorrhizal.crm.domain.repository.SessionState
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.every
import io.mockk.mockk
import io.mockk.verify
import kotlinx.coroutines.flow.MutableStateFlow
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = android.app.Application::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class AppLockScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(
        state: AppLockUiState = AppLockUiState(username = "alice"),
        onUnlockRequest: () -> Unit = {},
        onLogout: () -> Unit = {}
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                AppLockContent(
                    state = state,
                    onUnlockRequest = onUnlockRequest,
                    onLogout = onLogout
                )
            }
        }
    }

    // Verify #1: the lock screen appears with the unlock affordance and no
    // session data beyond the username already shown on the OS lock screen.
    @Test
    fun `shows the lock heading and unlock button`() {
        setContent()

        composeTestRule.onNodeWithText("Your data is locked").assertIsDisplayed()
        composeTestRule.onNodeWithText("Signed in as alice").assertIsDisplayed()
        composeTestRule.onNodeWithText(
            "Unlock with your fingerprint, face or device PIN to continue."
        )
            .assertIsDisplayed()
        composeTestRule.onNodeWithText("Unlock").assertIsDisplayed()
        composeTestRule.onNodeWithText("Log out").assertIsDisplayed()
    }

    @Test
    fun `the unlock button requests the local auth prompt`() {
        var requested = false
        setContent(onUnlockRequest = { requested = true })

        composeTestRule.onNodeWithText("Unlock").performClick()

        assertTrue(requested)
    }

    @Test
    fun `logout requests the session logout`() {
        var loggedOut = false
        setContent(onLogout = { loggedOut = true })

        composeTestRule.onNodeWithText("Log out").performClick()

        assertTrue(loggedOut)
    }

    // An in-flight prompt shows no button (the OS dialog is up).
    @Test
    fun `an in-flight unlock hides the unlock button`() {
        setContent(state = AppLockUiState(isUnlocking = true, username = "alice"))

        composeTestRule.onNodeWithText("Unlock").assertDoesNotExist()
    }

    // A device that can no longer satisfy the gate shows the reason and keeps
    // logout as the only way off the screen.
    @Test
    fun `an unsupported device shows the reason and no unlock button`() {
        setContent(state = AppLockUiState(username = "alice", canAuthenticate = false))

        composeTestRule.onNodeWithText(
            "Fingerprint, face or device PIN unlock is not available on this device. " +
                "Log out to sign in with your password."
        ).assertIsDisplayed()
        composeTestRule.onNodeWithText("Unlock").assertDoesNotExist()
        composeTestRule.onNodeWithText("Log out").assertIsDisplayed()
    }

    // Verify: a transient auth failure is shown as an assertive live region so
    // TalkBack announces it (the screen has no auto-dismissing UI of its own).
    @Test
    fun `an auth failure is announced as an assertive live region`() {
        var shown = false
        composeTestRule.setContent {
            MycorrhizalTheme {
                AppLockContent(
                    state = AppLockUiState(errorRes = R.string.app_lock_auth_failed),
                    onUnlockRequest = {},
                    onLogout = {},
                    onErrorShown = { shown = true }
                )
            }
        }

        composeTestRule.onNodeWithText("Couldn't unlock. Please try again.")
            .assert(
                SemanticsMatcher.expectValue(
                    SemanticsProperties.LiveRegion,
                    LiveRegionMode.Assertive
                )
            )
        assertTrue(shown)
    }

    @Test
    fun `heading carries heading semantics`() {
        setContent()

        composeTestRule.onNodeWithText("Your data is locked")
            .assert(SemanticsMatcher.keyIsDefined(SemanticsProperties.Heading))
    }

    // --- The stateful host (prompter injected, so no BiometricPrompt/Activity is needed) ----------

    private class FakePrompter(private val outcome: AppLockAuthOutcome) : AppUnlockPrompter {
        var requests = 0
        var lastTitle: String? = null
        override suspend fun requestUnlock(
            title: String,
            subtitle: String,
            negativeButtonText: String?
        ): AppLockAuthOutcome {
            requests++
            lastTitle = title
            return outcome
        }
    }

    private fun hostViewModel(
        canAuthenticate: Boolean = true,
        controller: AppLockController = mockk(relaxed = true)
    ): AppLockViewModel {
        val authRepository = mockk<AuthRepository>(relaxed = true)
        val capabilities = mockk<LocalAuthCapabilities> {
            every { canEnableLocalAuth() } returns canAuthenticate
        }
        val sessionManager = mockk<SessionManager> {
            every { observeSession() } returns MutableStateFlow(
                SessionState(isLoggedIn = true, username = "alice")
            )
        }
        return AppLockViewModel(controller, authRepository, capabilities, sessionManager)
    }

    // The host auto-prompts once on first display and hands a success to the
    // controller; a cancel keeps the lock screen and Unlock retries. Added
    // because Compose codegen growth in this untested host pushed the file past
    // the coverage ratchet with no source change.
    @Test
    fun `the host prompts once on first display and reports a success to the controller`() {
        val controller = mockk<AppLockController>(relaxed = true)
        val prompter = FakePrompter(AppLockAuthOutcome.Success)
        composeTestRule.setContent {
            MycorrhizalTheme {
                AppLockScreen(
                    viewModel = hostViewModel(controller = controller),
                    prompter = prompter
                )
            }
        }
        composeTestRule.waitForIdle()

        assertEquals(1, prompter.requests)
        assertEquals("Your data is locked", prompter.lastTitle)
        verify(exactly = 1) { controller.onUserAuthenticated() }
    }

    @Test
    fun `a cancelled prompt keeps the lock screen and the unlock button retries`() {
        val controller = mockk<AppLockController>(relaxed = true)
        val prompter = FakePrompter(AppLockAuthOutcome.Cancelled)
        composeTestRule.setContent {
            MycorrhizalTheme {
                AppLockScreen(
                    viewModel = hostViewModel(controller = controller),
                    prompter = prompter
                )
            }
        }
        composeTestRule.waitForIdle()
        assertEquals(1, prompter.requests)
        verify(exactly = 0) { controller.onUserAuthenticated() }

        composeTestRule.onNodeWithText("Unlock").performClick()
        composeTestRule.waitForIdle()

        assertEquals(2, prompter.requests)
        verify(exactly = 0) { controller.onUserAuthenticated() }
        composeTestRule.onNodeWithText("Your data is locked").assertIsDisplayed()
    }

    @Test
    fun `the host does not prompt when the device cannot authenticate`() {
        val prompter = FakePrompter(AppLockAuthOutcome.Success)
        composeTestRule.setContent {
            MycorrhizalTheme {
                AppLockScreen(
                    viewModel = hostViewModel(canAuthenticate = false),
                    prompter = prompter
                )
            }
        }
        composeTestRule.waitForIdle()

        assertEquals(0, prompter.requests)
    }
}
