package com.mycorrhizal.crm.e2e

import androidx.compose.ui.test.ExperimentalTestApi
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.hasContentDescription
import androidx.compose.ui.test.hasSetTextAction
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.createEmptyComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextClearance
import androidx.compose.ui.test.performTextInput
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.runner.lifecycle.ActivityLifecycleMonitorRegistry
import androidx.test.runner.lifecycle.Stage
import com.mycorrhizal.crm.MainActivity
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Issue #1343: the cross-task delivery path the public `mycorrhizal://` VIEW filter
 * (#1269) and the ACTION_SEND share target (#1271) exist for.
 *
 * [DeepLinkE2eTest] / [ShareToCrmE2eTest] deliver from inside the running activity
 * (issue #1321) — that proves the in-app route but is NOT what a browser, another app,
 * a notification or `adb shell am start` do. Those start MainActivity from outside the
 * task, with `FLAG_ACTIVITY_NEW_TASK`, so the singleTask activity gets `onNewIntent`
 * (warm) or a fresh `onCreate` (cold). This class drives exactly that: the intents are
 * issued by `am start` through the shell (UiAutomation), which is what `adb shell am
 * start` runs.
 *
 * Teardown-hang guard (the #1321 problem): this class deliberately does not use an
 * ActivityScenarioRule/ActivityScenario (whose close waits for DESTROYED on an activity
 * a cross-task start may have left PAUSED). It uses [createEmptyComposeRule], starts
 * the app itself, and in [tearDown] finishes every MainActivity directly and only
 * polls (bounded, best effort) for them to go away — nothing can block.
 */
@OptIn(ExperimentalTestApi::class)
@RunWith(AndroidJUnit4::class)
class CrossTaskIntentE2eTest {

    @get:Rule
    val compose = createEmptyComposeRule()

    private val backend = E2eBackend()
    private val instrumentation = InstrumentationRegistry.getInstrumentation()
    private val pkg = instrumentation.targetContext.packageName

    private val given = "${E2eConfig.TEST_CONTACT_PREFIX} Cross ${System.nanoTime()}"
    private val surname = "Task"
    private val displayName = "$given $surname"
    private var contactId = 0L

    @Before
    fun setUp() {
        backend.registerSeedUser()
        backend.login()
        backend.cleanupTestContacts()
        contactId = backend.createContact(given, surname).id
        // Start the app from the shell too (a plain launcher-style start).
        shell("am start -n $pkg/com.mycorrhizal.crm.MainActivity")
        awaitActivity()
        clearSession()
        loginViaUi()
    }

    @After
    fun tearDown() {
        runCatching { backend.deleteContact(contactId) }
        runCatching { clearSession() }
        finishAllActivities()
    }

    // --- VIEW deep link (#1269) ----------------------------------------------

    @Test
    fun warmContactLinkFromOutsideTheTaskOpensTheContactAndIsConsumedOnce() {
        viewCrossTask("mycorrhizal://contacts/$contactId")
        waitFor(hasContentDescription("Mark $displayName as favorite"))

        // #1268 consume-once: the handled link is stripped from the retained intent...
        compose.waitForIdle()
        assertNull("a consumed deep link must not stay on the intent", activity()!!.intent.data)

        // ...so backing out and recreating the activity does not replay it.
        clickBack()
        waitForText("Dashboard")
        instrumentation.runOnMainSync { activity()!!.recreate() }
        awaitActivity()
        waitForText("Dashboard")
        compose.waitForIdle()
        assertEquals(
            "a recreate must not re-navigate to the consumed link",
            0,
            compose.onAllNodes(hasContentDescription("Mark $displayName as favorite")).fetchSemanticsNodes().size,
        )
    }

    @Test
    fun warmSearchLinkFromOutsideTheTaskFiltersTheList() {
        viewCrossTask("mycorrhizal://search?q=${given.replace(" ", "%20")}")
        waitForText(displayName)
        // The prefilled search field: an editable node holding exactly the query.
        waitFor(hasSetTextAction() and hasText(given))
    }

    // Issue #1399: the app is ALREADY on Contacts (empty search) when the link arrives —
    // the reused/restored Contacts entry used to keep its old, empty query.
    //
    // Deliberately does NOT press HOME between the two links. The pre-#1639 version
    // backgrounded the app ("leaving it on Contacts") to model a link delivered to a
    // backgrounded task, but that injects a *restore* path the test cannot control: on a
    // contended emulator the process can be killed while backgrounded, so the second
    // `am start` recreates the activity with saved state and the app's ADR-0029 consume-once
    // guard skips the new link, leaving the search field empty for the full timeout
    // (`ComposeTimeoutException` at 30s, both attempts of run 37929593004). The contract
    // #1399 actually pins — a link arriving while the Contacts entry is already on the back
    // stack must still apply its query — is fully exercised by the warm `onNewIntent` path
    // from Contacts, which is what a foreground/browser deep link does. The cold path has
    // its own test (coldContactLinkFromOutsideTheTaskOpensTheContact) and the recreate
    // consume-once behaviour is pinned in warmContactLinkFromOutsideTheTaskOpensTheContactAndIsConsumedOnce.
    @Test
    fun warmSearchLinkWhileAlreadyOnContactsFiltersTheList() {
        viewCrossTask("mycorrhizal://search") // empty q -> plain Contacts, empty search field
        waitForText(displayName)
        compose.waitForIdle()
        viewCrossTask("mycorrhizal://search?q=${given.replace(" ", "%20")}")
        waitFor(hasSetTextAction() and hasText(given))
        waitForText(displayName)
    }

    @Test
    fun warmRejectedLinkFromOutsideTheTaskLeavesTheStartScreenAlone() {
        viewCrossTask("mycorrhizal://settings", mustStart = false) // matches no filter at all
        viewCrossTask("mycorrhizal://contacts/42/edit") // resolves, then the app rejects the route
        compose.waitForIdle()
        waitForText("Dashboard")
        assertEquals(
            "a rejected link must not navigate",
            0,
            compose.onAllNodes(hasContentDescription("Mark $displayName as favorite")).fetchSemanticsNodes().size,
        )
    }

    @Test
    fun coldContactLinkFromOutsideTheTaskOpensTheContact() {
        finishAllActivities()
        viewCrossTask("mycorrhizal://contacts/$contactId")
        waitFor(hasContentDescription("Mark $displayName as favorite"), COLD_TIMEOUT_MS)
    }

    // --- ACTION_SEND share target (#1271) ------------------------------------

    @Test
    fun warmShareFromOutsideTheTaskOpensThePickerThenAPrefilledNoteForm() {
        val shared = "crosstaskshare"
        shareCrossTask(shared)
        waitForText("Share to…")
        waitForText(displayName)
        compose.onNodeWithText(displayName).performClick()
        waitForText(shared)

        // Nothing is written until the user saves; backing out discards the draft.
        clickBack()
        compose.waitForIdle()
        assertEquals(emptyList<String>(), backend.noteContents(contactId))
        assertNull(
            "a consumed share must not stay on the intent",
            activity()!!.intent.getStringExtra("android.intent.extra.TEXT"),
        )
    }

    @Test
    fun coldShareFromOutsideTheTaskOpensThePicker() {
        finishAllActivities()
        shareCrossTask("crosstaskcoldshare")
        waitFor(hasText("Share to…"), COLD_TIMEOUT_MS)
        assertEquals(emptyList<String>(), backend.noteContents(contactId))
    }

    // --- helpers -------------------------------------------------------------

    /**
     * `am start -a VIEW -d <uri>` from the shell: NEW_TASK, from outside the app's task.
     *
     * [mustStart]: fail the test if `am` could not start anything (e.g. the URI matched no
     * intent filter). A rejected-link test passes `false` for links the filter deliberately
     * does not resolve.
     */
    private fun viewCrossTask(uri: String, mustStart: Boolean = true) {
        require(uri.none { it.isWhitespace() || it == '\'' }) { "URI must be a single unquoted shell word" }
        val out = shell("am start -W -a android.intent.action.VIEW -d $uri -p $pkg")
        if (mustStart) assertStarted(out, uri)
    }

    /** `am start -a SEND` from the shell; [text] must be a single shell word. */
    private fun shareCrossTask(text: String) {
        require(text.none { it.isWhitespace() || it == '\'' }) { "text must be a single unquoted shell word" }
        val out = shell("am start -W -a android.intent.action.SEND -t text/plain -p $pkg --es android.intent.extra.TEXT $text")
        assertStarted(out, text)
    }

    /**
     * Runs a shell command and returns its output once it has completed.
     *
     * UiAutomation.executeShellCommand does NOT run the command through a shell: it splits
     * on whitespace and passes quote characters through literally. Quoting an argument
     * (`-d 'mycorrhizal://…'`) therefore sent the URI `'mycorrhizal://…'` — unresolvable
     * (`am start` result -91, nothing started) — and `--es … 'text'` delivered the quotes as
     * part of the text (issue #1372). Pass bare single-word arguments.
     */
    private fun shell(command: String): String {
        val pfd = instrumentation.uiAutomation.executeShellCommand(command)
        val out = java.io.FileInputStream(pfd.fileDescriptor).use { String(it.readBytes()) }
        pfd.close()
        return out
    }

    /**
     * `am start -W` prints `Status: ok` on stdout once the activity has started. A failure
     * (`Error: Activity not started, unable to resolve Intent …`) goes to STDERR, which
     * [shell] does not read — so success is asserted on stdout, not failure looked for.
     */
    private fun assertStarted(amOutput: String, what: String) {
        assertTrue("`am start -W` for $what did not report `Status: ok`; it printed: [$amOutput]", amOutput.contains("Status: ok"))
    }

    private fun mainActivities(): List<MainActivity> = E2eActivities.mainActivities()

    private fun activity(): MainActivity? = mainActivities().firstOrNull()

    private fun awaitActivity(timeoutMs: Long = COLD_TIMEOUT_MS) {
        val deadline = System.currentTimeMillis() + timeoutMs
        while (System.currentTimeMillis() < deadline) {
            var resumed = false
            instrumentation.runOnMainSync {
                resumed = ActivityLifecycleMonitorRegistry.getInstance()
                    .getActivitiesInStage(Stage.RESUMED).filterIsInstance<MainActivity>().isNotEmpty()
            }
            if (resumed) return
            Thread.sleep(200)
        }
        error("MainActivity never resumed")
    }

    /** Finishes every live MainActivity directly and polls (bounded, never throws) for them to go. */
    private fun finishAllActivities() = E2eActivities.finishAllMainActivities()

    /**
     * Polls until at least one node matches [matcher].
     *
     * Deliberately not `compose.waitUntilAtLeastOneExists`: when a cross-task
     * `am start` is (re)launching MainActivity there is a frame with no compose
     * root at all, and `waitUntilAtLeastOneExists`'s condition lets the
     * "No compose hierarchies found in the app" `IllegalStateException` escape
     * on its first evaluation instead of retrying -- which flaked the cross-task
     * tests (#1438, run 37195560627). Catching it here and reporting `false`
     * lets the poll ride out that window; a hierarchy that never comes back
     * still times out with the real wait error.
     */
    private fun waitFor(matcher: SemanticsMatcher, timeoutMs: Long = DEFAULT_TIMEOUT_MS) {
        compose.waitUntil(timeoutMs) {
            try {
                compose.onAllNodes(matcher).fetchSemanticsNodes().isNotEmpty()
            } catch (_: IllegalStateException) {
                false
            }
        }
    }

    private fun waitForText(text: String, timeoutMs: Long = DEFAULT_TIMEOUT_MS) =
        waitFor(hasText(text), timeoutMs)

    private fun clickBack() {
        waitFor(hasContentDescription("Back"))
        compose.onNodeWithContentDescription("Back").performClick()
    }

    private fun clearSession() {
        val session = activity()!!.sessionManager
        runBlocking {
            session.awaitHydrated()
            session.clearSession()
        }
        waitForText("Sign in")
    }

    private fun replaceTextInField(label: String, text: String) {
        waitForText(label)
        val field = compose.onNodeWithText(label)
        field.performTextClearance()
        field.performTextInput(text)
    }

    private fun loginViaUi() {
        waitForText("Sign in")
        replaceTextInField("Server URL", E2eConfig.serverUrl)
        replaceTextInField("Username or email", E2eConfig.SEED_USERNAME)
        waitForText("Password")
        compose.onNode(hasText("Password") and hasSetTextAction()).performTextInput(E2eConfig.SEED_PASSWORD)
        compose.onNodeWithText("Sign in").performClick()
        waitForText("Dashboard")
    }

    private companion object {
        const val DEFAULT_TIMEOUT_MS = 30_000L
        const val COLD_TIMEOUT_MS = 45_000L
    }
}
