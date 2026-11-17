package com.mycorrhizal.crm.feature.circles

import androidx.compose.ui.graphics.Color
import com.mycorrhizal.crm.model.network.CircleColors
import com.mycorrhizal.crm.model.network.CircleTimelineItem
import com.mycorrhizal.crm.model.util.DateFormat
import java.util.Locale
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test

class CircleTimelineFormatTest {

    private fun item(data: Map<String, Any?>?, date: String = "2026-09-01T10:00:00Z") =
        CircleTimelineItem(type = "note", id = "1", date = date, data = data)

    @Test
    fun `summary prefers content then title then message then name then description then type`() {
        assertEquals("c", item(mapOf("content" to "c", "title" to "t")).summary())
        assertEquals(
            "t",
            item(mapOf("content" to "  ", "title" to "t", "message" to "m")).summary()
        )
        assertEquals("m", item(mapOf("message" to "m", "name" to "n")).summary())
        assertEquals("n", item(mapOf("name" to "n", "description" to "d")).summary())
        assertEquals("d", item(mapOf("description" to "d", "type" to "moved")).summary())
        assertEquals("moved", item(mapOf("type" to "moved")).summary())
    }

    @Test
    fun `summary skips non-string values and tolerates missing data`() {
        assertEquals("t", item(mapOf("content" to 12, "title" to "t")).summary())
        assertEquals("", item(null).summary())
        assertEquals("", item(emptyMap()).summary())
    }

    @Test
    fun `a date is rendered from its calendar day without a timezone shift`() {
        // 23:30 at -05:00 must stay on the 1st (web uses calendarDateOnly).
        assertEquals(
            "01.09.2026",
            formatTimelineDate("2026-09-01T23:30:00-05:00", null, DateFormat.EU, Locale.US)
        )
    }

    @Test
    fun `a date follows the user's date format preference`() {
        val iso = "2026-09-01T10:00:00Z"
        assertEquals("2026-09-01", formatTimelineDate(iso, null, DateFormat.ISO, Locale.US))
        assertEquals("09/01/2026", formatTimelineDate(iso, null, DateFormat.US, Locale.US))
        assertEquals("Sep 1, 2026", formatTimelineDate(iso, null, DateFormat.US_MMM, Locale.US))
    }

    @Test
    fun `an activity with a later end date renders a range`() {
        val start = "01.09.2026"
        val end = "03.09.2026"
        assertEquals(
            "$start – $end",
            formatTimelineDate(
                "2026-09-01T10:00:00Z",
                "2026-09-03T10:00:00Z",
                DateFormat.EU,
                Locale.US
            )
        )
        assertEquals(
            start,
            formatTimelineDate(
                "2026-09-01T10:00:00Z",
                "2026-09-01T18:00:00Z",
                DateFormat.EU,
                Locale.US
            )
        )
        assertEquals(
            start,
            formatTimelineDate("2026-09-01T10:00:00Z", "", DateFormat.EU, Locale.US)
        )
    }

    @Test
    fun `an unparseable date falls back to the raw string`() {
        assertEquals("soon", formatTimelineDate("soon", null, DateFormat.EU, Locale.US))
    }

    @Test
    fun `every palette token has a colour and an unset or unknown token has none`() {
        for (token in CircleColors.ALL) assertNotNull(token, circleColorOrNull(token))
        assertNull(circleColorOrNull(""))
        assertNull(circleColorOrNull("chartreuse"))
        assertEquals(Color(0xFFE53935), circleColorOrNull("red"))
    }

    @Test
    fun `the palette mirrors the backend token list in order`() {
        assertEquals(
            listOf("red", "orange", "yellow", "green", "teal", "blue", "purple", "pink", "grey"),
            CircleColors.ALL
        )
        assertEquals(500, CircleColors.MAX_DESCRIPTION)
    }
}
