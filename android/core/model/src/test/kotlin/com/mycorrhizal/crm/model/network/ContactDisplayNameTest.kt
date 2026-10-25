package com.mycorrhizal.crm.model.network

import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * Issue #1762: the dashboard and the contact list share one display-name
 * rule — components first with the nickname quoted — instead of the
 * dashboard's old nickname-only variant.
 */
class ContactDisplayNameTest {

    @Test
    fun `a nickname is quoted between first and last name`() {
        val contact = ContactSummary(
            id = 1,
            firstname = "Aoife",
            nickname = "Eefa",
            lastname = "Ó Conaill"
        )

        assertEquals("Aoife \"Eefa\" Ó Conaill", contact.displayName)
        assertEquals(
            "Aoife \"Eefa\" Ó Conaill",
            contactDisplayName("Aoife", "Eefa", "Ó Conaill")
        )
    }

    @Test
    fun `a dashboard-shaped contact renders the same as the list row`() {
        // The dashboard's random-contact projection and the list row both go
        // through this function, so these two calls are deliberately identical.
        val rendered = contactDisplayName(
            firstname = "Pinky",
            nickname = null,
            lastname = "शर्मा",
            fallbackId = "#7"
        )

        assertEquals("Pinky शर्मा", rendered)
    }

    @Test
    fun `blank components are skipped and fn is the fallback`() {
        assertEquals(
            "Elizabeth Brunning",
            contactDisplayName("Elizabeth", "", "Brunning", fn = "Liz")
        )
        assertEquals("Liz", contactDisplayName("", " ", null, fn = "Liz"))
        assertEquals("#9", contactDisplayName(null, null, "", fn = null, fallbackId = "#9"))
    }
}
