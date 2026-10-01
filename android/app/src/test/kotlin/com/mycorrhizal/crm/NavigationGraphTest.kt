package com.mycorrhizal.crm

import android.app.Application
import androidx.compose.material3.Text
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.testTag
import androidx.compose.foundation.clickable
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.assertTextEquals
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.navigation.NavType
import androidx.navigation.compose.ComposeNavigator
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.navArgument
import androidx.navigation.testing.TestNavHostController
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

// Issue #679: the back-stack contract of navigateToRoot — the shared navigation
// used by the drawer/rail destinations AND notification deep links — is pinned
// against a real TestNavHostController. Every deep link collapses the stack to
// the start destination (with save/restore + single-top), so system-back from a
// pushed route always returns to the dashboard, never to a stale intermediate
// screen or a blank stack. The destinations are placeholders; what's under test
// is the navigation options, which the app's AppNavGraph mounts identically.
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class NavigationGraphTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun host(): Pair<TestNavHostController, () -> String?> {
        val navController = TestNavHostController(ApplicationProvider.getApplicationContext())
        composeTestRule.setContent {
            MycorrhizalTheme {
                navController.navigatorProvider.addNavigator(ComposeNavigator())
                NavHost(navController = navController, startDestination = "home") {
                    composable("home") { Text("Home") }
                    composable("contacts") { Text("Contacts") }
                    composable(
                        "contacts/{contactId}",
                        arguments = listOf(navArgument("contactId") { type = NavType.IntType }),
                    ) { Text("Contact ${it.arguments?.getInt("contactId")}") }
                    composable("circles/{circleId}") { Text("Circle") }
                    composable("tags") { Text("Tags") }
                }
            }
        }
        val currentRoute = { navController.currentBackStackEntry?.destination?.route }
        return navController to currentRoute
    }

    @Test
    fun `a deep link pushes onto the stack and back returns to the start destination`() {
        val (navController, currentRoute) = host()

        navController.navigateToRoot("contacts/7")

        assertEquals("contacts/{contactId}", currentRoute())
        assertEquals("home", navController.previousBackStackEntry?.destination?.route)

        navController.popBackStack()

        assertEquals("home", currentRoute())
        assertNull(navController.previousBackStackEntry)
    }

    @Test
    fun `a destination tap collapses a pushed detail back to the start destination`() {
        val (navController, currentRoute) = host()

        navController.navigateToRoot("contacts/7")
        navController.navigateToRoot("tags")

        // The tags tap must not stack on top of the contact detail — it pops
        // back to the start destination, so back returns to home.
        assertEquals("tags", currentRoute())
        assertEquals("home", navController.previousBackStackEntry?.destination?.route)

        navController.popBackStack()
        assertEquals("home", currentRoute())
    }

    @Test
    fun `re-deep-linking to a route already on the stack does not duplicate it`() {
        val (navController, currentRoute) = host()

        navController.navigateToRoot("contacts/7")
        navController.navigateToRoot("contacts/7")

        assertEquals("contacts/{contactId}", currentRoute())
        // single-top + collapse-to-start: exactly one contact entry above home.
        navController.popBackStack()
        assertEquals("home", currentRoute())
        assertNull(navController.previousBackStackEntry)
    }

    @Test
    fun `navigating to the current start destination is a single-top no-op`() {
        val (navController, currentRoute) = host()

        navController.navigateToRoot("home")

        assertEquals("home", currentRoute())
        assertNull("navigating home-to-home must not stack a duplicate", navController.previousBackStackEntry)
    }

    @Test
    fun `deep-link args survive the collapse and land in the destination`() {
        val (navController, currentRoute) = host()

        navController.navigateToRoot("contacts/42")

        assertEquals("contacts/{contactId}", currentRoute())
        // The argument rides along: the placeholder reads it back.
        assertEquals(42, navController.currentBackStackEntry?.arguments?.getInt("contactId"))
    }

    @Test
    fun `an unknown deep-link route is not navigated`() {
        val (navController, currentRoute) = host()

        // deepLinkRoute returns null for foreign links; the consumer only
        // navigates non-null results. Pinned here with the real mapper so the
        // NavHost is never asked for a route it does not define.
        val route = deepLinkRoute(android.net.Uri.parse("mycorrhizal://unknown/7"))
        assertNull(route)
        if (route != null) navController.navigateToRoot(route)

        assertEquals("home", currentRoute())
    }

    // Issue #1399: mirrors the real `contacts?search={search}` destination — the
    // "field" is entry-scoped state seeded once from the argument (exactly what
    // ContactListViewModel does at construction), cleared by tapping "clear".
    private fun searchHost(): TestNavHostController {
        val navController = TestNavHostController(ApplicationProvider.getApplicationContext())
        composeTestRule.setContent {
            MycorrhizalTheme {
                navController.navigatorProvider.addNavigator(ComposeNavigator())
                NavHost(navController = navController, startDestination = "home") {
                    composable("home") { Text("Home") }
                    composable(
                        "contacts?search={search}",
                        arguments = listOf(
                            navArgument("search") {
                                type = NavType.StringType
                                nullable = true
                                defaultValue = null
                            },
                        ),
                    ) { entry ->
                        var field by remember { mutableStateOf(entry.arguments?.getString("search").orEmpty()) }
                        Text(field, Modifier.testTag("field"))
                        Text("clear", Modifier.testTag("clear").clickable { field = "" })
                    }
                }
            }
        }
        return navController
    }

    @Test
    fun `a search link delivered while already on contacts applies the new query`() {
        val navController = searchHost()

        navController.navigateToRoot("contacts")
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithTag("field").assertTextEquals("")

        navController.navigateToRoot("contacts?search=Ann")
        composeTestRule.waitForIdle()

        assertEquals("Ann", navController.currentBackStackEntry?.arguments?.getString("search"))
        composeTestRule.onNodeWithTag("field").assertTextEquals("Ann")
        // #679 contract holds: back still returns to the dashboard.
        navController.popBackStack()
        assertEquals("home", navController.currentBackStackEntry?.destination?.route)
    }

    @Test
    fun `a search link applies after contacts was visited earlier and left`() {
        val navController = searchHost()

        navController.navigateToRoot("contacts")
        navController.navigateToRoot("home")
        navController.navigateToRoot("contacts?search=Bob")
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithTag("field").assertTextEquals("Bob")
    }

    @Test
    fun `a delivered search is consumed once and not re-applied on recomposition`() {
        val navController = searchHost()

        navController.navigateToRoot("contacts")
        navController.navigateToRoot("contacts?search=Ann")
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithTag("field").assertTextEquals("Ann")

        composeTestRule.onNodeWithTag("clear").performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithTag("field").assertTextEquals("")
    }

    // Issue #1399 follow-up: path-arg links (`contacts/<id>`) share the save/restore path.
    private fun detailHost(): TestNavHostController {
        val navController = TestNavHostController(ApplicationProvider.getApplicationContext())
        composeTestRule.setContent {
            MycorrhizalTheme {
                navController.navigatorProvider.addNavigator(ComposeNavigator())
                NavHost(navController = navController, startDestination = "home") {
                    composable("home") { Text("Home") }
                    composable(
                        "contacts/{contactId}",
                        arguments = listOf(navArgument("contactId") { type = NavType.IntType }),
                    ) { Text("Contact ${it.arguments?.getInt("contactId")}", Modifier.testTag("detail")) }
                    composable("tags") {
                        var count by androidx.compose.runtime.saveable.rememberSaveable { mutableStateOf(0) }
                        Text("count $count", Modifier.testTag("count").clickable { count++ })
                    }
                }
            }
        }
        return navController
    }

    @Test
    fun `a contact link delivered while another contact is open shows the new contact`() {
        val navController = detailHost()

        navController.navigateToRoot("contacts/7")
        navController.navigateToRoot("contacts/8")
        composeTestRule.waitForIdle()

        assertEquals(8, navController.currentBackStackEntry?.arguments?.getInt("contactId"))
        composeTestRule.onNodeWithTag("detail").assertTextEquals("Contact 8")
        // #679: back from the deep-linked detail returns to the dashboard.
        navController.popBackStack()
        assertEquals("home", navController.currentBackStackEntry?.destination?.route)
    }

    @Test
    fun `a contact link after another contact was visited and left shows the new contact`() {
        val navController = detailHost()

        navController.navigateToRoot("contacts/7")
        navController.navigateToRoot("home")
        navController.navigateToRoot("contacts/8")
        composeTestRule.waitForIdle()

        assertEquals(8, navController.currentBackStackEntry?.arguments?.getInt("contactId"))
        composeTestRule.onNodeWithTag("detail").assertTextEquals("Contact 8")
        navController.popBackStack()
        assertEquals("home", navController.currentBackStackEntry?.destination?.route)
    }

    @Test
    fun `an argument-free destination still restores its saved state`() {
        val navController = detailHost()

        navController.navigateToRoot("tags")
        composeTestRule.onNodeWithTag("count").performClick()
        composeTestRule.onNodeWithTag("count").assertTextEquals("count 1")
        navController.navigateToRoot("home")
        navController.navigateToRoot("tags")
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithTag("count").assertTextEquals("count 1")
    }
}
