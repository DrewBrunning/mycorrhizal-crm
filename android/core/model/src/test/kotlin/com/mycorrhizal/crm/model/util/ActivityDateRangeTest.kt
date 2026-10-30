package com.mycorrhizal.crm.model.util

import java.time.LocalDate
import java.time.ZoneId
import java.util.Locale
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** Issue #2006: the activity date-range display, mirroring web's `formatActivityDateRange`. */
class ActivityDateRangeTest {

    private val start = "2026-05-12T00:00:00Z"
    private val end = "2026-05-15T00:00:00Z"
    private val la = ZoneId.of("America/Los_Angeles")

    private fun fmt(s: String?, e: String?, format: String, zone: ZoneId = la) =
        ActivityDateRange.format(s, e, format, zone, Locale.ENGLISH)

    @Test
    fun `no end, equal end, reversed end or unparseable end shows the start day only`() {
        assertEquals("12.05.2026", fmt(start, null, DateFormat.EU))
        assertEquals("12.05.2026", fmt(start, "", DateFormat.EU))
        assertEquals("12.05.2026", fmt(start, start, DateFormat.EU))
        assertEquals("12.05.2026", fmt(start, "2026-05-01T00:00:00Z", DateFormat.EU))
        assertEquals("12.05.2026", fmt(start, "garbage", DateFormat.EU))
    }

    @Test
    fun `numeric formats show both ends in full`() {
        assertEquals("12.05.2026 – 15.05.2026", fmt(start, end, DateFormat.EU))
        assertEquals("05/12/2026 – 05/15/2026", fmt(start, end, DateFormat.US))
        assertEquals("2026-05-12 – 2026-05-15", fmt(start, end, DateFormat.ISO))
        assertEquals("12/05/2026 – 15/05/2026", fmt(start, end, DateFormat.CA))
        assertEquals("12-05-2026 – 15-05-2026", fmt(start, end, DateFormat.EU_HYPHEN))
    }

    @Test
    fun `month-name formats collapse a range inside one month`() {
        assertEquals("12–15 May, 2026", fmt(start, end, DateFormat.EU_MMM))
        assertEquals("12–15 May, 2026", fmt(start, end, DateFormat.EU_MMMM))
        assertEquals("02–15 May, 2026", fmt("2026-05-02T00:00:00Z", end, DateFormat.EU_MMMM))
        assertEquals("May 12–15, 2026", fmt(start, end, DateFormat.US_MMM))
        assertEquals("May 12–15, 2026", fmt(start, end, DateFormat.US_MMMM))
    }

    @Test
    fun `month-name formats show both ends across a month or year boundary`() {
        assertEquals(
            "12 May, 2026 – 2 Jun, 2026",
            fmt(start, "2026-06-02T00:00:00Z", DateFormat.EU_MMM)
        )
        assertEquals(
            "December 30, 2026 – January 2, 2027",
            fmt("2026-12-30T00:00:00Z", "2027-01-02T00:00:00Z", DateFormat.US_MMMM)
        )
    }

    @Test
    fun `a calendar day is never shifted by the device zone`() {
        val west = ZoneId.of("America/Los_Angeles")
        val east = ZoneId.of("Pacific/Auckland")
        for (zone in listOf(west, east)) {
            assertEquals(
                "2026-01-01 – 2026-01-03",
                fmt("2026-01-01T00:00:00Z", "2026-01-03T00:00:00Z", DateFormat.ISO, zone)
            )
        }
    }

    @Test
    fun `a start with a time of day reads in the device zone like a single-day row`() {
        // 2026-05-12T01:00:00Z is still May 11 in Los Angeles.
        assertEquals(
            "2026-05-11 – 2026-05-15",
            fmt("2026-05-12T01:00:00Z", end, DateFormat.ISO, la)
        )
    }

    @Test
    fun `rangeOrNull is null for a single day and the range otherwise`() {
        assertNull(ActivityDateRange.rangeOrNull(start, null, DateFormat.ISO, la))
        assertNull(ActivityDateRange.rangeOrNull(start, start, DateFormat.ISO, la, Locale.ENGLISH))
        assertEquals(
            "2026-05-12 – 2026-05-15",
            ActivityDateRange.rangeOrNull(start, end, DateFormat.ISO, la, Locale.ENGLISH)
        )
    }

    @Test
    fun `endBeforeStart is true only for a strictly earlier end day`() {
        assertTrue(ActivityDateRange.endBeforeStart(start, "2026-05-11T00:00:00Z", la))
        assertFalse(ActivityDateRange.endBeforeStart(start, start, la))
        assertFalse(ActivityDateRange.endBeforeStart(start, end, la))
        assertFalse(ActivityDateRange.endBeforeStart(start, null, la))
        assertFalse(ActivityDateRange.endBeforeStart(start, "", la))
        assertFalse(ActivityDateRange.endBeforeStart("", end, la))
    }

    @Test
    fun `startDay reads a calendar day as is and an instant in the zone`() {
        assertEquals(LocalDate.of(2026, 5, 12), ActivityDateRange.startDay(start, la))
        assertEquals(
            LocalDate.of(2026, 5, 11),
            ActivityDateRange.startDay("2026-05-12T01:00:00Z", la)
        )
        assertNull(ActivityDateRange.startDay("", la))
        assertNull(ActivityDateRange.startDay(null, la))
    }
}
