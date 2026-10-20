package com.mycorrhizal.crm.model.util

import com.mycorrhizal.crm.model.network.PartialDate
import com.mycorrhizal.crm.model.util.DateFormat.display
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.time.ZoneId
import java.util.Locale
import java.util.TimeZone

class DateFormatTest {

    private val originalTimeZone = TimeZone.getDefault()
    private val originalLocale = Locale.getDefault()

    private val chicago = ZoneId.of("America/Chicago")
    private val auckland = ZoneId.of("Pacific/Auckland")
    private val utc = ZoneId.of("UTC")

    @Before
    fun pinZoneAndLocale() {
        // A non-UTC device zone, so a renderer that follows the zone when it
        // should not (calendar days) is caught. Locale pinned to English so the
        // default-locale overloads are deterministic.
        TimeZone.setDefault(TimeZone.getTimeZone("America/Los_Angeles"))
        Locale.setDefault(Locale.ENGLISH)
    }

    @After
    fun restore() {
        TimeZone.setDefault(originalTimeZone)
        Locale.setDefault(originalLocale)
    }

    // --- formatFull: every token, the same strings as web's DateFormatProvider.test.ts ---

    @Test
    fun `every token renders like web for 2026-06-11`() {
        val expected = mapOf(
            "eu" to "11.06.2026",
            "us" to "06/11/2026",
            "iso" to "2026-06-11",
            "ca" to "11/06/2026",
            "eu-hyphen" to "11-06-2026",
            "us-mmm" to "Jun 11, 2026",
            "us-mmmm" to "June 11, 2026",
            "eu-mmm" to "11 Jun, 2026",
            "eu-mmmm" to "11 June, 2026",
        )
        assertEquals(DateFormat.ALL_FORMATS.toSet(), expected.keys)
        expected.forEach { (token, text) ->
            assertEquals(token, text, DateFormat.formatFull(2026, 6, 11, token, Locale.ENGLISH))
        }
    }

    @Test
    fun `every token renders distinctly`() {
        val rendered = DateFormat.ALL_FORMATS.map { DateFormat.formatFull(2026, 6, 11, it, Locale.ENGLISH) }
        assertEquals(rendered.size, rendered.toSet().size)
    }

    @Test
    fun `single digit day shows web's padding quirk between eu-mmm and eu-mmmm`() {
        // Web pads the day for eu-mmmm (DD) but not eu-mmm (D). Mirrored on purpose.
        assertEquals("5 Jun, 2026", DateFormat.formatFull(2026, 6, 5, "eu-mmm", Locale.ENGLISH))
        assertEquals("05 June, 2026", DateFormat.formatFull(2026, 6, 5, "eu-mmmm", Locale.ENGLISH))
        assertEquals("June 5, 2026", DateFormat.formatFull(2026, 6, 5, "us-mmmm", Locale.ENGLISH))
    }

    @Test
    fun `an unknown token falls back to eu like web`() {
        assertEquals("11.06.2026", DateFormat.formatFull(2026, 6, 11, "bogus", Locale.ENGLISH))
    }

    @Test
    fun `month names follow the locale`() {
        assertEquals("11 Juni, 2026", DateFormat.formatFull(2026, 6, 11, "eu-mmmm", Locale.GERMAN))
        assertEquals("11 juin, 2026", DateFormat.formatFull(2026, 6, 11, "eu-mmmm", Locale.FRENCH))
        assertEquals("11 giugno, 2026", DateFormat.formatFull(2026, 6, 11, "eu-mmmm", Locale.ITALIAN))
        assertEquals("11 junio, 2026", DateFormat.formatFull(2026, 6, 11, "eu-mmmm", Locale.forLanguageTag("es")))
        assertEquals("März 3, 2026", DateFormat.formatFull(2026, 3, 3, "us-mmmm", Locale.GERMAN))
        // The default-locale overload follows Locale.getDefault().
        Locale.setDefault(Locale.GERMAN)
        assertEquals("11 Juni, 2026", DateFormat.formatFull(2026, 6, 11, "eu-mmmm"))
        assertEquals("Juni", DateFormat.monthName(6))
    }

