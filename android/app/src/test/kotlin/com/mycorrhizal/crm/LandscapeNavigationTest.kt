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
    fun `permanent navigation starts at medium width and needs at least medium height`() {
        assertFalse("landscape phone", usesPermanentNavigation(size(914, 411)))
        assertTrue("tablet", usesPermanentNavigation(size(1280, 800)))
        assertTrue("medium height boundary", usesPermanentNavigation(size(1000, 480)))
        assertFalse("compact width", usesPermanentNavigation(size(400, 900)))
        assertTrue("medium width", usesPermanentNavigation(size(700, 900)))
        assertFalse("medium width but compact height", usesPermanentNavigation(size(700, 411)))
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
            // Issue #1758: the footer destination (Settings) is pinned below the scrolling list.
            if (navDestinationsIn(NavGroup.FOOTER).none { it.route == route }) node.performScrollTo()
            node.assertIsDisplayed().assertHeightIsAtLeast(48.dp)
        }
    }

    @Test
    @Config(qualifiers = "w1280dp-h800dp")
    fun `tablet rail shows the primary destinations and More at standard height`() {
        setScaffold(size(1280, 800))
        composeTestRule.onNodeWithTag("navigation-rail").assertExists()
        (railDestinations.map { it.route } + "more").forEach { route ->
            composeTestRule.onNodeWithTag("rail-$route")
                .assertIsDisplayed().assertHeightIsAtLeast(48.dp)
        }
    }

    @Test
    @Config(qualifiers = "w700dp-h900dp")
    fun `medium-width tablet shows the rail`() {
        setScaffold(size(700, 900))
        composeTestRule.onNodeWithTag("navigation-rail").assertExists()
        composeTestRule.onNodeWithTag("rail-home").assertIsDisplayed()
    }

    @Test
    @Config(qualifiers = "w1000dp-h480dp")
    fun `the short rail still shows every primary destination and More`() {
        setScaffold(size(1000, 480))
        (railDestinations.map { it.route } + "more").forEach { route ->
            composeTestRule.onNodeWithTag("rail-$route")
                .assertIsDisplayed().assertHeightIsAtLeast(48.dp)
        }
    }
}
