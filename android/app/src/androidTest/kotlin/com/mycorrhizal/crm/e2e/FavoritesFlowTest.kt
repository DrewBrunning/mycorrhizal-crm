package com.mycorrhizal.crm.e2e

import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Issue #238: the favorites flow (issue #212) end-to-end against the real
 * backend — star on a list row, favorites-only filter, dashboard Favorites
 * block, header star on the detail page, and the star state surviving
 * navigation and a full activity relaunch.
 *
 * Note on the dashboard block: DashboardViewModel only loads in `init` (no
 * resume reload), so a dashboard that was composed before the favorite existed
 * shows stale data. The test therefore re-authenticates after favoriting — a
 * fresh NavHost/ViewModel — so the Favorites block reflects the server truth.
 */
@RunWith(AndroidJUnit4::class)
class FavoritesFlowTest : E2eBaseTest() {

    private val favGiven = uniqueName("Fav")
    private val favSurname = "Alpha"
    private val favDisplayName = "$favGiven $favSurname"

    private val plainGiven = uniqueName("Plain")
    private val plainSurname = "Beta"
    private val plainDisplayName = "$plainGiven $plainSurname"

    @Before
    fun seedContacts() {
        createTestContact(favGiven, favSurname)
        createTestContact(plainGiven, plainSurname)
    }

    @Test
    fun starOnListThenFilterThenDashboardBlockThenHeaderStarThenRelaunch() {
        // --- star on a list row --------------------------------------------
        navigateViaDrawer("Contacts")
        // Search by the given name so the search field's own text never
        // collides with the row's display text.
        searchFor(favGiven)
        waitForText(favDisplayName)
        compose.onNodeWithContentDescription("Mark $favDisplayName as favorite").performClick()
        // The optimistic flip is reconciled against the server: the label
        // changing to "Unmark …" proves the POST round-tripped.
        waitForContentDescription("Unmark $favDisplayName as favorite")

        // --- favorites-only filter ------------------------------------------
        // Scope to this run's two contacts so the filter's effect is
        // unambiguous.
        searchFor(E2eConfig.TEST_CONTACT_PREFIX)
        waitForText(favDisplayName)
        waitForText(plainDisplayName)
        compose.onNodeWithTag("favorites-toggle").performClick()
        // The toggle triggers a reload (empty list + skeleton first); waiting
        // for the favorite to reappear means the reload has rendered, so the
        // non-favorite's absence is now meaningful.
        waitForText(favDisplayName)
        compose.onAllNodesWithText(plainDisplayName).assertCountEquals(0)

        // --- dashboard Favorites block --------------------------------------
        // Fresh NavHost so the dashboard actually loads the now-existing
        // favorite (see the class doc).
        clearSession()
        loginViaUi()
        waitForText("Favorites")
        waitForText(favDisplayName)

        // --- header star on the detail page ---------------------------------
        onFirstText(favDisplayName).performClick()
        waitForContentDescription("Unmark $favDisplayName as favorite")
        // Toggle off, then back on — the star must round-trip through the
        // server both ways.
        compose.onNodeWithContentDescription("Unmark $favDisplayName as favorite").performClick()
        waitForContentDescription("Mark $favDisplayName as favorite")
        compose.onNodeWithContentDescription("Mark $favDisplayName as favorite").performClick()
        waitForContentDescription("Unmark $favDisplayName as favorite")

        // --- star state survives navigation ---------------------------------
        clickBack()
        waitForText("Favorites")
        waitForText(favDisplayName)

        // --- star state survives a full relaunch ----------------------------
        compose.activityRule.scenario.recreate()
        waitForDashboardOrLogin()
        waitForText("Favorites")
        waitForText(favDisplayName)

        // The list row still shows the filled star after relaunch.
        navigateViaDrawer("Contacts")
        searchFor(favGiven)
        waitForText(favDisplayName)
        waitForContentDescription("Unmark $favDisplayName as favorite")
    }

    private fun waitForDashboardOrLogin() {
        compose.waitUntil(30_000) {
            compose.onAllNodesWithText("Dashboard").fetchSemanticsNodes().isNotEmpty() ||
                compose.onAllNodesWithText("Sign in").fetchSemanticsNodes().isNotEmpty()
        }
        val loggedIn = compose.onAllNodesWithText("Dashboard").fetchSemanticsNodes().isNotEmpty()
        if (!loggedIn) {
            // Session hydration raced the recreation — recover deterministically.
            loginViaUi()
        }
    }
}
