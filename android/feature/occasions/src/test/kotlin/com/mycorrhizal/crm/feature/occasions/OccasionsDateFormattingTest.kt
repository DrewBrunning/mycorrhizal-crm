package com.mycorrhizal.crm.feature.occasions

import com.mycorrhizal.crm.model.network.OccasionEvent
import java.time.Instant
import java.time.ZoneId
import java.time.ZoneOffset
import java.util.Locale
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** Issue #1753: occasion times render in local time, with web's (#1789) range collapsing. */
class OccasionsDateFormattingTest {

    private val chicago = ZoneId.of("America/Chicago")
    private val auckland = ZoneId.of("Pacific/Auckland")
    private val utc = ZoneId.of("UTC")
    private val locale = Locale.GERMANY // 24-hour, so assertions don't depend on the JDK's AM/PM spacing

    private fun event(start: String, end: String? = null) =
        OccasionEvent(id = "e1", title = "BBQ", startsAt = start, endsAt = end)

    private fun `when`(
        event: OccasionEvent,
        format: String = "eu",
        zone: ZoneId = chicago,
        is24Hour: Boolean = true
    ) = formatEventWhen(event, format, is24Hour, zone, locale)

    @Test
    fun `a start is shown in the device zone, not UTC`() {
        // The audit's case: 18:06Z is 13:06 in Chicago (CDT).
        assertEquals("26.10.2026 13:06", `when`(event("2026-10-26T18:06:00Z"), zone = chicago))
        assertEquals("26.10.2026 18:06", `when`(event("2026-10-26T18:06:00Z"), zone = utc))
        // Another zone converts again, and can move the date.
        assertEquals("27.10.2026 07:06", `when`(event("2026-10-26T18:06:00Z"), zone = auckland))
    }

    @Test
    fun `times follow the 12-24 hour flag in an en-US or de locale`() {
        val e = event("2026-10-26T19:30:00Z", "2026-10-26T21:00:00Z") // 14:30 - 16:00 CDT
        val us = { h: Boolean ->
            formatEventWhen(
                e,
                "eu",
                h,
                chicago,
                Locale.US
            ).replace('\u202f', ' ')
        }
        assertEquals("26.10.2026 14:30 – 16:00", us(true))
        assertEquals("26.10.2026 2:30 PM – 4:00 PM", us(false))
        val de = formatEventWhen(e, "eu", false, chicago, Locale.GERMANY).replace('\u202f', ' ')
        assertEquals("26.10.2026 2:30 PM – 4:00 PM", de)
    }

    @Test
    fun `the date part follows the date format preference`() {
        val e = event("2026-10-26T18:06:00Z")
        assertEquals("2026-10-26 13:06", `when`(e, "iso"))
        assertEquals("10/26/2026 13:06", `when`(e, "us"))
        assertEquals("26 Oktober, 2026 13:06", `when`(e, "eu-mmmm"))
    }

    @Test
    fun `no end shows only the start`() {
        assertEquals("26.10.2026 13:06", `when`(event("2026-10-26T18:06:00Z", null)))
        assertEquals("26.10.2026 13:06", `when`(event("2026-10-26T18:06:00Z", "")))
    }

    @Test
    fun `a zero-length event shows no range even with a different spelling of the same instant`() {
        assertEquals(
            "26.10.2026 13:06",
            `when`(event("2026-10-26T18:06:00Z", "2026-10-26T18:06:00Z"))
        )
        assertEquals(
            "26.10.2026 13:06",
            `when`(event("2026-10-26T18:06:00Z", "2026-10-26T18:06:00.000Z"))
        )
    }

    @Test
    fun `a same-day event shows date start to end time`() {
        assertEquals(
            "26.10.2026 13:06 – 15:00",
            `when`(event("2026-10-26T18:06:00Z", "2026-10-26T20:00:00Z"))
        )
    }

    @Test
    fun `sameness is judged on the local day, not the UTC day`() {
        // 23:30Z and 01:30Z the next UTC day are both the 26th in Chicago.
        assertEquals(
            "26.10.2026 18:30 – 20:30",
            `when`(event("2026-10-26T23:30:00Z", "2026-10-27T01:30:00Z"))
        )
        // The same two instants straddle midnight in UTC: two days.
        assertEquals(
            "26.10.2026 23:30 – 27.10.2026 01:30",
            `when`(event("2026-10-26T23:30:00Z", "2026-10-27T01:30:00Z"), zone = utc)
        )
    }

    @Test
    fun `a multi-day event shows the full start and end`() {
        assertEquals(
            "26.10.2026 13:06 – 28.10.2026 12:00",
            `when`(event("2026-10-26T18:06:00Z", "2026-10-28T17:00:00Z"))
        )
    }

    @Test
    fun `an unparseable value is shown as-is instead of throwing`() {
        assertEquals("soon", instantToDisplay("soon", "eu", true, chicago, locale))
        assertEquals("", instantToDisplay("", "eu", true, chicago, locale))
    }

    // --- picker round trip (#1753: confirm create and display agree on the zone) ---

    @Test
    fun `an event picked at 18_00 local is stored as UTC and shows 18_00 locally`() {
        // The picker hands over the chosen date as UTC-midnight millis + wall time.
        val dateMillis = Instant.parse("2026-10-26T00:00:00Z").toEpochMilli()
        val stored = toRfc3339(dateMillis, 18, 0, chicago)
        assertEquals("2026-10-26T23:00:00Z", stored) // CDT is UTC-5
        assertEquals("26.10.2026 18:00", instantToDisplay(stored, "eu", true, chicago, locale))
        // On a device in another zone the same stored instant shows the converted time.
        assertEquals("26.10.2026 23:00", instantToDisplay(stored, "eu", true, utc, locale))
        assertEquals("27.10.2026 12:00", instantToDisplay(stored, "eu", true, auckland, locale))
    }

    @Test
    fun `the picker initial selection is the local date and wall time`() {
        // 02:30Z on the 27th is still the evening of the 26th in Chicago.
        val initial = pickerInitial(Instant.parse("2026-10-27T02:30:00Z"), chicago)
        assertEquals(Instant.parse("2026-10-26T00:00:00Z").toEpochMilli(), initial.dateMillis)
        assertEquals(21, initial.hour)
        assertEquals(30, initial.minute)
    }

    @Test
    fun `opening and confirming the picker unchanged round-trips the instant`() {
        for (zone in listOf(chicago, auckland, utc, ZoneOffset.ofHoursMinutes(5, 30))) {
            val original = Instant.parse("2026-10-27T02:30:00Z")
            val initial = pickerInitial(original, zone)
            assertEquals(
                zone.toString(),
                original.toString(),
                toRfc3339(initial.dateMillis, initial.hour, initial.minute, zone)
            )
        }
    }

    @Test
    fun `end-before-start compares instants, not strings`() {
        assertTrue(isEndBeforeStart("2026-10-26T18:00:00Z", "2026-10-26T17:00:00Z"))
        assertFalse(isEndBeforeStart("2026-10-26T18:00:00Z", "2026-10-26T18:00:00Z"))
        assertFalse(isEndBeforeStart("2026-10-26T18:00:00Z", ""))
        // Same instant, different spelling: not before.
        assertFalse(isEndBeforeStart("2026-10-26T18:00:00Z", "2026-10-26T18:00:00.000Z"))
        assertFalse(isEndBeforeStart("garbage", "2026-10-26T18:00:00Z"))
    }
}
