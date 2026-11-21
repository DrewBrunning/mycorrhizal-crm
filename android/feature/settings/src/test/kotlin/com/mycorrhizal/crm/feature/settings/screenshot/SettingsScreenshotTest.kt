package com.mycorrhizal.crm.feature.settings.screenshot

import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onRoot
import com.github.takahirom.roborazzi.RobolectricDeviceQualifiers
import com.github.takahirom.roborazzi.captureRoboImage
import com.mycorrhizal.crm.domain.about.AppBuildInfo
import com.mycorrhizal.crm.feature.settings.SettingsRootContent
import com.mycorrhizal.crm.feature.settings.SettingsUiState
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import java.io.File
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issue #2534: visual-regression goldens for the Settings root.
 *
 * The stateless [SettingsRootContent] is rendered from a hand-built
 * [SettingsUiState] — no ViewModel, no Hilt, no network, no clock, no generated
 * IDs — so a diff can only come from a layout, theme or font-scale change. The
 * screen's `rememberSettingsGating()` composition locals keep their fail-open
 * defaults here (unknown capabilities/version show every entry), and a non-admin
 * session hides only the Administration row, so the golden is the full index.
 *
 * Matrix: light/dark at font scale 1.0 and light at 2.0 on a Pixel 7, plus one
 * medium-tablet run. Dark at 2.0 is deliberately omitted: this screen is a
 * single-column list of rows whose geometry is theme-independent, so that cell
 * would duplicate the light-2.0 layout with different colours already covered by
 * dark-1.0 — it would double the golden count without pinning a new branch.
 *
 * [SettingsRootContent]'s `SettingsColumn` paints no background (in the app the
 * surrounding `Scaffold` does), so the capture wraps it in a themed `Surface`
 * with `colorScheme.background` — otherwise the dark-mode background would not
 * be part of the golden.
 *
 * Goldens live in `src/test/screenshots/`; regenerate them with
 * `./gradlew :feature:settings:recordRoborazziDebug`.
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(sdk = [35], qualifiers = RobolectricDeviceQualifiers.Pixel7)
class SettingsScreenshotTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun lightThemeAtDefaultFontScale() = capture("Settings_light_1.0.png", darkTheme = false)

    @Test
    fun darkThemeAtDefaultFontScale() = capture("Settings_dark_1.0.png", darkTheme = true)

    @Test
    fun lightThemeAtLargeFontScale() =
        capture("Settings_light_2.0.png", darkTheme = false, fontScale = 2f)

    @Test
    @Config(qualifiers = RobolectricDeviceQualifiers.MediumTablet)
    fun tabletLightThemeAtDefaultFontScale() =
        capture("Settings_light_1.0_tablet.png", darkTheme = false)

    private fun capture(goldenName: String, darkTheme: Boolean, fontScale: Float = 1f) {
        // Before setContent: Compose reads the font scale when it composes.
        RuntimeEnvironment.setFontScale(fontScale)
        composeTestRule.setContent {
            MycorrhizalTheme(darkTheme = darkTheme) {
                Surface(
                    modifier = Modifier.fillMaxSize(),
                    color = MaterialTheme.colorScheme.background
                ) {
                    SettingsRootContent(state = STATE, onLogout = {})
                }
            }
        }
        // onRoot() waits for idle, so the capture cannot race a pending layout.
        composeTestRule.onRoot().captureRoboImage(File(OUTPUT_DIR, goldenName))
    }

    private companion object {
        /** Relative to the module directory, which is the test task's working directory. */
        val OUTPUT_DIR = File("src/test/screenshots")

        /** Fixed identity: changing any value here is a deliberate golden update. */
        val STATE = SettingsUiState(
            buildInfo = AppBuildInfo(
                versionName = "1.3.0",
                versionCode = 1042,
                buildType = "release",
                flavor = "obtainium",
                applicationId = "com.mycorrhizal.crm"
            )
        )
    }
}
