package com.mycorrhizal.crm

import android.app.Application
import androidx.compose.material3.Text
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.navigation.compose.ComposeNavigator
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.testing.TestNavHostController
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.data.session.RoleRefresher
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.flow.MutableSharedFlow
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issue #1826: when a re-fetch finds the user lost admin, an admin-only screen
 * is left for the dashboard (and the caller shows a snackbar); anywhere else the
 * navigation is untouched. Pinned against a real [TestNavHostController].
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class AdminRoutesTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun host(start: String = "home"): TestNavHostController {
        val navController = TestNavHostController(ApplicationProvider.getApplicationContext())
        composeTestRule.setContent {
            MycorrhizalTheme {
                navController.navigatorProvider.addNavigator(ComposeNavigator())
                NavHost(navController = navController, startDestination = start) {
                    composable("home") { Text("Home") }
                    composable("contacts") { Text("Contacts") }
                    composable("settings") { Text("Settings") }
                    composable("admin/users") { Text("Users") }
                    composable("admin/system-events") { Text("Events") }
                }
            }
        }
        return navController
    }

    private fun TestNavHostController.route() = currentBackStackEntry?.destination?.route

    @Test
    fun `admin routes are admin-only and ordinary routes are not`() {
        assertTrue(isAdminOnlyRoute("admin/users"))
        assertTrue(isAdminOnlyRoute("admin/system-events"))
        assertTrue(isAdminOnlyRoute("admin"))
        // #1833's Settings "Administration" page.
        assertTrue(isAdminOnlyRoute("settings/administration"))
        assertFalse(isAdminOnlyRoute("settings"))
        assertFalse(isAdminOnlyRoute("contacts"))
        // A prefix match must not catch look-alike routes.
        assertFalse(isAdminOnlyRoute("administrators"))
        assertFalse(isAdminOnlyRoute(null))
    }

    @Test
    fun `leaving an admin-only screen goes to the dashboard and reports it`() {
        val nav = host()
        nav.navigate("admin/users")
        assertEquals("admin/users", nav.route())

        assertTrue(nav.leaveAdminOnlyScreen())

        assertEquals("home", nav.route())
        // The admin screen is gone from the back stack: back does not return to it.
        assertEquals(null, nav.previousBackStackEntry)
    }

    @Test
    fun `if the dashboard is not on the stack it is navigated to`() {
        val nav = host(start = "contacts")
        nav.navigate("admin/users")

        assertTrue(nav.leaveAdminOnlyScreen())

        assertEquals("home", nav.route())
    }

    @Test
    fun `a screen that is not admin-only is left exactly where it is`() {
        val nav = host()
        nav.navigate("contacts")

        assertFalse(nav.leaveAdminOnlyScreen())

        assertEquals("contacts", nav.route())
        assertEquals("home", nav.previousBackStackEntry?.destination?.route)
    }

    @Test
    fun `a demotion on an admin screen leaves it and announces once, elsewhere it is silent`() = kotlinx.coroutines.test.runTest {
        val nav = host()
        var announced = 0
        nav.navigate("admin/users")
        nav.leaveAdminScreenOnDemotion(kotlinx.coroutines.flow.flowOf(Unit)) { announced++ }
        assertEquals("home", nav.route())
        assertEquals(1, announced)

        nav.leaveAdminScreenOnDemotion(kotlinx.coroutines.flow.flowOf(Unit)) { announced++ }
        assertEquals("not on an admin screen: no announcement", 1, announced)
    }

    @Test
    fun `the view model exposes the refresher's demotion events`() {
        val flow = MutableSharedFlow<Unit>()
        val refresher = mockk<RoleRefresher> { every { demotions } returns flow }

        assertSame(flow, PermissionChangeViewModel(refresher).demotions)
    }
}
