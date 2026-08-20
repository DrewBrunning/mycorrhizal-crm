package com.mycorrhizal.crm.e2e

import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Issue #238: the primary full-app flow — login → contact list → detail →
 * edit → list refresh — against the real docker-compose.test.yml backend.
 *
 * Every assertion that depends on server data polls via the wait* helpers,
 * since Compose's idle detection cannot know when a network round-trip has
 * landed.
 */
@RunWith(AndroidJUnit4::class)
class LoginListDetailEditTest : E2eBaseTest() {

    private val givenName = uniqueName("Edit")
    private val surname = "Temp"
    private val originalDisplayName = "$givenName $surname"
    private val renamedGiven = "$givenName Renamed"
    private val renamedDisplayName = "$renamedGiven $surname"

    @Before
    fun seedContact() {
        createTestContact(givenName, surname)
    }

    @Test
    fun loginThenListDetailEditThenListRefresh() {
        // Base @Before logged in through the real login UI — land on the
        // dashboard.
        waitForText("Dashboard")

        // --- contact list ---
        navigateViaDrawer("Contacts")
        // Search by the given name so the search field's own text ("<given>")
        // never collides with the row's display text ("<given> <surname>").
        searchFor(givenName)
        waitForText(originalDisplayName)
        compose.onNodeWithText(originalDisplayName).performClick()

        // --- detail ---
        waitForText(originalDisplayName)
        waitForContentDescription("Edit contact")

        // --- edit ---
        clickContentDescription("Edit contact")
        replaceTextInField("Given name", renamedGiven)
        compose.onNodeWithText("Save changes").performScrollTo().performClick()

        // Back on the detail page, now showing the renamed contact (the detail
        // reloads on resume).
        waitForText(renamedDisplayName)

        // --- list refresh ---
        clickContentDescription("Back")
        waitForText("Search contacts")
        // The old search no longer matches; the list reloaded on resume, so
        // re-searching for the new name must surface the renamed row — proving
        // the list reflects the server-side edit, not a cached page.
        searchFor(renamedGiven)
        waitForText(renamedDisplayName)
    }
}
