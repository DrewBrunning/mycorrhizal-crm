package com.mycorrhizal.crm.e2e

import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Issue #2002: the contact Trash screen's restore and permanent-delete flows
 * against the real backend.
 *
 * The delete goes through the real contact action menu, then the test lists,
 * restores and purges through the drawer's Trash destination. Server state is
 * the ground truth (the unit tests already pin the screen's rendering).
 */
@RunWith(AndroidJUnit4::class)
class TrashFlowE2eTest : E2eBaseTest() {

    @Test
    fun deleteRestoreAndPurgeThroughTheTrashScreen() {
        val restoreGiven = uniqueName("TrashRestore")
        val restoreDisplay = "$restoreGiven Delta"
        val restore = createTestContact(restoreGiven, "Delta")

        // Delete through the detail action menu + confirmation.
        navigateViaDrawer("Contacts")
        searchFor(restoreGiven)
        waitForText(restoreDisplay)
        clickTextWhenStable(restoreDisplay)
        clickContentDescription("Contact actions")
        waitForText("Delete contact")
        compose.onNodeWithText("Delete contact").performClick()
        waitForText("Delete contact?")
        onLastText("Delete").performClick()

        compose.waitUntil(15_000) {
            backend.trashEntries().any {
                it.getJSONObject("contact").optLong("id") == restore.id
            }
        }

        // Restore from the Trash screen; the contact is live again server-side.
        navigateViaDrawer("Trash")
        waitForTag("trash-restore-${restore.id}")
        compose.onNodeWithTag("trash-restore-${restore.id}").performClick()
        compose.waitUntil(15_000) { backend.contactExists(restore.id) }

        // Delete directly through the API, then purge through the screen.
        val purgeGiven = uniqueName("TrashPurge")
        val purge = createTestContact(purgeGiven, "Delta")
        backend.deleteContact(purge.id)

        // Leave and re-enter the destination so its list reloads.
        navigateViaDrawer("Contacts")
        navigateViaDrawer("Trash")
        waitForTag("trash-delete-${purge.id}")
        compose.onNodeWithTag("trash-delete-${purge.id}").performClick()
        waitForTag("trash-purge-confirm")
        compose.onNodeWithTag("trash-purge-confirm").performClick()

        compose.waitUntil(15_000) { !backend.contactExists(purge.id) }
        assertTrue(
            "the purged contact must leave the trash",
            backend.trashEntries().none {
                it.getJSONObject("contact").optLong("id") == purge.id
            }
        )
    }
}
