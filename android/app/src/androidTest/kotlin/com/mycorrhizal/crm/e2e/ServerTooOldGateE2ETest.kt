package com.mycorrhizal.crm.e2e

import androidx.compose.ui.test.ExperimentalTestApi
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextClearance
import androidx.compose.ui.test.performTextInput
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.mycorrhizal.crm.MainActivity
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Issue #914 (the below-baseline half; the real-server half is
 * [OldServerCompatibilityE2ETest]): issue #692's blocking "server needs an
 * upgrade" gate ([com.mycorrhizal.crm.compat.ServerTooOldScreen]) fires when a
 * configured server's /health reports a version older than this app's
 * baseline ([com.mycorrhizal.crm.domain.compat.ServerCapabilities.MIN_SUPPORTED_SERVER_VERSION],
 * 1.0.0 since issue #1170). The baseline moves only at a major and no released
 * server is actually below it right now, so there is no real historical image
 * to boot for this case the way [OldServerCompatibilityE2ETest] boots a real
 * v1.0.0. A minimal in-process HTTP stub serving a synthetic below-floor
 * /health response is the real equivalent: this drives the actual app, the
 * actual network stack and the actual [ServerTooOldScreen] composable against
 * a real HTTP response, only the response body is synthetic rather than a live
 * backend's.
 *
 * The gate logic itself is covered at the unit level
 * (ServerCapabilitiesTest, CompatibilityOutcomeTest, ServerTooOldScreenTest);
 * this pins the one thing those cannot: the real app, a real HTTP round trip,
 * and the real pre-login root-surface swap.
 *
 * Mirrors [ForceUpdateGateE2ETest]'s structure (the sibling blocking gate for
 * a client below the server's floor) — same helpers, same pre-login escape
 * hatch shape.
 */
@OptIn(ExperimentalTestApi::class)
@RunWith(AndroidJUnit4::class)
class ServerTooOldGateE2ETest {

    @get:Rule
    val compose = createAndroidComposeRule<MainActivity>()

    private lateinit var oldServerStub: MockWebServer

    @Before
    fun setUp() {
        oldServerStub = MockWebServer()
        // Several identical responses queued: Compose's text-input dispatch
        // isn't guaranteed to be a single character-count edit, and the
        // session persists+re-checks on every distinct URL value observed —
        // one enqueued response risks the second request finding an empty
        // queue (MockWebServer throws) rather than reproducing what a real
        // repeatedly-polled server would just keep answering.
        repeat(10) {
            oldServerStub.enqueue(
                MockResponse()
                    .setResponseCode(200)
                    .setBody("""{"status":"healthy","version":"$BELOW_FLOOR_VERSION"}"""),
            )
        }
        oldServerStub.start()
        clearSession()
    }

    @After
    fun tearDown() {
        runCatching { clearSession() }
        runCatching { oldServerStub.shutdown() }
    }

    @Test
    fun configuringBelowBaselineServer_showsServerTooOldGateAndBlocksDashboard() {
        waitForText("Sign in")
        val serverUrl = stubUrl()

        // Configuring the below-baseline server URL raises the pre-login
        // gate: /health (resolved on URL change) reports a version below
        // this app's 1.0.0 baseline, so the auth form is swapped for the
        // blocking screen instead of authenticating against a contract this
        // app does not implement.
        replaceTextInField("Server URL", serverUrl)

        waitForText("Server needs an upgrade")
        waitForSubstringText(BASELINE_VERSION)
        waitForSubstringText(BELOW_FLOOR_VERSION)
        waitForText("Server: $serverUrl")

        // No credentials were ever offered against the refused server, and
        // the app never proceeds to the dashboard.
        compose.waitUntil(5_000) {
            compose.onAllNodesWithText("Username or email").fetchSemanticsNodes().isEmpty() &&
                compose.onAllNodesWithText("Dashboard").fetchSemanticsNodes().isEmpty()
        }
    }

    @Test
    fun preLoginServerTooOldGate_returnsToTheAuthFlow() {
        waitForText("Sign in")
        replaceTextInField("Server URL", stubUrl())

        waitForText("Server needs an upgrade")

        // No session exists behind a pre-login gate, so the escape is "Back
        // to sign in" (to point at a different server), not "Log out" —
        // same shape as the force-update gate's pre-login escape.
        waitForText("Back to sign in")
        compose.onNodeWithText("Back to sign in").performClick()

        waitForText("Sign in")
    }

    // --- minimal shared helpers (mirrors ForceUpdateGateE2ETest) -------------

    private fun stubUrl(): String = "http://127.0.0.1:${oldServerStub.port}"

    private fun waitForText(text: String, timeoutMs: Long = 30_000) {
        compose.waitUntilAtLeastOneExists(hasText(text), timeoutMs)
    }

    private fun waitForSubstringText(text: String, timeoutMs: Long = 30_000) {
        compose.waitUntilAtLeastOneExists(hasText(text, substring = true), timeoutMs)
    }

    private fun replaceTextInField(label: String, text: String) {
        waitForText(label)
        val field = compose.onNodeWithText(label)
        field.performTextClearance()
        field.performTextInput(text)
    }

    private fun clearSession() {
        val session = compose.activity.sessionManager
        runBlocking {
            session.awaitHydrated()
            session.clearSession(keepServerUrl = false)
        }
        waitForText("Sign in")
    }

    private companion object {
        /** Mirrors [com.mycorrhizal.crm.domain.compat.ServerCapabilities.MIN_SUPPORTED_SERVER_VERSION]
         *  as a literal — the required-version half of the rendered message. */
        const val BASELINE_VERSION = "1.0.0"

        /** A version below [BASELINE_VERSION] — the synthetic server's
         *  reported version (the retired 0.9.x line). */
        const val BELOW_FLOOR_VERSION = "0.9.0"
    }
}
