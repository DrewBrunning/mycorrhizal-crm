package com.mycorrhizal.crm

import android.app.Application
import androidx.compose.material3.Text
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
}
