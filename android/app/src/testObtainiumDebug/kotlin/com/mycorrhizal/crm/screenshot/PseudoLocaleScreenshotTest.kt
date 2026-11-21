package com.mycorrhizal.crm.screenshot

import android.app.Application
import androidx.compose.runtime.Composable
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onRoot
import com.github.takahirom.roborazzi.captureRoboImage
import java.io.File
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issue #2535: en-XA pseudo-locale visual regression, light theme at font scale
 * 1.0 only — the point is text expansion, not a second theme matrix.
 *
 * These live in `:app` (and specifically in the `testObtainiumDebug` variant
 * source set) because aapt2 generates `values-en-rXA` only at the app's link
 * step: the feature modules have no `src/main/res` for their own screens and
 * their library test resource APKs carry zero `(en-rXA)` entries, so the same
 * test there would render plain English and pass silently. Obtainium is the
 * superset flavor, so covering it covers foss/play, and the pseudo resources
 * are debug-only. See "Screenshot goldens" in `android/AGENTS.md` before moving
 * them.
 *
 * Fixtures are shared with [PseudoLocaleHardcodedStringTest] and are the same
 * hand-built state as the module-local base goldens (#2534), so
 * `ContactList_pseudo.png` can be compared with `ContactList_light_1.0.png`.
 *
 * Goldens live in `src/test/screenshots/`; record with
 * `./gradlew :app:recordRoborazziObtainiumDebug` and commit the PNG diff.
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(sdk = [35], application = Application::class, qualifiers = PSEUDO_LOCALE_QUALIFIERS)
class PseudoLocaleScreenshotTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun login() = capture("Login_pseudo.png") { PseudoLoginScreen() }

    @Test
    fun contactList() = capture("ContactList_pseudo.png") { PseudoContactListScreen() }

    @Test
    fun contactDetail() = capture("ContactDetail_pseudo.png") { PseudoContactDetailScreen() }

    @Test
    fun settings() = capture("Settings_pseudo.png") { PseudoSettingsScreen() }

    private fun capture(goldenName: String, content: @Composable () -> Unit) {
        // Before setContent: Compose reads the font scale when it composes.
        RuntimeEnvironment.setFontScale(1f)
        composeTestRule.setContent { content() }
        // onRoot() waits for idle, so the capture cannot race a pending layout.
        composeTestRule.onRoot().captureRoboImage(File(OUTPUT_DIR, goldenName))
    }

    private companion object {
        /** Relative to the module directory, which is the test task's working directory. */
        val OUTPUT_DIR = File("src/test/screenshots")
    }
}
