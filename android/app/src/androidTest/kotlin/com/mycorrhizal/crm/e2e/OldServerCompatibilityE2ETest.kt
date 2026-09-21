package com.mycorrhizal.crm.e2e

import androidx.compose.ui.test.ExperimentalTestApi
import androidx.compose.ui.test.hasContentDescription
import androidx.compose.ui.test.hasSetTextAction
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextClearance
import androidx.compose.ui.test.performTextInput
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.mycorrhizal.crm.MainActivity
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assume.assumeTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Issue #914: no test ever booted a real *baseline* server, so the "a released
 * server at the floor still authenticates and serves the app" claim in
 * docs/client-compatibility-policy.md rested on the version-gate *logic* being
 * unit-tested (ServerCapabilitiesTest, CompatibilityOutcomeTest) plus a belief
 * that a real floor server's responses still parse the way that logic assumes.
 * Nothing ever proved that belief against an actual release binary.
 *
 * This test targets a THIRD backend instance (docker-compose.compat-test.yml,
 * port 7302), the pinned release image
 * `ghcr.io/drewbrunning/mycorrhizal-crm:1.0.0` — the real release at this
 * app's migration floor
 * ([com.mycorrhizal.crm.domain.compat.ServerCapabilities.MIN_SUPPORTED_SERVER_VERSION];
 * raised from v0.6.0 at the 1.0 major, issue #1170) — not a synthetic stub. It
 * only exists in the CI legs that start it (android-e2e / android-e2e-min-sdk),
 * so the test skips when it is not reachable — a local run without that third
 * backend is not a failure. The companion synthetic-response case (a server
 * reporting a version BELOW the baseline, the blocking "server needs an
 * upgrade" gate) is [ServerTooOldGateE2ETest] — the newest below-baseline
 * server is the retired v0.9.x line, which is not worth pinning, so that case
 * is a stubbed /health instead.
 *
 * What this proves that no unit test can: a real server AT the baseline is not
 * refused — login succeeds and the app reaches the dashboard rather than the
 * "server too old" gate misfiring on the boundary version itself — and a
 * baseline capability (the API Tokens screen) loads against the real server's
 * actual /health response rather than a hand-constructed
 * [com.mycorrhizal.crm.model.network.ServerHealth] in a Robolectric test.
 *
 * The non-blocking "server could be upgraded" notice
 * (issue #528's [CompatibilityGate] third state) is deliberately NOT exercised
 * here: it only fires when the CLIENT's own versionName is newer than the
 * server's, and the debug APK's versionName (0.1.0, see
 * MycorrhizalAndroidApplicationPlugin.kt) is below every real released server
 * version — reproducing it against a real backend would need a
 * debug-build-only versionName override, which would break
 * [ForceUpdateGateE2ETest]'s literal "0.1.0" assertion. That state stays
 * covered at the unit level (CompatibilityOutcomeTest, MainViewModelTest).
 */
@OptIn(ExperimentalTestApi::class)
@RunWith(AndroidJUnit4::class)
class OldServerCompatibilityE2ETest {

    @get:Rule
    val compose = createAndroidComposeRule<MainActivity>()

    private val backend = E2eBackend(serverUrl = OLD_SERVER_URL)

    @Before
    fun setUp() {
        assumeTrue("old-server backend at $OLD_SERVER_URL is not reachable", backend.isReachable())
        backend.registerSeedUser()
        clearSession()
    }

    @After
    fun tearDown() {
        runCatching { clearSession() }
    }

    @Test
    fun realV100Server_authenticatesAndServesBaselineSurface() {
        waitForText("Sign in")
        replaceTextInField("Server URL", OLD_SERVER_URL)
        replaceTextInField("Username or email", E2eConfig.SEED_USERNAME)
        waitForText("Password")
        compose.onNode(hasText("Password") and hasSetTextAction()).performTextInput(E2eConfig.SEED_PASSWORD)
        compose.onNodeWithText("Sign in").performClick()

        // The real v1.0.0 baseline server is AT the floor, not below it: the
        // "server too old" gate must not fire, and the app must reach the
        // dashboard exactly as it would against the current test backend.
        waitForText("Dashboard")

        // Navigate Settings -> API Tokens — a baseline capability the real
        // floor server must serve.
        clickContentDescription("Menu")
        waitForText("Settings")
        onLastText("Settings").performClick()
        waitForText("API Tokens")
        compose.onNodeWithText("API Tokens").performScrollTo().performClick()

        // The screen loaded against the real server (empty token list — a
        // fresh per-container seed user).
        waitForText("No API tokens yet")
    }

    // --- minimal shared helpers (mirrors E2eBaseTest / ForceUpdateGateE2ETest) ---

    private fun waitForText(text: String, timeoutMs: Long = 30_000) {
        compose.waitUntilAtLeastOneExists(hasText(text), timeoutMs)
    }

    private fun replaceTextInField(label: String, text: String) {
        waitForText(label)
        val field = compose.onNodeWithText(label)
        field.performTextClearance()
        field.performTextInput(text)
    }

    private fun clickContentDescription(cd: String, timeoutMs: Long = 30_000) {
        compose.waitUntilAtLeastOneExists(hasContentDescription(cd), timeoutMs)
        compose.onNodeWithContentDescription(cd).performClick()
    }

    /** The last node matching [text] — drawer items render on top of the
     *  screen behind them, so they are the last match when the label also
     *  appears there. */
    private fun onLastText(text: String) =
        compose.onAllNodesWithText(text).let { it.get(it.fetchSemanticsNodes().size - 1) }

    private fun clearSession() {
        val session = compose.activity.sessionManager
        runBlocking {
            session.awaitHydrated()
            session.clearSession(keepServerUrl = false)
        }
        waitForText("Sign in")
    }

    private companion object {
        /** The dedicated pinned-v1.0.0 backend started by android-e2e's CI
         *  legs (see docker-compose.compat-test.yml) — reached via adb
         *  reverse like the suite's other backends. */
        const val OLD_SERVER_URL = "http://127.0.0.1:7302"
    }
}
