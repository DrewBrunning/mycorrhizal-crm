package com.mycorrhizal.crm.ui.components

import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * Issue #2377: [looksLikeEmail] only drives a "doesn't look like an email
 * address" hint. It is a forgiving shape check, not a validator, and mirrors
 * the web's `frontend/src/utils/emailFormat.ts` table.
 */
class EmailFormatTest {

    @Test
    fun `plausible addresses are not flagged`() {
        listOf(
            "alice@example.com",
            "a.b+tag@sub.example.co.uk",
            "  padded@example.com  ",
            "üser@exämple.de"
        ).forEach { assertEquals(it, true, looksLikeEmail(it)) }
    }

    @Test
    fun `malformed addresses are flagged`() {
        listOf(
            "not-an-email",
            "missing@tld",
            "@example.com",
            "two@@example.com",
            "spa ce@example.com",
            "alice@"
        ).forEach { assertEquals(it, false, looksLikeEmail(it)) }
    }

    @Test
    fun `a blank value is not flagged because there is nothing to check yet`() {
        assertEquals(true, looksLikeEmail(""))
        assertEquals(true, looksLikeEmail("   "))
    }
}
