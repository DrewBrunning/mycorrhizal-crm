package com.mycorrhizal.crm.e2e

import androidx.compose.ui.test.ExperimentalTestApi
import androidx.compose.ui.test.hasSetTextAction
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
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
 * The locale is switched through the platform's per-app locale service (API 33+; the app
 * declares `android:localeConfig`), the same path as the system Settings per-app language picker.
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

    /** Runs `cmd locale set-app-locales` as the shell and waits for it to finish, then for the
     *  activity recreation it triggers to settle. */
    private fun setAppLocales(locales: String) {
        val pkg = compose.activity.packageName
        val automation = InstrumentationRegistry.getInstrumentation().uiAutomation
        val pfd = automation.executeShellCommand(
            "cmd locale set-app-locales $pkg --locales ${if (locales.isEmpty()) "\"\"" else locales}"
        )
        FileInputStream(pfd.fileDescriptor).use { it.readBytes() }
        pfd.close()
        compose.waitForIdle()
    }

    private companion object {
        const val WRONG_PASSWORD = "definitely-not-the-password"

        // core/ui values-de/strings.xml: login_sign_in, error_reason_auth_invalid_credentials.
        const val GERMAN_SIGN_IN = "Anmelden"
        const val GERMAN_INVALID_CREDENTIALS = "Ungültige E-Mail-Adresse oder ungültiges Passwort"

        // The server's English message, identical to values/strings.xml.
        const val ENGLISH_INVALID_CREDENTIALS = "Invalid email or password"
    }
}
