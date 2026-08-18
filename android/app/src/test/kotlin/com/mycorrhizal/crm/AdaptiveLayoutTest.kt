package com.mycorrhizal.crm

import android.app.Application
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.DrawerValue
import androidx.compose.material3.Text
import androidx.compose.material3.rememberDrawerState
import androidx.compose.material3.windowsizeclass.ExperimentalMaterial3WindowSizeClassApi
import androidx.compose.material3.windowsizeclass.WindowSizeClass
import androidx.compose.material3.windowsizeclass.WindowWidthSizeClass
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotSelected
import androidx.compose.ui.test.assertIsSelected
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

    private fun setMainNavScaffold(
        windowSizeClass: WindowSizeClass,
        currentRoute: String? = "home",
        onDestinationClick: (String) -> Unit = {},
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                MainNavScaffold(
                    currentRoute = currentRoute,
                    windowSizeClass = windowSizeClass,
                    drawerState = rememberDrawerState(initialValue = DrawerValue.Closed),
                    onDestinationClick = onDestinationClick,
                ) {
                    Text("CONTENT", modifier = Modifier.fillMaxSize().testTag("main-content"))
                }
            }
        }
    }

    private fun setAdaptiveContent(
        isTwoPane: Boolean,
        currentRoute: String?,
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                AdaptiveMainContent(
                    isTwoPane = isTwoPane,
                    currentRoute = currentRoute,
                    content = { Text("DETAIL", modifier = Modifier.testTag("fake-detail")) },
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

    @Test
    fun `the width class at expanded size is expanded`() {
        assertTrue(expandedSize.widthSizeClass == WindowWidthSizeClass.Expanded)
        assertTrue(compactSize.widthSizeClass == WindowWidthSizeClass.Compact)
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
