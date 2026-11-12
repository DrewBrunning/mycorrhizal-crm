package com.mycorrhizal.crm.model.network

import com.mycorrhizal.crm.model.MoshiProvider
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * ADR 0024 (issue #2521): the standing-obligation wire models parse the backend
 * response (`backend/models/occasion_obligation.go`) and serialize the full-replace input
 * (`OccasionObligationInput`) with the exact snake_case keys.
 */
class OccasionObligationModelTest {

    private val moshi = MoshiProvider.get()

    @Test
    fun `a full backend obligation parses every field`() {
        val json = """{"id":"o1","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-02T00:00:00Z",
            "entity_id":"11111111-1111-4111-8111-111111111111","kind":"gift","label":"Birthday gift",
            "anchor_month":12,"anchor_day":25,"linked_life_event_id":"le1","lead_time_days":14,
            "active":false,"sensitivity":"private","notes":"likes tea"}"""
        val o = moshi.adapter(OccasionObligation::class.java).fromJson(json)!!
        assertEquals("o1", o.id)
        assertEquals("11111111-1111-4111-8111-111111111111", o.entityId)
        assertEquals("gift", o.kind)
        assertEquals(12, o.anchorMonth)
        assertEquals(25, o.anchorDay)
        assertEquals("le1", o.linkedLifeEventId)
        assertEquals(14, o.leadTimeDays)
        assertFalse(o.active)
        assertEquals("private", o.sensitivity)
        assertEquals("likes tea", o.notes)
        assertFalse(o.deleted)
    }

    @Test
    fun `a sparse obligation has no anchor and safe defaults`() {
        val o = moshi.adapter(OccasionObligation::class.java)
            .fromJson(
                """{"id":"o2","entity_id":"e","kind":"card","label":"Xmas","lead_time_days":0,"active":true,"sensitivity":"normal"}"""
            )!!
        assertNull(o.anchorMonth)
        assertNull(o.anchorDay)
        assertNull(o.notes)
        assertFalse(o.hasAnchor)
        assertTrue(o.active)
    }

    @Test
    fun `hasAnchor needs both month and day`() {
        assertTrue(OccasionObligation(anchorMonth = 3, anchorDay = 4).hasAnchor)
        assertFalse(OccasionObligation(anchorMonth = 3).hasAnchor)
        assertFalse(OccasionObligation(anchorDay = 4).hasAnchor)
    }

    @Test
    fun `a list page and the create wrapper unwrap the way the backend sends them`() {
        val page = moshi.adapter(OccasionObligationsResponse::class.java).fromJson(
            """{"occasion_obligations":[{"id":"a"},{"id":"b","deleted":true}],"total":2,"next_cursor":"c2","limit":100}"""
        )!!
        assertEquals(listOf("a", "b"), page.occasionObligations.map { it.id })
        assertTrue(page.occasionObligations[1].deleted)
        assertEquals("c2", page.nextCursor)
        val created = moshi.adapter(CreateOccasionObligationResponse::class.java)
            .fromJson("""{"message":"ok","occasion_obligation":{"id":"n1"}}""")!!
        assertEquals("n1", created.occasionObligation?.id)
    }

    @Test
    fun `input serializes snake_case keys and omits an unset anchor`() {
        val json = moshi.adapter(OccasionObligationInput::class.java).toJson(
            OccasionObligationInput(
                entityId = "e1",
                kind = "card",
                label = "Xmas",
                leadTimeDays = 7,
                active = true,
                sensitivity = "normal"
            )
        )
        assertTrue(json, json.contains(""""entity_id":"e1""""))
        assertTrue(json, json.contains(""""lead_time_days":7"""))
        assertTrue(json, json.contains(""""active":true"""))
        assertFalse(json, json.contains("anchor_month"))
        assertFalse(json, json.contains("anchor_day"))
    }

    @Test
    fun `kind suggestions mirror the backend's named kinds`() {
        assertEquals(listOf("card", "gift", "invite"), OccasionObligationKinds.ALL)
    }
}
