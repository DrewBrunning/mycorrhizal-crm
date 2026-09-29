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
        compose.onNodeWithText(given) // the prefilled search field
    }

    @Test
    fun warmRejectedLinkFromOutsideTheTaskLeavesTheStartScreenAlone() {
        viewCrossTask("mycorrhizal://settings")
        viewCrossTask("mycorrhizal://contacts/42/edit")
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

    /** `am start -a VIEW -d <uri>` from the shell: NEW_TASK, from outside the app's task. */
    private fun viewCrossTask(uri: String) {
        shell("am start -a android.intent.action.VIEW -d '$uri' -p $pkg")
    }

    /** `am start -a SEND` from the shell; [text] must be a single shell word. */
    private fun shareCrossTask(text: String) {
        require(text.none { it.isWhitespace() || it == '\'' })
        shell("am start -a android.intent.action.SEND -t text/plain -p $pkg --es android.intent.extra.TEXT '$text'")
    }

    /** Runs a shell command and drains its output so it has completed before returning. */
    private fun shell(command: String) {
        val pfd = instrumentation.uiAutomation.executeShellCommand(command)
        java.io.FileInputStream(pfd.fileDescriptor).use { it.readBytes() }
        pfd.close()
    }

    private fun mainActivities(): List<MainActivity> {
        val found = mutableListOf<MainActivity>()
        instrumentation.runOnMainSync {
            val monitor = ActivityLifecycleMonitorRegistry.getInstance()
            for (stage in Stage.values()) {
                if (stage == Stage.DESTROYED) continue
                monitor.getActivitiesInStage(stage).filterIsInstance<MainActivity>().forEach { found += it }
            }
        }
        return found
    }

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
    private fun finishAllActivities() {
        instrumentation.runOnMainSync { mainActivities().forEach { it.finishAndRemoveTask() } }
        val deadline = System.currentTimeMillis() + 10_000
        while (System.currentTimeMillis() < deadline && mainActivities().isNotEmpty()) Thread.sleep(200)
    }

    private fun waitFor(matcher: SemanticsMatcher, timeoutMs: Long = DEFAULT_TIMEOUT_MS) =
        compose.waitUntilAtLeastOneExists(matcher, timeoutMs)

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
