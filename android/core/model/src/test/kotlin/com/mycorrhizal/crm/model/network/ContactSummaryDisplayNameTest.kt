package com.mycorrhizal.crm.model.network

import org.junit.Assert.assertEquals
import org.junit.Test

class ContactSummaryDisplayNameTest {

    @Test
    fun `derives the display name from firstname and lastname instead of a given-name-only fn`() {
        // The backend's fn is often only the given name; web's list renders
        // "Elizabeth Brunning", and Android now does the same (M5 §3.2).
        val summary = ContactSummary(
            id = 1,
            uid = "u1",
            firstname = "Elizabeth",
            lastname = "Brunning",
            fn = "Elizabeth",
        )
        assertEquals("Elizabeth Brunning", summary.displayName)
    }

    @Test
    fun `includes the nickname between firstname and lastname, quoted like web`() {
        val summary = ContactSummary(
            id = 1,
            uid = "u1",
            firstname = "Elizabeth",
            nickname = "Liz",
            lastname = "Brunning",
            fn = "Elizabeth",
        )
        assertEquals("Elizabeth \"Liz\" Brunning", summary.displayName)
    }

    @Test
    fun `falls back to fn when no name components are present`() {
        val summary = ContactSummary(id = 2, uid = "u2", fn = "The Dude")
        assertEquals("The Dude", summary.displayName)
    }

    @Test
    fun `falls back to the id when nothing at all is present`() {
        val summary = ContactSummary(id = 9, uid = "u9")
        assertEquals("#9", summary.displayName)
    }
}
