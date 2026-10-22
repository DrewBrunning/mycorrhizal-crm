package com.mycorrhizal.crm

import android.app.Application
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.requiredWidth
import androidx.compose.material3.DrawerValue
import androidx.compose.material3.Text
import androidx.compose.material3.rememberDrawerState
import androidx.compose.material3.windowsizeclass.ExperimentalMaterial3WindowSizeClassApi
import androidx.compose.material3.windowsizeclass.WindowSizeClass
import androidx.compose.material3.windowsizeclass.WindowWidthSizeClass
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotDisplayed
import androidx.compose.ui.test.assertIsNotSelected
import androidx.compose.ui.test.assertIsSelected
import androidx.compose.ui.test.assertWidthIsEqualTo
import androidx.compose.ui.test.getUnclippedBoundsInRoot
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.unit.DpSize
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

// Issue #150: the tablet layout (NavigationRail instead of the drawer, and the
// two-pane contacts list/detail) is a host-level decision, so it is tested at
// the host level — the stateless frames MainNavScaffold/AdaptiveMainContent
// receive a fake WindowSizeClass and slot content, no per-screen duplication.
@OptIn(ExperimentalMaterial3WindowSizeClassApi::class)
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class AdaptiveLayoutTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    /** 1000dp wide — past the 840dp Expanded threshold. */
    private val expandedSize = WindowSizeClass.calculateFromSize(DpSize(1000.dp, 700.dp))

    /** 400dp wide — a phone. */
    private val compactSize = WindowSizeClass.calculateFromSize(DpSize(400.dp, 700.dp))

    /** 700dp wide — the Medium width class (600-839dp). */
    private val mediumSize = WindowSizeClass.calculateFromSize(DpSize(700.dp, 900.dp))

    /** 914x411dp — a phone in landscape: Expanded wide but Compact tall. */
    private val landscapePhoneSize = WindowSizeClass.calculateFromSize(DpSize(914.dp, 411.dp))

    private fun setMainNavScaffold(
        windowSizeClass: WindowSizeClass,
        currentRoute: String? = "home",
        onDestinationClick: (String) -> Unit = {},
        railInsets: WindowInsets = WindowInsets(0, 0, 0, 0),
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                MainNavScaffold(
                    currentRoute = currentRoute,
                    windowSizeClass = windowSizeClass,
                    drawerState = rememberDrawerState(initialValue = DrawerValue.Closed),
                    onDestinationClick = onDestinationClick,
                    railInsets = railInsets,
                ) {
                    Text("CONTENT", modifier = Modifier.fillMaxSize().testTag("main-content"))
                }
            }
        }
    }

    private fun setAdaptiveContent(
        isTwoPane: Boolean,
        currentRoute: String?,
        constrainWidth: Boolean = false,
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                AdaptiveMainContent(
                    isTwoPane = isTwoPane,
                    currentRoute = currentRoute,
                    constrainWidth = constrainWidth,
                    content = { Text("DETAIL", modifier = Modifier.fillMaxSize().testTag("fake-detail")) },
                    listPane = { Text("LIST", modifier = Modifier.testTag("fake-list")) },
                )
            }
        }
    }

    // --- MainNavScaffold: rail vs drawer on the width class ------------------

    @Test
    fun `expanded width shows the navigation rail instead of the drawer`() {
        setMainNavScaffold(expandedSize)

        composeTestRule.onNodeWithTag("navigation-rail").assertExists()
        composeTestRule.onNodeWithTag("rail-home").assertIsDisplayed()
        composeTestRule.onNodeWithTag("rail-contacts").assertExists()
        composeTestRule.onNodeWithTag("main-content").assertIsDisplayed()
    }

    @Test
    fun `medium width shows the navigation rail instead of the drawer`() {
        // Issue #1776: M3 recommends the rail from Medium (600-839dp), not only Expanded.
        setMainNavScaffold(mediumSize)

        composeTestRule.onNodeWithTag("navigation-rail").assertExists()
        composeTestRule.onNodeWithTag("rail-home").assertIsDisplayed()
        composeTestRule.onNodeWithTag("main-content").assertIsDisplayed()
    }

    @Test
    fun `landscape phone keeps the drawer despite its expanded width`() {
        // Issue #1419: Expanded wide but Compact tall has no vertical room for the rail.
        setMainNavScaffold(landscapePhoneSize)

        composeTestRule.onNodeWithTag("navigation-rail").assertDoesNotExist()
        composeTestRule.onNodeWithTag("main-content").assertIsDisplayed()
    }

    @Test
    fun `the rail shows only the primary destinations plus More`() {
        setMainNavScaffold(expandedSize)

        assertTrue("the rail primary set is 5-7 destinations", railDestinations.size in 5..7)
        railDestinations.forEach { destination ->
            composeTestRule.onNodeWithTag("rail-${destination.route}").assertExists()
        }
        composeTestRule.onNodeWithTag("rail-more").assertExists()
        // Secondary destinations live behind More, so their labels cannot truncate the rail.
        listOf("activities", "notes", "occasions", "data", "import", "audit", "settings").forEach { route ->
            composeTestRule.onNodeWithTag("rail-$route").assertDoesNotExist()
        }
    }

    @Test
    fun `rail content starts below the status bar inset`() {
        // Issue #1776: the rail used to draw its scrolling content under the status bar.
        setMainNavScaffold(expandedSize, railInsets = WindowInsets(0, 48, 0, 0))

        val top = composeTestRule.onNodeWithTag("rail-home").getUnclippedBoundsInRoot().top
        assertTrue("rail item top was $top, expected below the 48dp status-bar inset", top >= 48.dp)
    }

    @Test
    fun `More opens the full grouped drawer`() {
        setMainNavScaffold(expandedSize)
        composeTestRule.onNodeWithTag("drawer-settings").assertIsNotDisplayed()

        composeTestRule.onNodeWithTag("rail-more").performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithTag("drawer-home").assertIsDisplayed()
        composeTestRule.onNodeWithTag("drawer-settings").assertIsDisplayed()
    }

    @Test
    fun `More is selected when the current route lives behind it`() {
        setMainNavScaffold(expandedSize, currentRoute = "settings")

        composeTestRule.onNodeWithTag("rail-more").assertIsSelected()
        railDestinations.forEach { destination ->
            composeTestRule.onNodeWithTag("rail-${destination.route}").assertIsNotSelected()
        }
    }

    @Test
    fun `a primary rail destination is selected instead of More`() {
        setMainNavScaffold(expandedSize, currentRoute = "contacts/5")

        composeTestRule.onNodeWithTag("rail-contacts").assertIsSelected()
        composeTestRule.onNodeWithTag("rail-more").assertIsNotSelected()
    }

    @Test
    fun `compact width has no rail and still hosts the content`() {
        setMainNavScaffold(compactSize)

        composeTestRule.onNodeWithTag("navigation-rail").assertDoesNotExist()
        composeTestRule.onNodeWithTag("rail-home").assertDoesNotExist()
        composeTestRule.onNodeWithTag("main-content").assertIsDisplayed()
    }

    @Test
    fun `tapping a rail item reports its route to the navigation callback`() {
        var clicked: String? = null
        setMainNavScaffold(expandedSize, onDestinationClick = { clicked = it })

        composeTestRule.onNodeWithTag("rail-contacts").performClick()

        assertEquals("contacts", clicked)
    }

    @Test
    fun `the rail highlights the destination for the current route`() {
        setMainNavScaffold(expandedSize, currentRoute = "contacts/5")

        composeTestRule.onNodeWithTag("rail-contacts").assertIsSelected()
        composeTestRule.onNodeWithTag("rail-home").assertIsNotSelected()
    }

    @Test
    fun `the rail highlights a bare route match too`() {
        setMainNavScaffold(expandedSize, currentRoute = "home")

        composeTestRule.onNodeWithTag("rail-home").assertIsSelected()
        composeTestRule.onNodeWithTag("rail-contacts").assertIsNotSelected()
    }

    // --- AdaptiveMainContent: two-pane vs single-pane on route + width -------

    @Test
    fun `expanded width on a contacts route shows the list and detail panes`() {
        setAdaptiveContent(isTwoPane = true, currentRoute = "contacts/5")

        composeTestRule.onNodeWithTag("two-pane-list").assertIsDisplayed()
        composeTestRule.onNodeWithTag("two-pane-detail").assertIsDisplayed()
        composeTestRule.onNodeWithText("LIST").assertIsDisplayed()
        composeTestRule.onNodeWithText("DETAIL").assertIsDisplayed()
    }

    @Test
    fun `the bare contacts route is two-pane too`() {
        setAdaptiveContent(isTwoPane = true, currentRoute = "contacts")

        composeTestRule.onNodeWithTag("two-pane-list").assertIsDisplayed()
        composeTestRule.onNodeWithTag("two-pane-detail").assertIsDisplayed()
    }

    @Test
    fun `a merge route keeps the two-pane layout`() {
        setAdaptiveContent(isTwoPane = true, currentRoute = "merge/5")

        composeTestRule.onNodeWithTag("two-pane-list").assertIsDisplayed()
        composeTestRule.onNodeWithTag("two-pane-detail").assertIsDisplayed()
    }

    @Test
    fun `a non-contacts route stays single-pane even at expanded width`() {
        setAdaptiveContent(isTwoPane = true, currentRoute = "home")

        composeTestRule.onNodeWithTag("two-pane-list").assertDoesNotExist()
        composeTestRule.onNodeWithTag("two-pane-detail").assertDoesNotExist()
        composeTestRule.onNodeWithText("DETAIL").assertIsDisplayed()
        composeTestRule.onNodeWithText("LIST").assertDoesNotExist()
    }

    @Test
    fun `phone widths stay single-pane regardless of route`() {
        setAdaptiveContent(isTwoPane = false, currentRoute = "contacts/5")

        composeTestRule.onNodeWithTag("two-pane-list").assertDoesNotExist()
        composeTestRule.onNodeWithTag("two-pane-detail").assertDoesNotExist()
        composeTestRule.onNodeWithText("DETAIL").assertIsDisplayed()
        composeTestRule.onNodeWithText("LIST").assertDoesNotExist()
    }

    // --- Issue #1776: single-pane content max width ---------------------------

    @Test
    fun `a wide single pane is capped at the content max width and centred`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                Box(Modifier.requiredWidth(1000.dp)) {
                    AdaptiveMainContent(
                        isTwoPane = false,
                        currentRoute = "home",
                        constrainWidth = true,
                        content = { Text("DETAIL", modifier = Modifier.fillMaxSize().testTag("capped-detail")) },
                    )
                }
            }
        }

        composeTestRule.onNodeWithTag("content-width-frame").assertIsDisplayed()
        composeTestRule.onNodeWithTag("capped-detail").assertWidthIsEqualTo(ContentMaxWidth)
    }

    @Test
    fun `a compact single pane is not width-capped`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                Box(Modifier.requiredWidth(400.dp)) {
                    AdaptiveMainContent(
                        isTwoPane = false,
                        currentRoute = "home",
                        constrainWidth = false,
                        content = { Text("DETAIL", modifier = Modifier.fillMaxSize().testTag("full-detail")) },
                    )
                }
            }
        }

        composeTestRule.onNodeWithTag("content-width-frame").assertDoesNotExist()
        composeTestRule.onNodeWithTag("full-detail").assertWidthIsEqualTo(400.dp)
    }

    @Test
    fun `only the single-pane frame is capped, not the two-pane split`() {
        setAdaptiveContent(isTwoPane = true, currentRoute = "contacts/5", constrainWidth = true)

        composeTestRule.onNodeWithTag("content-width-frame").assertDoesNotExist()
        composeTestRule.onNodeWithTag("two-pane-list").assertIsDisplayed()
        composeTestRule.onNodeWithTag("two-pane-detail").assertIsDisplayed()
    }

    @Test
    fun `the width class at expanded size is expanded`() {
        assertTrue(expandedSize.widthSizeClass == WindowWidthSizeClass.Expanded)
        assertTrue(compactSize.widthSizeClass == WindowWidthSizeClass.Compact)
        assertTrue(mediumSize.widthSizeClass == WindowWidthSizeClass.Medium)
    }

    @Test
    fun `medium and expanded widths constrain a single pane, compact does not`() {
        assertFalse(constrainsSinglePane(WindowWidthSizeClass.Compact))
        assertTrue(constrainsSinglePane(WindowWidthSizeClass.Medium))
        assertTrue(constrainsSinglePane(WindowWidthSizeClass.Expanded))
    }
}

/** Issue #150: the two-pane route-boundary matcher, tested as a pure function. */
class ContactsSectionRouteTest {

    @Test
    fun `contact routes are inside the two-pane section`() {
        assertTrue(isInContactsSection("contacts"))
        assertTrue(isInContactsSection("contacts/5"))
        assertTrue(isInContactsSection("contacts/5/edit"))
        assertTrue(isInContactsSection("contacts/new"))
        assertTrue(isInContactsSection("contacts/5/prep"))
        assertTrue(isInContactsSection("merge/5"))
    }

    @Test
    fun `other routes are outside the two-pane section`() {
        assertFalse(isInContactsSection("home"))
        assertFalse(isInContactsSection("notes"))
        assertFalse(isInContactsSection("settings"))
        assertFalse(isInContactsSection("contactsx"))
        assertFalse(isInContactsSection(null))
    }
}