    @Test
    fun `numeric formats are locale independent`() {
        for (locale in listOf(Locale.GERMAN, Locale.FRENCH, Locale.ITALIAN, Locale.forLanguageTag("es"))) {
            assertEquals("11.06.2026", DateFormat.formatFull(2026, 6, 11, "eu", locale))
            assertEquals("06/11/2026", DateFormat.formatFull(2026, 6, 11, "us", locale))
        }
    }

    // --- PartialDate.display ---

    @Test
    fun `full date renders every token`() {
        val date = PartialDate(year = 1990, month = 6, day = 15)
        assertEquals("15.06.1990", date.display("eu"))
        assertEquals("06/15/1990", date.display("us"))
        assertEquals("1990-06-15", date.display("iso"))
        assertEquals("15 June, 1990", date.display("eu-mmmm"))
    }

    @Test
    fun `yearless month and day matches web formatYearlessDate`() {
        val date = PartialDate(year = null, month = 4, day = 30)
        val expected = mapOf(
            "eu" to "30.04.",
            "us" to "04/30",
            "iso" to "04-30",
            "ca" to "30/04",
            "eu-hyphen" to "30-04",
            "us-mmm" to "Apr 30",
            "us-mmmm" to "April 30",
            "eu-mmm" to "30 Apr",
            "eu-mmmm" to "30 April",
        )
        expected.forEach { (token, text) -> assertEquals(token, text, date.display(token)) }
    }

    @Test
    fun `year only renders just the year`() {
        assertEquals("2020", PartialDate(year = 2020, month = null, day = null).display("eu"))
    }

