package com.mycorrhizal.crm.e2e

import androidx.compose.ui.test.assertIsOff
import androidx.compose.ui.test.assertIsOn
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Issue #238: archive/delete and the audit undo round-trip against the real
 * backend.
 *
 * Archive and delete go through the real detail-screen action menu; the audit
 * assertions read the real /audit log (via the app's own audit screen for the
 * undo, and via [E2eBackend] for the contract checks the screen doesn't
 * expose). Undo is a contact-*update*-only contract on the backend (a delete
 * event 400s), so the round-trip is exercised on the archive, which fires an
 * update event.
 */
@RunWith(AndroidJUnit4::class)
class ArchiveDeleteAuditTest : E2eBaseTest() {

    private val archGiven = uniqueName("Arch")
    private val archSurname = "Gamma"
    private val archDisplayName = "$archGiven $archSurname"

    private val delGiven = uniqueName("Del")
    private val delSurname = "Delta"
    private val delDisplayName = "$delGiven $delSurname"

    private lateinit var arch: E2eBackend.SeedContact
    private lateinit var del: E2eBackend.SeedContact

    @Before
    fun seedContacts() {
        arch = createTestContact(archGiven, archSurname)
        del = createTestContact(delGiven, delSurname)
    }

    @Test
    fun archiveContactThenUndoViaAuditRestoresIt() {
        navigateViaDrawer("Contacts")
        searchFor(archGiven)
        waitForText(archDisplayName)
        compose.onNodeWithText(archDisplayName).performClick()

        // Archive through the detail action menu + confirm dialog.
        clickContentDescription("Contact actions")
        waitForText("Archive")
        compose.onNodeWithText("Archive").performClick()
        waitForText("Archive contact?")
        onLastText("Archive").performClick()

        // The archive round-tripped: the contact is archived server-side.
        compose.waitUntil(15_000) { backend.contactState(arch.id).optBoolean("archived") }

        // Back on the list: hidden by default, visible with the archived
        // toggle on.
        clickBack()
        waitForText("Search contacts")
        compose.onNodeWithTag("archived-toggle").assertIsOff()
        compose.onNodeWithTag("archived-toggle").performClick()
        waitForText(archDisplayName)
        compose.onNodeWithTag("archived-toggle").assertIsOn()
        // Leave the toggle off for the post-undo default-view check.
        compose.onNodeWithTag("archived-toggle").performClick()

        // The archive fired a contact-update audit event, which is the one
        // kind undo accepts (its vcard uid is the audit entity_id).
        val updateEvent = backend.auditEvents(entityType = "contact", entityId = arch.uid)
            .firstOrNull { it.optString("operation") == "update" }
        assertNotNull("expected a contact update event for the archive", updateEvent)
        val undoTag = "audit-undo-${updateEvent!!.optLong("id")}"

        // Undo through the app's own audit screen.
        navigateViaDrawer("Audit log")
        waitForTag(undoTag)
        compose.onNodeWithTag(undoTag).performClick()
        waitForText("Undo this change?")
        onLastText("Undo").performClick()
        waitForText("Contact restored to its previous state")

        // Undo restored the pre-archive state (unarchived).
        compose.waitUntil(15_000) { !backend.contactState(arch.id).optBoolean("archived") }

        // Back out of the audit screen (a drawer destination pops to home, not
        // back to the contacts list) and into a fresh contacts view — with the
        // archived toggle off, the restored contact shows in the default list.
        clickBack()
        waitForText("Dashboard")
        navigateViaDrawer("Contacts")
        searchFor(archGiven)
        waitForText(archDisplayName)
    }

    @Test
    fun deleteContactIsLoggedAndUndoRejectsDelete() {
        navigateViaDrawer("Contacts")
        searchFor(delGiven)
        waitForText(delDisplayName)
        compose.onNodeWithText(delDisplayName).performClick()

        // Delete through the detail action menu + confirm dialog.
        clickContentDescription("Contact actions")
        waitForText("Delete contact")
        compose.onNodeWithText("Delete contact").performClick()
        waitForText("Delete contact?")
        onLastText("Delete").performClick()

        // The detail navigates back on delete; the row is gone from the list.
        waitForText("Search contacts")
        waitForTextGone(delDisplayName)

        // The delete is recorded in the audit log...
        val deleteEvent = backend.auditEvents(entityType = "contact", entityId = del.uid)
            .firstOrNull { it.optString("operation") == "delete" }
        assertNotNull("expected a delete audit event", deleteEvent)
        // ...and undo rejects it: the backend contract is update-only.
        assertEquals(400, backend.undoAuditEvent(deleteEvent!!.optLong("id")))
    }
}
