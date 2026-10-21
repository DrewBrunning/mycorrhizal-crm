package com.mycorrhizal.crm

import android.app.Application
import android.content.Context
import android.content.res.Configuration
import androidx.compose.foundation.layout.Column
import androidx.compose.material3.Text
import androidx.compose.runtime.getValue
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.junit4.StateRestorationTester
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertIsNotSelected
import androidx.compose.ui.test.assertIsSelected
import androidx.compose.ui.test.assertTextEquals
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import java.util.Locale
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issue #1758: the drawer destination registry (grouping, labels vs screen titles, icons) and the
 * drawer's selected state. The registry is also what the navigation rail (#1776) renders.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class NavDestinationsTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    // --- registry ------------------------------------------------------------------------------

    @Test
    fun `groups follow the issue 1758 layout with Settings pinned last`() {
        fun routes(group: NavGroup) = navDestinationsIn(group).map { it.route }

        assertEquals(
            listOf("home", "contacts", "circles", "tags", "households", "network", "map"),
            routes(NavGroup.PEOPLE),
        )
        assertEquals(listOf("activities", "notes", "occasions", "shares"), routes(NavGroup.INTERACTIONS))
        assertEquals(listOf("data", "import", "audit"), routes(NavGroup.DATA_ADMIN))
        assertEquals(listOf("settings"), routes(NavGroup.FOOTER))
        assertEquals("settings", navDestinations.last().route)
    }

    @Test
    fun `each group is contiguous so a divider falls only between groups`() {
        val order = navDestinations.map { it.group }
        val collapsed = order.filterIndexed { i, g -> i == 0 || order[i - 1] != g }
        assertEquals("a group appears in two separate runs: $order", collapsed.distinct(), collapsed)
    }

    @Test
    fun `routes are unique and no icon glyph is shared between destinations`() {
        assertEquals(navDestinations.size, navDestinations.map { it.route }.toSet().size)
        // Issue #1758: Activities and Occasions both used a calendar glyph.
        val glyphs = navDestinations.map { it.icon.name }
        assertEquals("duplicate icon among $glyphs", glyphs.size, glyphs.toSet().size)
    }

    /**
     * The title each destination's screen shows in its top bar. A destination added to
     * [navDestinations] without an entry here fails the test below, so the list cannot grow a
     * label nobody compared with a title.
     */
    private val screenTitleRes: Map<String, Int> = mapOf(
        "home" to R.string.nav_dashboard,
        "contacts" to R.string.nav_contacts,
        "circles" to R.string.circles_title,
        "tags" to R.string.tags_title,
        "households" to R.string.households_title,
        "network" to R.string.nav_network,
        "map" to R.string.nav_map,
        "activities" to R.string.nav_activities,
        "notes" to R.string.nav_notes_unassigned,
        "occasions" to R.string.occasions_title,
        "shares" to R.string.shares_title,
        "data" to R.string.settings_data,
        "import" to R.string.import_title,
        "audit" to R.string.audit_title,
        "settings" to R.string.settings_title,
    )

    private fun localized(locale: Locale): Context {
        val base = ApplicationProvider.getApplicationContext<Application>()
        val config = Configuration(base.resources.configuration).apply { setLocale(locale) }
        return base.createConfigurationContext(config)
    }

    @Test
    fun `every destination has a screen title entry`() {
        assertEquals(navDestinations.map { it.route }.toSet(), screenTitleRes.keys)
    }

    @Test
    fun `the drawer label equals the screen title for every destination in every locale`() {
        for (locale in listOf(Locale.ENGLISH, Locale.GERMAN, Locale("es"), Locale.FRENCH, Locale.ITALIAN)) {
            val context = localized(locale)
            for (destination in navDestinations) {
                val label = context.getString(destination.labelRes)
                val title = context.getString(screenTitleRes.getValue(destination.route))
                assertEquals("${destination.route} [$locale]: drawer label vs screen title", title, label)
            }
        }
    }

    @Test
    fun `the notes destination names the unassigned subset it lists`() {
        // Issue #1757: "Notes" read as "all of my notes"; the screen lists only contact-less ones.
        val label = localized(Locale.ENGLISH).getString(navDestinations.first { it.route == "notes" }.labelRes)
        assertEquals("Unassigned notes", label)
    }

    @Test
    fun `selection matches the route itself, its children and its owned routes only`() {
        val contacts = navDestinations.first { it.route == "contacts" }
        val settings = navDestinations.first { it.route == "settings" }
        val data = navDestinations.first { it.route == "data" }

        assertTrue(isSelected("contacts", contacts))
        assertTrue(isSelected("contacts?search={search}", contacts))
        assertTrue(isSelected("contacts/{contactId}/notes", contacts))
        assertFalse(isSelected("contacts-archive", contacts))
        assertFalse(isSelected(null, contacts))
        assertTrue(isSelected("settings/appearance", settings))
        assertTrue(isSelected("webhooks", settings))
        assertTrue(isSelected("admin/users", settings))
        assertTrue(isSelected("data/custom-export", data))
        assertFalse(isSelected("data", settings))
    }

    @Test
    fun `serverHostOf extracts the host and tolerates garbage`() {
        assertEquals("crm.example.com", serverHostOf("https://crm.example.com/api"))
        assertEquals("crm.example.com", serverHostOf(" https://crm.example.com:8443 "))
        assertEquals(null, serverHostOf(null))
        assertEquals(null, serverHostOf("   "))
        assertEquals("not a url", serverHostOf("not a url"))
    }

    // --- the drawer -----------------------------------------------------------------------------

    private fun setDrawer(
        currentRoute: String?,
        username: String? = null,
        serverHost: String? = null,
        onClick: (String) -> Unit = {},
        onHeaderClick: () -> Unit = {},
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                DrawerContent(
                    currentRoute = currentRoute,
                    onDestinationClick = onClick,
                    username = username,
                    serverHost = serverHost,
                    onHeaderClick = onHeaderClick,
                )
            }
        }
    }

    @Test
    fun `the current destination is selected and no other is`() {
        setDrawer(currentRoute = "contacts/7")

        composeTestRule.onNodeWithTag("drawer-contacts").assertIsSelected()
        allDrawerRoutes.filter { it != "contacts" && it != "settings" }.forEach { route ->
            composeTestRule.onNodeWithTag("drawer-$route").performScrollTo().assertIsNotSelected()
        }
        // Settings is pinned below the scrolling list, so it has no scroll container to scroll in.
        composeTestRule.onNodeWithTag("drawer-settings").assertIsNotSelected()
    }

    @Test
    fun `a Settings sub-screen keeps Settings selected`() {
        setDrawer(currentRoute = "settings/privacy")
        composeTestRule.onNodeWithTag("drawer-settings").assertIsSelected()
    }

    @Test
    fun `the selected destination is exposed to TalkBack as a selected tab`() {
        setDrawer(currentRoute = "home")
        composeTestRule.onNodeWithTag("drawer-home")
            .assert(SemanticsMatcher.expectValue(SemanticsProperties.Selected, true))
            .assert(SemanticsMatcher.keyIsDefined(SemanticsProperties.Role))
    }

    @Test
    fun `section labels are headings and Settings sits outside the scrolling list`() {
        setDrawer(currentRoute = "home")

        listOf("people", "interactions", "data_admin").forEach { group ->
            composeTestRule.onNodeWithTag("drawer-group-$group")
                .assert(SemanticsMatcher.keyIsDefined(SemanticsProperties.Heading))
        }
        composeTestRule.onNodeWithText("People").assertExists()
        composeTestRule.onNodeWithText("Interactions").assertExists()
        composeTestRule.onNodeWithText("Data & admin").assertExists()
        // Pinned: reachable with no scrolling at all, so no performScrollTo is needed.
        composeTestRule.onNodeWithTag("drawer-settings").assertExists().assertIsNotSelected()
    }

    @Test
    fun `tapping a destination reports its route`() {
        val clicked = mutableListOf<String>()
        setDrawer(currentRoute = "home", onClick = { clicked += it })

        composeTestRule.onNodeWithTag("drawer-tags").performScrollTo().performClick()
        composeTestRule.onNodeWithTag("drawer-settings").performClick()

        assertEquals(listOf("tags", "settings"), clicked)
    }

    @Test
    fun `the header shows who is signed in and where, and opens Servers`() {
        var opened = 0
        setDrawer(currentRoute = "home", username = "alice", serverHost = "crm.example.com", onHeaderClick = { opened++ })

        composeTestRule.onNodeWithText("alice").assertExists()
        composeTestRule.onNodeWithText("crm.example.com").assertExists()
        composeTestRule.onNodeWithTag("drawer-header").performClick()
        assertEquals(1, opened)
    }

    @Test
    fun `the header falls back to the app name and is not tappable without a session`() {
        var opened = 0
        setDrawer(currentRoute = "home", onHeaderClick = { opened++ })

        composeTestRule.onNodeWithTag("drawer-header").assertTextEquals("Mycorrhizal").performClick()
        assertEquals(0, opened)
    }

    // --- selection survives rotation -------------------------------------------------------------

    @Test
    fun `the selected destination survives a configuration change`() {
        val tester = StateRestorationTester(composeTestRule)
        tester.setContent {
            MycorrhizalTheme {
                val navController = rememberNavController()
                val entry by navController.currentBackStackEntryAsState()
                Column {
                    NavHost(navController, startDestination = "home") {
                        allDrawerRoutes.forEach { route -> composable(route) { Text("screen-$route") } }
                    }
                    DrawerContent(
                        currentRoute = entry?.destination?.route,
                        onDestinationClick = { navController.navigateToRoot(it) },
                    )
                }
            }
        }
        composeTestRule.onNodeWithTag("drawer-home").assertIsSelected()

        composeTestRule.onNodeWithTag("drawer-circles").performScrollTo().performClick()
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithTag("drawer-circles").assertIsSelected()
        composeTestRule.onNodeWithTag("drawer-home").assertIsNotSelected()

        tester.emulateSavedInstanceStateRestore()

        composeTestRule.onNodeWithTag("drawer-circles").assertIsSelected()
        composeTestRule.onNodeWithTag("drawer-home").assertIsNotSelected()
        composeTestRule.onNodeWithText("screen-circles").assertExists()
    }
}
