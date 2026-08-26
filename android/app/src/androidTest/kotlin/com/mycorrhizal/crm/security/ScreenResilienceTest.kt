package com.mycorrhizal.crm.security

import android.view.WindowManager
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.mycorrhizal.crm.MainActivity
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Issue #507: MASVS-L2 resilience re-evaluation reversed two of the four
 * previously-declined controls (screenshot prevention, tapjacking protection —
 * see `docs/security/masvs-l1.md` P3 and `docs/security/threat-model.md`
 * gating decision 3). Both are window-level flags set in
 * `MainActivity.onCreate`, so they can only be verified against a real Window
 * — a Robolectric JVM test's shadow Window does not model FLAG_SECURE or
 * touch-filtering — hence this is an instrumented (device/emulator) test, not
 * a unit test. Neither requires the docker-compose backend or a logged-in
 * session (the flags are set before any network call), so this does not use
 * `E2eBaseTest`.
 */
@RunWith(AndroidJUnit4::class)
class ScreenResilienceTest {

    @Test
    fun mainActivityWindowIsFlaggedSecure() {
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            scenario.onActivity { activity ->
                val flags = activity.window.attributes.flags
                assertNotEquals(
                    "MainActivity's window must set FLAG_SECURE so screenshots, screen " +
                        "recording, and the recent-apps thumbnail never expose contact PII",
                    0,
                    flags and WindowManager.LayoutParams.FLAG_SECURE,
                )
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
