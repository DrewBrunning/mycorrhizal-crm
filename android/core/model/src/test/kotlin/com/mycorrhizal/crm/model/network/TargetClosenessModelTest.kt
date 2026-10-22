package com.mycorrhizal.crm.model.network

import com.mycorrhizal.crm.model.MoshiProvider
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** Issue #1798: the wire shape of target closeness (backend #1796). */
class TargetClosenessModelTest {

    private val moshi = MoshiProvider.get()

    @Test
    fun `target closeness response parses an offer and round-trips`() {
        val adapter = moshi.adapter(ContactTargetClosenessResponse::class.java)
        val response = adapter.fromJson(
            """
            {
              "contact_id": 12,
              "target_closeness": "close",
              "default_interval_days": 30,
              "cadence_policy_offer": {"entity_id": "u12", "target_interval_days": 30}
            }
            """.trimIndent(),
        )!!

        assertEquals(12, response.contactId)
        assertEquals("close", response.targetCloseness)
        assertEquals(30, response.defaultIntervalDays)
        assertEquals("u12", response.cadencePolicyOffer?.entityId)
        assertEquals(30, response.cadencePolicyOffer?.targetIntervalDays)

        // A response model must survive its own encoding unchanged.
        assertEquals(response, adapter.fromJson(adapter.toJson(response)))
    }

    @Test
    fun `target closeness response parses the all-null DELETE shape`() {
        val response = moshi.adapter(ContactTargetClosenessResponse::class.java).fromJson(
            """{"contact_id":12,"target_closeness":null,"default_interval_days":null,"cadence_policy_offer":null}""",
        )!!

        assertEquals(12, response.contactId)
        assertNull(response.targetCloseness)
        assertNull(response.defaultIntervalDays)
        assertNull(response.cadencePolicyOffer)
    }

    @Test
    fun `score target is null when clear and parses every field when set`() {
        val adapter = moshi.adapter(ContactScoreResponse::class.java)

        val cleared = adapter.fromJson(
            """{"contact_id":1,"score":50,"band":"chanterelle","target":null}""",
        )!!
        assertNull(cleared.target)

        // An older server that predates the field also reads as no target.
        val old = adapter.fromJson("""{"contact_id":1,"score":50,"band":"chanterelle"}""")!!
        assertNull(old.target)

        val targeted = adapter.fromJson(
            """
            {
              "contact_id": 1,
              "score": 72,
              "band": "moss",
              "target": {
                "tier": "close",
                "status": "drifting",
                "value": 65.9,
                "interval_days": 30,
                "interval_source": "target_tier",
                "reason": "Recency and frequency score 66 against a 30-day interval from the close target"
              }
            }
            """.trimIndent(),
        )!!

        val target = targeted.target!!
        assertEquals("close", target.tier)
        assertEquals(TargetStatus.DRIFTING, target.status)
        assertEquals(65.9, target.value, 0.0)
        assertEquals(30, target.intervalDays)
        assertEquals("target_tier", target.intervalSource)
        assertEquals(
            "Recency and frequency score 66 against a 30-day interval from the close target",
            target.reason,
        )
    }

    @Test
    fun `score target has data-class value semantics`() {
        val target = ContactScoreTarget(
            tier = "close",
            status = TargetStatus.DRIFTING,
            value = 65.9,
            intervalDays = 30,
            intervalSource = "target_tier",
            reason = "reason",
        )

        // Pins copy/equals/hashCode/toString/componentN so the DTO behaves like
        // every other generated model the ViewModel copies into state.
        assertEquals(target, target.copy())
        assertEquals(target.hashCode(), target.copy().hashCode())
        assertEquals(target.toString(), target.copy().toString())
        // Destructuring covers componentN for the first three; detekt caps a
        // single destructuring declaration at three entries, so the rest are
        // read directly to pin every generated component function.
        val (tier, status, value) = target
        assertEquals("close", tier)
        assertEquals(TargetStatus.DRIFTING, status)
        assertEquals(65.9, value, 0.0)
        assertEquals(30, target.component4())
        assertEquals("target_tier", target.component5())
        assertEquals("reason", target.component6())

        // Defaults match the backend's zero-value DTO shape.
        val empty = ContactScoreTarget()
        assertEquals("", empty.tier)
        assertEquals("", empty.status)
        assertEquals(0.0, empty.value, 0.0)
        assertEquals(0, empty.intervalDays)
        assertEquals("", empty.intervalSource)
        assertEquals("", empty.reason)
    }
}
