package com.mycorrhizal.crm.feature.tracking

import com.mycorrhizal.crm.model.network.ContactSummary
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class QuickCapturePrefillTest {

    private val nowIso = "2026-08-18T12:00:00Z"

    private val jane = ContactSummary(
        id = 7,
        uid = "urn:uuid:jane",
        firstname = "Jane",
        lastname = "Doe",
        nickname = "Jay",
    )

    @Test
    fun `a matched contact seeds the participant and title`() {
        val prefill = QuickCapturePrefillFactory.forCall(jane, nowIso)

        assertEquals("Call", prefill.title)
        assertEquals("call", prefill.type)
        assertEquals(nowIso, prefill.date)
        assertEquals(1, prefill.participants.size)
        assertEquals(7, prefill.participants[0].id)
        // The canonical app display name — `firstname "nickname" lastname`.
        assertEquals("Jane \"Jay\" Doe", prefill.contactName)
    }

    @Test
    fun `an unknown number yields a contact-less activity rather than dropping it`() {
        val prefill = QuickCapturePrefillFactory.forCall(null, nowIso)

        assertTrue(prefill.participants.isEmpty())
        assertNull(prefill.contactName)
        assertEquals("Call", prefill.title)
        assertEquals("call", prefill.type)
    }

    @Test
    fun `a contact with only a nickname still names the chip`() {
        val nickOnly = ContactSummary(id = 3, nickname = "Jay")
        val prefill = QuickCapturePrefillFactory.forCall(nickOnly, nowIso)

        assertEquals("\"Jay\"", prefill.contactName)
        assertEquals("Jay", prefill.participants[0].nickname)
    }
}
