package com.mycorrhizal.crm.model.util

import com.mycorrhizal.crm.model.network.PartialDate
import com.mycorrhizal.crm.model.util.DateFormat.display
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.util.TimeZone

class DateFormatTest {

    @Test
    fun `full date renders eu format`() {
        val date = PartialDate(year = 1990, month = 6, day = 15)
        assertEquals("15 June 1990", date.display("eu"))
    }

    @Test
    fun `full date renders us format`() {
        val date = PartialDate(year = 1990, month = 6, day = 15)
        assertEquals("June 15, 1990", date.display("us"))
    }

    @Test
    fun `full date renders iso format`() {
        val date = PartialDate(year = 1990, month = 6, day = 15)
        assertEquals("1990-06-15", date.display("iso"))
    }

    @Test
    fun `yearless month and day renders without year`() {
        val date = PartialDate(year = null, month = 12, day = 25)
        assertEquals("25 December", date.display("eu"))
    }

    @Test
    fun `year only renders just the year`() {
        val date = PartialDate(year = 2020, month = null, day = null)
        assertEquals("2020", date.display("eu"))
    }

    @Test
    fun `empty date renders empty string`() {
        val date = PartialDate(year = null, month = null, day = null)
        assertEquals("", date.display("eu"))
    }

    @Test
    fun `hasMonthDay is true only when both month and day present`() {
        assertTrue(PartialDate(month = 6, day = 15).hasMonthDay)
        assertFalse(PartialDate(year = 1990).hasMonthDay)
    }

    @Test
    fun `isYearOnly is true only when year present and month absent`() {
        assertTrue(PartialDate(year = 1990).isYearOnly)
        assertFalse(PartialDate(year = 1990, month = 6).isYearOnly)
    }

    // --- formatTimestamp (the shared UTC renderer for cadence/briefing dates) ---

    private val originalTimeZone = TimeZone.getDefault()

    @Before
    fun shiftToWesternTimeZone() {
        // Force a non-UTC device zone so the tests prove formatTimestamp
        // renders in UTC (web parity) rather than following the device zone,
        // which would shift 2026-09-10T01:00:00Z to the previous day here.
        TimeZone.setDefault(TimeZone.getTimeZone("America/Los_Angeles"))
    }

    @After
    fun restoreTimeZone() {
        TimeZone.setDefault(originalTimeZone)
    }

    @Test
    fun `formatTimestamp renders the UTC calendar day regardless of the device zone`() {
        // 2026-09-10T01:00:00Z is 2026-09-09 18:00 in Los Angeles — a
        // device-zone renderer would show "9 September 2026".
        assertEquals("10 September 2026", DateFormat.formatTimestamp("2026-09-10T01:00:00Z", "eu"))
    }

    @Test
    fun `formatTimestamp honors the user date format`() {
        assertEquals("2026-09-10", DateFormat.formatTimestamp("2026-09-10T01:00:00Z", "iso"))
        assertEquals("September 10, 2026", DateFormat.formatTimestamp("2026-09-10T01:00:00Z", "us"))
    }

    @Test
    fun `formatTimestamp renders an offset timestamp as its UTC calendar day`() {
        // +02:00 on the 10th is still the 9th in UTC — the instant, converted.
        assertEquals("9 September 2026", DateFormat.formatTimestamp("2026-09-10T01:00:00+02:00", "eu"))
    }

    @Test
    fun `formatTimestamp is empty for blank or unparseable input`() {
        assertEquals("", DateFormat.formatTimestamp(null, "eu"))
        assertEquals("", DateFormat.formatTimestamp("", "eu"))
        assertEquals("", DateFormat.formatTimestamp("not-a-date", "eu"))
    }
}
