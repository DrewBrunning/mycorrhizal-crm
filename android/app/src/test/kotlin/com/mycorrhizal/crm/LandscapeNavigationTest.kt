package com.mycorrhizal.crm

import android.app.Application
import androidx.compose.material3.DrawerValue
import androidx.compose.material3.rememberDrawerState
import androidx.compose.material3.windowsizeclass.ExperimentalMaterial3WindowSizeClassApi
import androidx.compose.material3.windowsizeclass.WindowSizeClass
import androidx.compose.ui.test.assertHeightIsAtLeast
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.unit.DpSize
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

// Issue #1419: a phone in landscape (~914x411dp) is Expanded wide but Compact tall.
// It must keep the modal drawer (one scrolling list), not the 80dp rail; a genuine
// tablet keeps the rail. Both must keep every destination reachable at >= 48dp.
@OptIn(ExperimentalMaterial3WindowSizeClassApi::class)
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class LandscapeNavigationTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    // Derived from the real registry, not a hand-maintained copy (issue #1430:
    // "map" was added to the drawer while this list wasn't, so the landscape/
    // rail reachability checks silently skipped it).
    private val allRoutes = allDrawerRoutes

    private fun size(w: Int, h: Int) = WindowSizeClass.calculateFromSize(DpSize(w.dp, h.dp))

    private fun setScaffold(windowSizeClass: WindowSizeClass, drawerValue: DrawerValue = DrawerValue.Closed) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                MainNavScaffold(
                    currentRoute = "home",
                    windowSizeClass = windowSizeClass,
                    drawerState = rememberDrawerState(drawerValue),
                    onDestinationClick = {},
                ) {}
            }
        }
        composeTestRule.waitForIdle()
    }

    @Test
    fun `permanent navigation needs expanded width and at least medium height`() {
        assertFalse("landscape phone", usesPermanentNavigation(size(914, 411)))
        assertTrue("tablet", usesPermanentNavigation(size(1280, 800)))
        assertTrue("medium height boundary", usesPermanentNavigation(size(1000, 480)))
        assertFalse("compact width", usesPermanentNavigation(size(400, 900)))
        assertFalse("medium width", usesPermanentNavigation(size(700, 900)))
    }

    @Test
    @Config(qualifiers = "w914dp-h411dp")
    fun `landscape phone keeps the drawer instead of the rail`() {
        setScaffold(size(914, 411))
        composeTestRule.onNodeWithTag("navigation-rail").assertDoesNotExist()
    }

    @Test
    @Config(qualifiers = "w914dp-h411dp")
    fun `landscape phone drawer reaches every destination at standard height`() {
        setScaffold(size(914, 411), DrawerValue.Open)
        composeTestRule.onNodeWithTag("navigation-rail").assertDoesNotExist()
        allRoutes.forEach { route ->
            val node = composeTestRule.onNodeWithTag("drawer-$route")
            node.performScrollTo()
            node.assertIsDisplayed().assertHeightIsAtLeast(48.dp)
        }
    }

    @Test
    @Config(qualifiers = "w1280dp-h800dp")
    fun `tablet keeps the rail with every destination reachable`() {
        setScaffold(size(1280, 800))
        composeTestRule.onNodeWithTag("navigation-rail").assertExists()
        allRoutes.forEach { route ->
            val node = composeTestRule.onNodeWithTag("rail-$route")
            node.performScrollTo()
            node.assertIsDisplayed().assertHeightIsAtLeast(48.dp)
        }
    }

    // The whole rail scrolls as one: the top (primary) entry must stay reachable
    // after the list has been scrolled to the bottom on a short tablet window.
    @Test
    @Config(qualifiers = "w1000dp-h480dp")
    fun `the short rail scrolls primary and secondary entries together`() {
        setScaffold(size(1000, 480))
        composeTestRule.onNodeWithTag("rail-settings").performScrollTo()
        composeTestRule.onNodeWithTag("rail-home").performScrollTo()
        composeTestRule.onNodeWithTag("rail-home").assertIsDisplayed().assertHeightIsAtLeast(48.dp)
        composeTestRule.onNodeWithTag("rail-settings").performScrollTo().assertIsDisplayed()
    }
}
