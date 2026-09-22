package com.mycorrhizal.crm.model.util

import com.mycorrhizal.crm.model.network.PartialDate
import org.junit.Assert.assertEquals
import org.junit.Test

class PartialDateEditTest {

    @Test
    fun `a full date formats as year-month-day`() {
        assertEquals("1990-06-15", PartialDate(year = 1990, month = 6, day = 15).formatForEdit())
    }

    @Test
    fun `a yearless date formats with a leading double dash`() {
        assertEquals("--06-15", PartialDate(month = 6, day = 15).formatForEdit())
    }

    @Test
    fun `a year-only or empty partial formats blank`() {
        assertEquals("", PartialDate(year = 1990).formatForEdit())
        assertEquals("", PartialDate().formatForEdit())
    }

    @Test
    fun `a full date string parses back into year month day`() {
        val parsed = parsePartialDateForEdit("1990-06-15")
        assertEquals(1990, parsed.year)
        assertEquals(6, parsed.month)
        assertEquals(15, parsed.day)
    }

    @Test
    fun `a yearless date string parses with a null year`() {
        val parsed = parsePartialDateForEdit("--06-15")
        assertEquals(null, parsed.year)
        assertEquals(6, parsed.month)
        assertEquals(15, parsed.day)
    }

    @Test
    fun `format then parse round-trips exactly for a full date`() {
        val original = PartialDate(year = 1990, month = 6, day = 15)
        assertEquals(original, parsePartialDateForEdit(original.formatForEdit()))
    }

    @Test
    fun `format then parse round-trips exactly for a yearless date`() {
        val original = PartialDate(month = 12, day = 25)
        assertEquals(original, parsePartialDateForEdit(original.formatForEdit()))
    }
}
