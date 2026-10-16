package com.mycorrhizal.crm.security

import android.os.Build
import android.view.WindowManager
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.mycorrhizal.crm.MainActivity
import com.mycorrhizal.crm.domain.repository.LocalAuthSettingsRepository
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Issue #507 / #1741: MASVS-L2 resilience re-evaluation. Tapjacking protection
 * (`filterTouchesWhenObscured`) is unconditional; screenshot prevention
 * (`FLAG_SECURE`) is an opt-in, device-local setting (default off, see
 * `docs/security/masvs-l1.md` P6 and `docs/security/threat-model.md` gating
 * decision 3), and on API 33+ the recents thumbnail is always blanked via
 * `setRecentsScreenshotEnabled(false)`. All of it is window-level state set in
 * `MainActivity.onCreate`, so it can only be verified against a real Window —
 * a Robolectric JVM test's shadow Window does not model FLAG_SECURE or
 * touch-filtering — hence this is an instrumented (device/emulator) test, not
 * a unit test. None of it requires the docker-compose backend or a logged-in
 * session (the flags are set before any network call), so this does not use
 * `E2eBaseTest`.
 */
@RunWith(AndroidJUnit4::class)
class ScreenResilienceTest {

    private lateinit var settings: LocalAuthSettingsRepository

    @Before
    fun resetSetting() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity { settings = it.localAuthSettings }
        }
        runBlocking { settings.setBlockScreenshots(false) }
    }

    @After
    fun restoreSetting() {
        runBlocking { settings.setBlockScreenshots(false) }
    }

    private fun ActivityScenario<MainActivity>.hasFlagSecure(): Boolean {
        var secure = false
        onActivity { secure = (it.window.attributes.flags and WindowManager.LayoutParams.FLAG_SECURE) != 0 }
        return secure
    }

    private fun ActivityScenario<MainActivity>.awaitFlagSecure(expected: Boolean) {
        val deadline = System.currentTimeMillis() + 5_000
        while (hasFlagSecure() != expected && System.currentTimeMillis() < deadline) {
            Thread.sleep(50)
        }
        assertEquals("FLAG_SECURE after toggling the setting", expected, hasFlagSecure())
    }

    @Test
    fun screenshotsAreAllowedByDefault() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            assertFalse(
                "FLAG_SECURE must be absent while 'Block screenshots and screen recording' is off",
                scenario.hasFlagSecure(),
            )
        }
    }

    @Test
    fun settingOnSetsFlagSecureBeforeTheFirstFrameOnColdStart() {
        runBlocking { settings.setBlockScreenshots(true) }
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            // No waiting: the stored value must already be applied when the
            // activity first becomes available to the test.
            assertTrue("FLAG_SECURE must be set on cold start when the setting is on", scenario.hasFlagSecure())
        }
    }

    @Test
    fun togglingTheSettingFlipsTheFlagLiveWithoutARestart() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            assertFalse(scenario.hasFlagSecure())
            runBlocking { settings.setBlockScreenshots(true) }
            scenario.awaitFlagSecure(true)
            runBlocking { settings.setBlockScreenshots(false) }
            scenario.awaitFlagSecure(false)
        }
    }

    @Test
    fun recentsScreenshotsAreDisabledOnApi33PlusRegardlessOfTheSetting() {
        for (setting in listOf(false, true)) {
            runBlocking { settings.setBlockScreenshots(setting) }
            ActivityScenario.launch(MainActivity::class.java).use { scenario ->
                scenario.onActivity { activity ->
                    assertEquals(
                        "setRecentsScreenshotEnabled(false) is called on API 33+ (setting=$setting)",
                        Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU,
                        activity.recentsScreenshotDisabled,
                    )
                }
            }
        }
    }

    @Test
    fun mainActivityDecorViewFiltersTouchesWhenObscured() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity { activity ->
                assertTrue(
                    "MainActivity's decor view must filter touches while obscured by " +
                        "another window, so an overlay attack can't tap through it",
                    activity.window.decorView.filterTouchesWhenObscured,
                )
            }
        }
    }
}
