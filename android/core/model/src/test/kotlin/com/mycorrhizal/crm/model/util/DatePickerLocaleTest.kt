package com.mycorrhizal.crm.model.util

import java.time.DayOfWeek
import java.time.temporal.WeekFields
import java.util.Locale
import org.junit.Assert.assertEquals
import org.junit.Test

/** Issue #2654: the date picker's week start follows the device, else the user's date format. */
class DatePickerLocaleTest {
    private val enUs: Locale = Locale.forLanguageTag("en-US")

    private fun firstDay(device: Locale, dateFormat: String): DayOfWeek =
        WeekFields.of(DatePickerLocale.resolve(device, dateFormat)).firstDayOfWeek

    @Test
    fun `a day-first or ISO format starts the week on Monday even on a US-region device`() {
        for (format in listOf("eu", "eu-mmm", "eu-mmmm", "eu-hyphen", "ca", "iso")) {
            assertEquals(format, DayOfWeek.MONDAY, firstDay(enUs, format))
        }
    }

    @Test
    fun `a month-first format starts the week on Sunday even on a Monday-region device`() {
        for (format in listOf("us", "us-mmm", "us-mmmm")) {
            assertEquals(format, DayOfWeek.SUNDAY, firstDay(Locale.GERMANY, format))
        }
    }

    @Test
    fun `an explicit first-day setting on the device wins over the date format`() {
        val deviceSunday = Locale.forLanguageTag("de-DE-u-fw-sun")
        val deviceMonday = Locale.forLanguageTag("en-US-u-fw-mon")
        assertEquals(DayOfWeek.SUNDAY, firstDay(deviceSunday, "eu"))
        assertEquals(DayOfWeek.MONDAY, firstDay(deviceMonday, "us"))
    }

    @Test
    fun `the device language is kept so weekday and month names stay in the app language`() {
        assertEquals("en", DatePickerLocale.resolve(enUs, "eu").language)
        assertEquals("fr", DatePickerLocale.resolve(Locale.FRANCE, "us").language)
    }

    @Test
    fun `an unknown format falls back to the day-first convention`() {
        assertEquals(DayOfWeek.MONDAY, firstDay(enUs, "not-a-format"))
    }
}
