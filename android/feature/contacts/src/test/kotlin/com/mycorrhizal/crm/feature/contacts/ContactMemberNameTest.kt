package com.mycorrhizal.crm.feature.contacts

import com.mycorrhizal.crm.model.network.ContactSummary
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

// Issue #1768: circle/tag/household member rows show a resolved name, and never
// leak the `#id` placeholder that ContactSummary.displayName falls back to.
class ContactMemberNameTest {

    @Test
    fun `an unresolved contact has no display name`() {
        assertNull(memberDisplayName(null))
    }

    @Test
    fun `a resolved contact uses its derived display name`() {
        assertEquals(
            "Alice Smith",
            memberDisplayName(ContactSummary(id = 1, firstname = "Alice", lastname = "Smith"))
        )
    }

    @Test
    fun `a nickname is rendered by the shared derivation`() {
        assertEquals(
            "\"Ally\"",
            memberDisplayName(ContactSummary(id = 1, nickname = "Ally"))
        )
    }

    @Test
    fun `the id placeholder with no nickname stays unresolved`() {
        assertNull(memberDisplayName(ContactSummary(id = 1)))
    }
}
