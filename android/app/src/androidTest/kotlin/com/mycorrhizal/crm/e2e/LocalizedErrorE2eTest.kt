package com.mycorrhizal.crm.e2e

import android.os.Build
import androidx.compose.ui.test.ExperimentalTestApi
import androidx.compose.ui.test.hasSetTextAction
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.mycorrhizal.crm.data.repository.AppLocale
import java.io.FileInputStream
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Issue #2379: a real server `reason_code` renders as translated text on a device.
 *
 * The Robolectric screen/view-model tests (#2097, #2106) use fake errors; nothing else follows
 * the real server's `reason_code` through the app to the on-screen string. A wrong-password
 * login is a UI-reachable trigger: the server answers
 * `{"error":{"code":"INVALID_CREDENTIALS","message":"Invalid email or password",
 * "reason_code":"auth_invalid_credentials"}}`. The English server message equals the English
 * string, so the app runs in German: seeing the German text proves the reason-code lookup ran,
 * and the absence of the English server message proves it did not fall back to it.
 *
 * The locale is switched through the platform's per-app locale service on API 33+ (the app
 * declares `android:localeConfig`), the same path as the system Settings per-app language picker,
 * and through the app's in-app language override below that (the minSdk 26 job).
 * [E2eBaseTest]'s rule launches the activity before `@Before`, so the switch recreates the
 * running activity rather than preceding its launch.
 */
@OptIn(ExperimentalTestApi::class)
@RunWith(AndroidJUnit4::class)
class LocalizedErrorE2eTest : E2eBaseTest() {

    /** Not [E2eBaseTest.e2eSetUp]: that logs in through the English labels. */
    override fun e2eSetUp() {
        backend.registerSeedUser()
        backend.login()
        backend.cleanupTestContacts()
        setAppLocales("de")
        // Drop any persisted session so the login screen shows, now in German.
        val session = compose.activity.sessionManager
        runBlocking {
            session.awaitHydrated()
            session.clearSession()
        }
        waitForText(GERMAN_SIGN_IN)
    }

    /** Restores English first so the base teardown's English "Sign in" wait works. */
    override fun e2eTearDown() {
        runCatching { setAppLocales("") }
        super.e2eTearDown()
    }

    @Test
    fun wrongPasswordShowsTheTranslatedReasonCodeNotTheServerMessage() {
        replaceTextInField("Server-URL", E2eConfig.serverUrl)
        replaceTextInField("Benutzername oder E-Mail", E2eConfig.SEED_EMAIL)
        // "Passwort" also labels the mode-segmented button (LoginScreenTest's idiom).
        waitForText("Passwort")
        compose.onNode(hasText("Passwort") and hasSetTextAction())
            .performTextInput(WRONG_PASSWORD)
        compose.onNodeWithText(GERMAN_SIGN_IN).performClick()

        waitForText(GERMAN_INVALID_CREDENTIALS)
        assertTrue(
            "the English server message must not be shown",
            compose.onAllNodesWithText(ENGLISH_INVALID_CREDENTIALS).fetchSemanticsNodes().isEmpty()
        )
    }

    /** Runs `cmd locale set-app-locales` as the shell, then waits (bounded) for the activity
     *  recreation it triggers. Every blocking step is bounded: #2494 hung the whole job for an
     *  hour in this test's teardown, so a stuck shell read or idle wait must throw (teardown
     *  swallows it) rather than block forever. */
    private fun setAppLocales(locales: String) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU) {
            setInAppLocale(locales.ifEmpty { null })
            return
        }
        val before = compose.activity
        val pkg = before.packageName
        val automation = InstrumentationRegistry.getInstrumentation().uiAutomation
        val pfd = automation.executeShellCommand(
            "cmd locale set-app-locales $pkg --locales ${if (locales.isEmpty()) "\"\"" else locales}"
        )
        // The read ends when the shell command exits; run it off-thread so a command that never
        // closes its output cannot block this one.
        val reader = Thread {
            runCatching { FileInputStream(pfd.fileDescriptor).use { it.readBytes() } }
        }.apply { isDaemon = true; start() }
        reader.join(SHELL_TIMEOUT_MS)
        val stuck = reader.isAlive
        runCatching { pfd.close() }
        check(!stuck) { "cmd locale set-app-locales did not finish within ${SHELL_TIMEOUT_MS}ms" }
        awaitRecreated(before)
    }

    /** Below API 33 there is no per-app locale service (`cmd locale` is absent, so the minSdk 26
     *  job timed out waiting for German). The app's own in-app override applies there instead:
     *  persist it, update the synchronous cache `attachBaseContext` reads, then recreate. */
    private fun setInAppLocale(tag: String?) {
        val activity = compose.activity
        runBlocking { activity.appSettings.setLanguageOverride(tag) }
        AppLocale.languageTag = tag
        InstrumentationRegistry.getInstrumentation().runOnMainSync { activity.recreate() }
        awaitRecreated(activity)
    }

    /** Bounded replacement for `compose.waitForIdle()`: waits until the rule's activity is a
     *  new, resumed instance. Throws on timeout instead of blocking. */
    private fun awaitRecreated(old: androidx.activity.ComponentActivity) {
        compose.waitUntil(RECREATE_TIMEOUT_MS) {
            runCatching {
                val now = compose.activity
                now !== old &&
                    now.lifecycle.currentState.isAtLeast(androidx.lifecycle.Lifecycle.State.RESUMED)
            }.getOrDefault(false)
        }
    }

    private companion object {
        const val SHELL_TIMEOUT_MS = 15_000L
        const val RECREATE_TIMEOUT_MS = 15_000L

        const val WRONG_PASSWORD = "definitely-not-the-password"

        // core/ui values-de/strings.xml: login_sign_in, error_reason_auth_invalid_credentials.
        const val GERMAN_SIGN_IN = "Anmelden"
        const val GERMAN_INVALID_CREDENTIALS = "Ungültige E-Mail-Adresse oder ungültiges Passwort"

        // The server's English message, identical to values/strings.xml.
        const val ENGLISH_INVALID_CREDENTIALS = "Invalid email or password"
    }
}
