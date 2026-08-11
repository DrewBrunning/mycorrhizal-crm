package com.mycorrhizal.crm.model.util

import com.mycorrhizal.crm.model.network.PartialDate
import com.mycorrhizal.crm.model.util.DateFormat.display
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

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
}
