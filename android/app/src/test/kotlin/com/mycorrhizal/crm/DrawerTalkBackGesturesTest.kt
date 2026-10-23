package com.mycorrhizal.crm

import android.app.Application
import android.content.Context
import android.view.accessibility.AccessibilityManager
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.DrawerState
import androidx.compose.material3.DrawerValue
import androidx.compose.material3.rememberDrawerState
import androidx.compose.material3.windowsizeclass.ExperimentalMaterial3WindowSizeClassApi
import androidx.compose.material3.windowsizeclass.WindowSizeClass
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performTouchInput
import androidx.compose.ui.test.swipeRight
import androidx.compose.ui.unit.DpSize
import androidx.compose.ui.unit.dp
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.Shadows
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issue #1923: TalkBack's next/previous-item gesture is a one-finger horizontal
 * swipe, the same motion as the modal drawer's edge-swipe. With touch
 * exploration on, the drawer gesture must be disabled so it does not swallow
 * traversal starting near the left edge; the Menu/More controls still open it.
 */
@OptIn(ExperimentalMaterial3WindowSizeClassApi::class)
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class DrawerTalkBackGesturesTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val accessibilityManager
        get() = ApplicationProvider.getApplicationContext<Context>()
            .getSystemService(AccessibilityManager::class.java)

    @Before
    fun resetAccessibility() {
        Shadows.shadowOf(accessibilityManager).setEnabled(false)
        Shadows.shadowOf(accessibilityManager).setTouchExplorationEnabled(false)
    }

    @After
    fun restoreAccessibility() {
        Shadows.shadowOf(accessibilityManager).setTouchExplorationEnabled(false)
        Shadows.shadowOf(accessibilityManager).setEnabled(false)
    }

    private fun setContent(): DrawerState {
        lateinit var drawerState: DrawerState
        composeTestRule.setContent {
            MycorrhizalTheme {
                drawerState = rememberDrawerState(DrawerValue.Closed)
                MainNavScaffold(
                    currentRoute = "home",
                    windowSizeClass = WindowSizeClass.calculateFromSize(DpSize(400.dp, 800.dp)),
                    drawerState = drawerState,
                    onDestinationClick = {},
                ) {
                    Box(Modifier.fillMaxSize().testTag("drawer-gesture-surface"))
                }
            }
        }
        composeTestRule.waitForIdle()
        return drawerState
    }

    private fun swipeInFromLeftEdge() {
        composeTestRule.onNodeWithTag("drawer-gesture-surface").performTouchInput {
            swipeRight(startX = width * 0.02f, endX = width * 0.8f)
        }
        composeTestRule.waitForIdle()
    }

    @Test
    fun `the edge swipe still opens the drawer without touch exploration`() {
        val drawerState = setContent()

        swipeInFromLeftEdge()

        assertEquals(DrawerValue.Open, drawerState.currentValue)
    }

    @Test
    fun `touch exploration stops the edge swipe from opening the drawer`() {
        Shadows.shadowOf(accessibilityManager).setEnabled(true)
        Shadows.shadowOf(accessibilityManager).setTouchExplorationEnabled(true)
        val drawerState = setContent()

        swipeInFromLeftEdge()

        assertEquals(
            "TalkBack's swipe gestures must not be intercepted by the drawer",
            DrawerValue.Closed,
            drawerState.currentValue,
        )
    }
}
