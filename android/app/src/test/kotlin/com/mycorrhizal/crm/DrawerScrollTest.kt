package com.mycorrhizal.crm

import android.app.Application
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.test.assertHeightIsAtLeast
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.getUnclippedBoundsInRoot
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

// Issue #1402: the drawer's 15 entries sat in a non-scrolling Column, so the last
// (Settings) was squeezed to ~43dp on a Pixel 8a and trailing entries collapsed to
// zero height on shorter screens / larger font scales. Every entry must stay
// reachable by scrolling, at the standard (>= 48dp) height, all the same height.
@OptIn(androidx.compose.material3.windowsizeclass.ExperimentalMaterial3WindowSizeClassApi::class)
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class DrawerScrollTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    // Every route the real drawer/rail registers, not a hand-maintained copy:
    // issue #1430, "map" was added to the drawer while this list wasn't, so the
    // reachability guarantee silently skipped the new entry. Deriving it here
    // means a new DrawerDestination is covered automatically.
    // Capabilities are Unknown (fail-open) here, so every destination is offered.
    private val allRoutes = allDrawerRoutes

    private fun setDrawer(fontScale: Float? = null, onClick: (String) -> Unit = {}) {
        composeTestRule.setContent {
            val base = LocalDensity.current
            val density = if (fontScale == null) base else Density(base.density, fontScale)
            CompositionLocalProvider(LocalDensity provides density) {
                MycorrhizalTheme {
                    DrawerContent(currentRoute = "home", onDestinationClick = onClick)
                }
            }
        }
        composeTestRule.waitForIdle()
    }

    private fun assertEveryEntryReachableAtStandardHeight() {
        val heights = allRoutes.map { route ->
            val node = composeTestRule.onNodeWithTag("drawer-$route")
            node.performScrollTo()
            node.assertIsDisplayed()
            node.assertHeightIsAtLeast(48.dp)
            node.getUnclippedBoundsInRoot().let { it.bottom - it.top }
        }
        assertEquals("every drawer entry has the same height: $heights", 1, heights.toSet().size)
    }

    @Test
    @Config(qualifiers = "w360dp-h480dp")
    fun `every destination is reachable at standard height on a short window`() {
        setDrawer()
        assertEveryEntryReachableAtStandardHeight()
    }

    @Test
    @Config(qualifiers = "w360dp-h800dp")
    fun `every destination is reachable at standard height at font scale 2`() {
        setDrawer(fontScale = 2f)
        assertEveryEntryReachableAtStandardHeight()
    }

    @Test
    @Config(qualifiers = "w360dp-h480dp")
    fun `a short window at font scale 2 keeps every entry reachable`() {
        setDrawer(fontScale = 2f)
        assertEveryEntryReachableAtStandardHeight()
    }

    @Test
    @Config(qualifiers = "w360dp-h480dp")
    fun `every entry is reachable by scrolling and reports its route on tap`() {
        val clicked = mutableListOf<String>()
        setDrawer(onClick = { clicked += it })

        // Issue #1430: the E2E helper clicked a drawer label without scrolling
        // first, which silently dismissed the drawer for the trailing entries
        // (Settings). Scrolling before the tap is what makes a destination
        // reachable, so this pins it for *every* registered route.
        allRoutes.forEach { route ->
            val node = composeTestRule.onNodeWithTag("drawer-$route")
            node.performScrollTo()
            node.assertIsDisplayed()
            node.performClick()
        }

        assertEquals("every drawer route is tappable in order", allRoutes, clicked)
    }

    // The expanded-width rail pins the primary set and scrolls the secondary set;
    // pin that it stays reachable (and un-squeezed) at a short height.
    @Test
    @Config(qualifiers = "w1000dp-h480dp")
    fun `the navigation rail keeps the last destination reachable on a short window`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                androidx.compose.foundation.layout.Box {
                    MainNavScaffold(
                        currentRoute = "home",
                        windowSizeClass = androidx.compose.material3.windowsizeclass.WindowSizeClass
                            .calculateFromSize(androidx.compose.ui.unit.DpSize(1000.dp, 480.dp)),
                        drawerState = androidx.compose.material3.rememberDrawerState(
                            androidx.compose.material3.DrawerValue.Closed,
                        ),
                        onDestinationClick = {},
                    ) {}
                }
            }
        }
        composeTestRule.onNodeWithTag("rail-settings").performScrollTo()
        composeTestRule.onNodeWithTag("rail-settings").assertIsDisplayed().assertHeightIsAtLeast(48.dp)
    }
}
