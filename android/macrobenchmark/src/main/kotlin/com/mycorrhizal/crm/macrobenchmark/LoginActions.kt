package com.mycorrhizal.crm.macrobenchmark

import androidx.test.uiautomator.By
import androidx.test.uiautomator.UiDevice
import androidx.test.uiautomator.UiObject2
import androidx.test.uiautomator.Until

/**
 * Drives the real login screen far enough to land on the dashboard (issue
 * #263). UiAutomator, not Compose test APIs — a macrobenchmark runs against a
 * separately-built, minified `benchmark` APK it cannot instrument in-process.
 *
 * The Compose text fields expose their editable node as `android.widget.EditText`
 * to accessibility; on the password-mode login screen there are exactly three,
 * top-to-bottom: server URL, username/email, password.
 */
internal object LoginActions {

    private const val EDIT_TEXT = "android.widget.EditText"
    private const val SIGN_IN = "Sign in"

    private const val LOGIN_SCREEN_TIMEOUT_MS = 20_000L
    private const val DASHBOARD_TIMEOUT_MS = 45_000L

    /**
     * No-op if the dashboard is already showing (later benchmark iterations
     * reuse the persisted session); otherwise fills and submits the login form
     * and waits for the dashboard. Throws with a specific message on any step
     * that does not materialise, so a harness break is obvious in the log.
     */
    fun ensureOnDashboard(device: UiDevice) {
        if (device.wait(Until.hasObject(By.text(BenchmarkConfig.DASHBOARD_TITLE)), 3_000) == true) {
            return
        }

        check(device.wait(Until.hasObject(By.text(SIGN_IN)), LOGIN_SCREEN_TIMEOUT_MS) == true) {
            "login screen ('$SIGN_IN') never appeared"
        }

        val fields = device.findObjects(By.clazz(EDIT_TEXT))
        check(fields.size >= 3) {
            "expected 3 login fields (server URL, username, password), found ${fields.size}"
        }
        fields[0].replaceText(BenchmarkConfig.serverUrl)
        fields[1].replaceText(BenchmarkConfig.SEED_USERNAME)
        fields[2].replaceText(BenchmarkConfig.SEED_PASSWORD)

        // Dismiss the IME so it cannot cover the button, then submit.
        device.pressBack()
        checkNotNull(device.findObject(By.text(SIGN_IN))) { "'$SIGN_IN' button vanished" }.click()

        check(
            device.wait(
                Until.hasObject(By.text(BenchmarkConfig.DASHBOARD_TITLE)),
                DASHBOARD_TIMEOUT_MS,
            ) == true,
        ) {
            "dashboard ('${BenchmarkConfig.DASHBOARD_TITLE}') did not load within " +
                "${DASHBOARD_TIMEOUT_MS}ms of sign-in"
        }
        device.waitForIdle()
    }

    private fun UiObject2.replaceText(value: String) {
        click()
        // UiAutomator's setText replaces existing content; clear() first guards
        // against a field the app pre-populated (e.g. a remembered server URL).
        clear()
        text = value
    }
}
