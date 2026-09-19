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
    private const val DASHBOARD_FEED_TIMEOUT_MS = 45_000L

    /**
     * No-op if the dashboard is already showing (later benchmark iterations
     * reuse the persisted session); otherwise fills and submits the login form
     * and waits for the dashboard. Throws with a specific message on any step
     * that does not materialise, so a harness break is obvious in the log.
     */
    fun ensureOnDashboard(device: UiDevice) {
        if (device.wait(Until.hasObject(By.text(BenchmarkConfig.DASHBOARD_TITLE)), 3_000) == true) {
            waitForFeed(device)
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

        // Dismiss the IME so it cannot cover the button, then submit. Wait for
        // the button rather than a single `findObject`: the accessibility tree
        // is briefly unstable while the IME window is torn down, and a
        // no-retry lookup there is exactly the "'Sign in' button vanished"
        // flake this replaced.
        device.pressBack()
        val signIn = checkNotNull(
            device.wait(Until.findObject(By.text(SIGN_IN)), LOGIN_SCREEN_TIMEOUT_MS),
        ) { "'$SIGN_IN' button did not reappear after dismissing the keyboard" }
        // Let the IME-dismissal relayout settle before tapping coordinates.
        device.waitForIdle()
        signIn.click()

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
        waitForFeed(device)
    }

    /**
     * Blocks until the dashboard's feed is actually scrollable. The app-bar
     * title goes up as soon as the dashboard screen composes, while the widgets
     * are still the loading skeleton (which is not a scroll container) — so
     * `DashboardActions.scrollFeed`'s own 5s lookup used to race the load. The
     * seeded feed (see [SeedBackend]) overflows the viewport, so the single
     * dashboard `LazyColumn` reports scrollable once it renders.
     */
    private fun waitForFeed(device: UiDevice) {
        checkNotNull(device.wait(Until.findObject(By.scrollable(true)), DASHBOARD_FEED_TIMEOUT_MS)) {
            "dashboard feed did not render within ${DASHBOARD_FEED_TIMEOUT_MS}ms"
        }
    }

    private fun UiObject2.replaceText(value: String) {
        click()
        // UiAutomator's setText replaces existing content; clear() first guards
        // against a field the app pre-populated (e.g. a remembered server URL).
        clear()
        text = value
    }
}
