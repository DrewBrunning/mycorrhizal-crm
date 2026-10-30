package com.mycorrhizal.crm.model.network

import com.mycorrhizal.crm.model.MoshiProvider
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** Issue #2006: the optional multi-day `end_date` on the activity wire shapes. */
class ActivityEndDateTest {

    private val moshi = MoshiProvider.get()

    @Test
    fun `an activity with an end_date decodes it`() {
        val activity = moshi.adapter(Activity::class.java)
            .fromJson("""{"ID":7,"date":"2026-05-12T00:00:00Z","end_date":"2026-05-15T00:00:00Z"}""")
        assertEquals("2026-05-15T00:00:00Z", activity?.endDate)
    }

    @Test
    fun `an activity without end_date, or with null, decodes a null end`() {
        val adapter = moshi.adapter(Activity::class.java)
        assertNull(adapter.fromJson("""{"ID":7,"date":"2026-05-12T00:00:00Z"}""")?.endDate)
        assertNull(adapter.fromJson("""{"ID":7,"end_date":null}""")?.endDate)
    }

    @Test
    fun `an input with an end writes it`() {
        val json = moshi.adapter(ActivityInput::class.java)
            .toJson(ActivityInput(title = "Trip", endDate = "2026-05-15T00:00:00Z"))
        assertTrue(json, json.contains(""""end_date":"2026-05-15T00:00:00Z""""))
    }

    @Test
    fun `an input with no end writes an explicit null so an update clears it`() {
        val json = moshi.adapter(ActivityInput::class.java).toJson(ActivityInput(title = "Trip"))
        assertTrue(json, json.contains(""""end_date":null"""))
        // Other absent fields stay omitted: only end_date needs the explicit null.
        assertFalse(json, json.contains("location"))
    }
}