    @Test
    fun `empty date renders empty string`() {
        assertEquals("", PartialDate(year = null, month = null, day = null).display("eu"))
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

    // --- formatTimestamp: calendar days keep their date, real instants go local ---

    @Test
    fun `a midnight UTC calendar day keeps its UTC date in every zone`() {
        // activity/note date, reminder remind_at: stored 00:00Z. West of UTC the
        // instant is the previous evening; the day must still read the 21st.
        for (zone in listOf(chicago, auckland, utc)) {
            assertEquals(zone.id, "21.09.2026", DateFormat.formatTimestamp("2026-09-21T00:00:00Z", "eu", zone))
            assertEquals(zone.id, "2026-09-21", DateFormat.formatTimestamp("2026-09-21T00:00:00.000Z", "iso", zone))
            assertEquals(zone.id, "21.09.2026", DateFormat.formatTimestamp("2026-09-21T00:00:00+00:00", "eu", zone))
        }
    }

    @Test
    fun `a date-only value is never zone shifted`() {
        for (zone in listOf(chicago, auckland, utc)) {
            assertEquals(zone.id, "21.09.2026", DateFormat.formatTimestamp("2026-09-21", "eu", zone))
        }
    }

    @Test
    fun `a real instant renders its local calendar date`() {
        // 2026-09-10T01:00Z is the 9th, 20:00 in Chicago; the 10th in UTC; the 10th 13:00 in Auckland.
        assertEquals("09.09.2026", DateFormat.formatTimestamp("2026-09-10T01:00:00Z", "eu", chicago))
        assertEquals("10.09.2026", DateFormat.formatTimestamp("2026-09-10T01:00:00Z", "eu", utc))
        assertEquals("10.09.2026", DateFormat.formatTimestamp("2026-09-10T01:00:00Z", "eu", auckland))
        // 2026-09-10T14:00Z is already the 11th in Auckland (NZST/NZDT is ahead).
        assertEquals("11.09.2026", DateFormat.formatTimestamp("2026-09-10T14:00:00Z", "eu", auckland))
    }

    @Test
    fun `a midnight in a non-zero offset is a real instant`() {
        // Only an exact zero-offset midnight is a calendar day (web parity):
        // 02:00+02:00 on the 10th is 00:00Z but was not stored as a picked day.
        assertEquals("09.09.2026", DateFormat.formatTimestamp("2026-09-10T02:00:00+02:00", "eu", chicago))
    }

    @Test
    fun `the default zone is the device zone`() {
        // setUp pins Los_Angeles: 2026-09-10T01:00Z is the 9th there.
        assertEquals("09.09.2026", DateFormat.formatTimestamp("2026-09-10T01:00:00Z", "eu"))
        assertEquals("10.09.2026", DateFormat.formatTimestamp("2026-09-10T00:00:00Z", "eu"))
    }

    @Test
    fun `formatTimestamp honors every date format and locale`() {
        assertEquals("2026-09-10", DateFormat.formatTimestamp("2026-09-10T00:00:00Z", "iso", utc))
        assertEquals("09/10/2026", DateFormat.formatTimestamp("2026-09-10T00:00:00Z", "us", utc))
        assertEquals(
            "10 September, 2026",
            DateFormat.formatTimestamp("2026-09-10T00:00:00Z", "eu-mmmm", utc, Locale.ENGLISH),
        )
        assertEquals(
            "10 septembre, 2026",
            DateFormat.formatTimestamp("2026-09-10T00:00:00Z", "eu-mmmm", utc, Locale.FRENCH),
        )
    }

    @Test
    fun `formatTimestamp is empty for blank or unparseable input`() {
        assertEquals("", DateFormat.formatTimestamp(null, "eu"))
        assertEquals("", DateFormat.formatTimestamp("", "eu"))
        assertEquals("", DateFormat.formatTimestamp("not-a-date", "eu"))
    }

    @Test
    fun `calendarDay recognises only date-only and exact midnight UTC`() {
        assertEquals(Triple(2026, 9, 21), DateFormat.calendarDay("2026-09-21"))
        assertEquals(Triple(2026, 9, 21), DateFormat.calendarDay("2026-09-21T00:00:00Z"))
        assertEquals(null, DateFormat.calendarDay("2026-09-21T00:00:01Z"))
        assertEquals(null, DateFormat.calendarDay("2026-09-21T13:00:00Z"))
        assertEquals(null, DateFormat.calendarDay("garbage"))
    }

    // --- formatTime ---

    @Test
    fun `formatTime follows the 24-hour flag, not the locale default`() {
        val noon = "2026-10-26T19:30:00Z" // 14:30 in Chicago (CDT)
        // en-US with the device on 24-hour shows 14:30 even though en-US defaults to 12-hour.
        assertEquals("14:30", DateFormat.formatTime(noon, true, chicago, Locale.US))
        // en-US with 12-hour (newer JDKs put a narrow no-break space before PM).
        assertEquals("2:30 PM", DateFormat.formatTime(noon, false, chicago, Locale.US).replace('\u202f', ' '))
        // de defaults to 24-hour, but a 12-hour preference wins.
        assertEquals("2:30 PM", DateFormat.formatTime(noon, false, chicago, Locale.GERMANY).replace('\u202f', ' '))
        assertEquals("14:30", DateFormat.formatTime(noon, true, chicago, Locale.GERMANY))
        assertEquals("", DateFormat.formatTime(null, true))
        assertEquals("", DateFormat.formatTime("nope", false))
    }

    @Test
    fun `TimeFormatPreference auto defers to the device, overrides win`() {
        assertEquals(true, TimeFormatPreference.AUTO.is24Hour(true))
        assertEquals(false, TimeFormatPreference.AUTO.is24Hour(false))
        assertEquals(false, TimeFormatPreference.H12.is24Hour(true))
        assertEquals(true, TimeFormatPreference.H24.is24Hour(false))
        assertEquals(TimeFormatPreference.H12, TimeFormatPreference.fromToken("12h"))
        assertEquals(TimeFormatPreference.H24, TimeFormatPreference.fromToken("24h"))
        assertEquals(TimeFormatPreference.AUTO, TimeFormatPreference.fromToken("auto"))
        assertEquals(TimeFormatPreference.AUTO, TimeFormatPreference.fromToken(null))
    }
}
