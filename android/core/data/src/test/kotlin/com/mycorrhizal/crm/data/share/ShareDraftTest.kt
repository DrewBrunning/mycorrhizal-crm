package com.mycorrhizal.crm.data.share

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Test

class ShareDraftTest {

    @Test
    fun `control and format characters are stripped but newline and tab are kept`() {
        val raw = "a\u0000b\u0007c‮d​e\nf\tg\r"
        // \u0000/\u0007 = Cc, ‮ (RLO) / ​ (ZWSP) = Cf, \r = Cc.
        assertEquals("abcde\nf\tg", sanitizeSharedText(raw))
    }

    @Test
    fun `text at the limit is kept whole and one past it is truncated with an ellipsis`() {
        val exact = "x".repeat(SHARE_TEXT_MAX_CHARS)
        assertEquals(exact, sanitizeSharedText(exact))

        val over = sanitizeSharedText("x".repeat(SHARE_TEXT_MAX_CHARS + 1))!!
        assertEquals(SHARE_TEXT_MAX_CHARS + 1, over.length)
        assertEquals("x".repeat(SHARE_TEXT_MAX_CHARS) + "…", over)
    }

    @Test
    fun `null blank and all-control text sanitise to null`() {
        assertNull(sanitizeSharedText(null))
        assertNull(sanitizeSharedText("   \n\t "))
        assertNull(sanitizeSharedText("\u0000​"))
    }

    @Test
    fun `subject is prepended with a blank line`() {
        assertEquals("Title\n\nBody", composeSharedDraft("Title", "Body"))
    }

    @Test
    fun `missing subject or text falls back to the other`() {
        assertEquals("Body", composeSharedDraft(null, "Body"))
        assertEquals("Body", composeSharedDraft("  ", "Body"))
        assertEquals("Title", composeSharedDraft("Title", null))
        assertNull(composeSharedDraft(null, null))
        assertNull(composeSharedDraft("\u0000", "​"))
    }

    @Test
    fun `subject is sanitised too`() {
        assertEquals("Hi\n\nBody", composeSharedDraft("H‮i", "Body"))
    }

    @Test
    fun `take is single use`() {
        val holder = ShareDraftHolder()
        val key = holder.put("hello")
        assertEquals("hello", holder.take(key))
        assertNull(holder.take(key))
    }

    @Test
    fun `keys are unique and unknown keys yield null`() {
        val holder = ShareDraftHolder()
        val a = holder.put("a")
        val b = holder.put("a")
        assertNotEquals(a, b)
        assertNull(holder.take("nope"))
    }

    @Test
    fun `clear discards every held draft`() {
        val holder = ShareDraftHolder()
        val key = holder.put("secret")
        holder.clear()
        assertNull(holder.take(key))
    }
}
