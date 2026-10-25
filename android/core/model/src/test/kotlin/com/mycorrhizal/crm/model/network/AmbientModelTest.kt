package com.mycorrhizal.crm.model.network

import com.mycorrhizal.crm.model.MoshiProvider
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** Issue #1734: the wire shape of the ambient fields (backend #1733). */
class AmbientModelTest {

    private val moshi = MoshiProvider.get()

    @Test
    fun `crm ambient is null when absent, and true or false when present`() {
        val adapter = moshi.adapter(CRMEnvelope::class.java)
        assertNull(adapter.fromJson("""{"kind":"human"}""")?.ambient)
        assertEquals(true, adapter.fromJson("""{"ambient":true}""")?.ambient)
        assertEquals(false, adapter.fromJson("""{"ambient":false}""")?.ambient)
    }

    @Test
    fun `crm ambient is omitted when null so the server keeps its value, and written when set`() {
        val adapter = moshi.adapter(CRMEnvelope::class.java)
        assertFalse(adapter.toJson(CRMEnvelope(kind = "human")).contains("ambient"))
        assertEquals(
            true,
            adapter.toJson(CRMEnvelope(ambient = true)).contains(""""ambient":true""")
        )
        // An explicit false must be on the wire: it is how the user clears the flag.
        assertEquals(
            true,
            adapter.toJson(CRMEnvelope(ambient = false)).contains(""""ambient":false""")
        )
    }

    @Test
    fun `score parses ambient with a reason, and defaults for an older server`() {
        val adapter = moshi.adapter(ContactScoreResponse::class.java)
        val household = adapter.fromJson(
            """{"contact_id":1,"score":88,"band":"moss","ambient":true,"ambient_reason":"household"}"""
        )
        assertEquals(true, household?.ambient)
        assertEquals(AMBIENT_REASON_HOUSEHOLD, household?.ambientReason)
        val flag = adapter.fromJson("""{"contact_id":1,"ambient":true,"ambient_reason":"flag"}""")
        assertEquals(AMBIENT_REASON_FLAG, flag?.ambientReason)

        val normal = adapter.fromJson(
            """{"contact_id":1,"score":50,"band":"chanterelle","ambient":false}"""
        )
        assertFalse(normal!!.ambient)
        assertNull(normal.ambientReason)
        val old = adapter.fromJson("""{"contact_id":1,"score":50,"band":"chanterelle"}""")
        assertFalse(old!!.ambient)
    }

    @Test
    fun `cadence health parses ambient, and an ambient contact is never overdue`() {
        val adapter = moshi.adapter(CadenceHealth::class.java)
        val ambient = adapter.fromJson(
            """{"has_qualifying_interaction":true,"overdue_by":0,"ambient":true,"ambient_reason":"flag"}"""
        )!!
        assertTrue(ambient.ambient)
        assertEquals("flag", ambient.ambientReason)
        assertFalse(ambient.isOverdue)

        // A stale payload that disagrees with itself still reads as not overdue.
        assertFalse(CadenceHealth(overdueBy = 5, ambient = true).isOverdue)
        assertTrue(CadenceHealth(overdueBy = 5).isOverdue)
        assertFalse(CadenceHealth(overdueBy = 0).isOverdue)
        // Older server: no ambient keys.
        assertFalse(
            adapter.fromJson("""{"has_qualifying_interaction":true,"overdue_by":0}""")!!.ambient
        )
    }

    @Test
    fun `briefing cadence health parses ambient, and an ambient contact is never overdue`() {
        val adapter = moshi.adapter(BriefingCadenceHealth::class.java)
        val ambient = adapter.fromJson(
            """{"has_qualifying_interaction":true,"overdue_by":0,"ambient":true,"ambient_reason":"household"}"""
        )!!
        assertTrue(ambient.ambient)
        assertEquals("household", ambient.ambientReason)
        assertFalse(ambient.isOverdue)

        assertFalse(BriefingCadenceHealth(overdueBy = 5, ambient = true).isOverdue)
        assertTrue(BriefingCadenceHealth(overdueBy = 5).isOverdue)
    }
}
