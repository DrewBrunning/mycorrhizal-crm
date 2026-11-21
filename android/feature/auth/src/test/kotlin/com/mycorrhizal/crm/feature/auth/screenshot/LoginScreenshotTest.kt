package com.mycorrhizal.crm.feature.auth.screenshot

import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onRoot
import com.github.takahirom.roborazzi.RobolectricDeviceQualifiers
import com.github.takahirom.roborazzi.captureRoboImage
import com.mycorrhizal.crm.feature.auth.LoginMode
import com.mycorrhizal.crm.feature.auth.LoginScreenContent
import com.mycorrhizal.crm.feature.auth.LoginUiState
import com.mycorrhizal.crm.ui.R
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
 * Issue #2534: visual-regression goldens for the login screen.
 *
 * The stateless [LoginScreenContent] is rendered from a hand-built [LoginUiState]
 * — no ViewModel, no Hilt, no network, no clock, no generated IDs — in the
 * credentials (password) mode, light and dark at font scale 1.0, plus the
 * invalid-credentials error state. The screen's own `Scaffold` paints the themed
 * background, so no extra `Surface` wrapper is needed (unlike the contact detail).
 *
 * There is no tablet golden: this screen has no size-class branch (no
 * `WindowSizeClass`, `BoxWithConstraints` or width cap — one centred, vertically
 * scrolling column of full-width fields), so a MediumTablet run would only
 * stretch the same pixels and pin nothing new. The theme/font-scale cells cover
 * the layout differences this screen can actually produce.
 *
 * Goldens live in `src/test/screenshots/`; regenerate them with
 * `./gradlew :feature:auth:recordRoborazziDebug`.
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(sdk = [35], qualifiers = RobolectricDeviceQualifiers.Pixel7)
class LoginScreenshotTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun lightThemeAtDefaultFontScale() = capture("Login_light_1.0.png", darkTheme = false)

    @Test
    fun darkThemeAtDefaultFontScale() = capture("Login_dark_1.0.png", darkTheme = true)

    @Test
    fun errorStateInLightTheme() = capture(
        "Login_error_light_1.0.png",
        darkTheme = false,
        uiState = CREDENTIALS.copy(errorRes = R.string.login_error_invalid_credentials)
    )

    private fun capture(
        goldenName: String,
        darkTheme: Boolean,
        fontScale: Float = 1f,
        uiState: LoginUiState = CREDENTIALS
    ) {
        // Before setContent: Compose reads the font scale when it composes.
        RuntimeEnvironment.setFontScale(fontScale)
        composeTestRule.setContent {
            MycorrhizalTheme(darkTheme = darkTheme) {
                LoginScreenContent(
                    uiState = uiState,
                    onServerUrlChange = {},
                    onModeChange = {},
                    onSubmit = { _, _, _, _ -> }
                )
            }
        }
        // onRoot() waits for idle, so the capture cannot race a pending layout
        // or the error snackbar's entry animation.
        composeTestRule.onRoot().captureRoboImage(File(OUTPUT_DIR, goldenName))
    }

    private companion object {
        /** Relative to the module directory, which is the test task's working directory. */
        val OUTPUT_DIR = File("src/test/screenshots")

        /**
         * Fixed credentials-mode state: password sign-in with a typed server URL,
         * so SSO is enabled and the "enter a URL" hint is absent. The blank-URL
         * disabled-SSO hint (#1760) is asserted in `LoginScreenTest`, not pinned
         * here, to keep each golden attributable to one state.
         */
        val CREDENTIALS = LoginUiState(
            serverUrl = "https://crm.example.com",
            mode = LoginMode.PASSWORD
        )
    }
}
