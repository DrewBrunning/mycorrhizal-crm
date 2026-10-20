package com.mycorrhizal.crm

import android.app.Activity
import android.app.Application
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.core.view.WindowCompat
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

// Issue #1771: a live Settings theme switch does not recreate the Activity, so the
// system-bar icon appearance must be re-applied from the resolved theme alone.
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class SystemBarsEffectTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private var darkTheme by mutableStateOf(false)
    private var onPrimaryBar by mutableStateOf(true)
    private lateinit var activity: Activity

    private fun setContent() {
        composeTestRule.setContent {
            activity = LocalContext.current as Activity
            SystemBarsEffect(
                window = activity.window,
                darkTheme = darkTheme,
                onPrimaryBar = onPrimaryBar,
                statusBarColor = 0,
                navigationBarColor = 0,
            )
        }
        composeTestRule.waitForIdle()
    }

    private val controller
        get() = WindowCompat.getInsetsController(activity.window, activity.window.decorView)

    @Test
    fun `status and navigation icons flip with a live light to dark switch`() {
        setContent()
        // Light theme over the dark-toned primary bar: light icons; bone nav bar: dark icons.
        assertEquals(false, controller.isAppearanceLightStatusBars)
        assertEquals(true, controller.isAppearanceLightNavigationBars)

        darkTheme = true
        composeTestRule.waitForIdle()
        // Dark theme: light-toned primary bar -> dark icons; boneDark nav bar -> light icons.
        assertEquals(true, controller.isAppearanceLightStatusBars)
        assertEquals(false, controller.isAppearanceLightNavigationBars)

        darkTheme = false
        composeTestRule.waitForIdle()
        assertEquals(false, controller.isAppearanceLightStatusBars)
        assertEquals(true, controller.isAppearanceLightNavigationBars)
    }

    @Test
    fun `non-primary bar uses the intuitive direction in both themes`() {
        onPrimaryBar = false
        setContent()
        assertEquals(true, controller.isAppearanceLightStatusBars)

        darkTheme = true
        composeTestRule.waitForIdle()
        assertEquals(false, controller.isAppearanceLightStatusBars)
    }

    @Test
    fun `opening the drawer over a primary bar re-derives the status icons`() {
        darkTheme = true
        setContent()
        assertEquals(true, controller.isAppearanceLightStatusBars)

        onPrimaryBar = false
        composeTestRule.waitForIdle()
        assertEquals(false, controller.isAppearanceLightStatusBars)
    }
}
