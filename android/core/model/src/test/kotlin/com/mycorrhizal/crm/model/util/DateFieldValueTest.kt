package com.mycorrhizal.crm.model.util

import com.mycorrhizal.crm.model.network.PartialDate
import java.time.LocalDate
import java.time.LocalTime
import java.time.ZoneId
import java.util.Locale
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Issue #1726's value-mapping contract: date-only -> `YYYY-MM-DD`, date+time ->
 * an RFC 3339 instant with the device offset, year-unknown -> `--MM-DD`, and
 * every reverse direction. Pure, so it does not need Compose.
 */
class DateFieldValueTest {

    private val berlin = ZoneId.of("Europe/Berlin") // +02:00 in August
    private val chicago = ZoneId.of("America/Chicago") // -05:00 in August
    private val english = Locale.ENGLISH
    private val date = LocalDate.of(2026, 8, 10)

    // --- date-only ---

    @Test
    fun `date-only writes and reads a bare calendar day`() {
        assertEquals("2026-08-10", DateFieldValue.pick(date, null, DateFieldMode.DATE_ONLY))

        val state = DateFieldValue.parse("2026-08-10", DateFieldMode.DATE_ONLY)
        assertEquals(date, state.date)
        assertNull(state.time)
        assertEquals(
            "2026-08-10",
            DateFieldValue.display("2026-08-10", DateFieldMode.DATE_ONLY, DateFormat.ISO, true)
        )
    }

    @Test
    fun `a calendar-day instant reads as its calendar date, not a shifted one`() {
        // The Activity/Note/reminder no-time shape.
        val state = DateFieldValue.parse("2026-08-10T00:00:00Z", DateFieldMode.DATE_TIME, chicago)
        assertEquals(date, state.date)
        assertNull(state.time)
    }

    // --- date + optional time ---

    @Test
    fun `date-time without a time writes the calendar-day instant`() {
        // Activity/Note DTOs are Go time.Time: a bare date cannot unmarshal, so
        // the no-time shape is the exact UTC midnight the API already stores.
        assertEquals(
            "2026-08-10T00:00:00Z",
            DateFieldValue.pick(date, null, DateFieldMode.DATE_TIME, zone = berlin)
        )
    }

    @Test
    fun `date-time with a time writes a full RFC 3339 instant with the device offset`() {
        assertEquals(
            "2026-08-10T14:30:00+02:00",
            DateFieldValue.pick(date, LocalTime.of(14, 30), DateFieldMode.DATE_TIME, zone = berlin)
        )

        val state = DateFieldValue.parse(
            "2026-08-10T14:30:00+02:00",
            DateFieldMode.DATE_TIME,
            berlin
        )
        assertEquals(date, state.date)
        assertEquals(LocalTime.of(14, 30), state.time)
    }

    @Test
    fun `an existing real instant is shown in the device zone with its time`() {
        // 14:00Z is 09:00 in Chicago in August. The picker must open on the
        // local date and the local time, matching what the user saw stored.
        val state = DateFieldValue.parse("2026-08-10T14:00:00Z", DateFieldMode.DATE_TIME, chicago)
        assertEquals(date, state.date)
        assertEquals(LocalTime.of(9, 0), state.time)
    }

    @Test
    fun `an instant at exactly UTC midnight has no time`() {
        val state = DateFieldValue.parse(
            "2026-08-10T00:00:00.000Z",
            DateFieldMode.DATE_TIME,
            berlin
        )
        assertEquals(date, state.date)
        assertNull(state.time)
    }

    @Test
    fun `date-time display includes the time only when a time is stored`() {
        assertEquals(
            "2026-08-10",
            DateFieldValue.display(
                "2026-08-10T00:00:00Z",
                DateFieldMode.DATE_TIME,
                DateFormat.ISO,
                true,
                berlin,
                english
            )
        )
        assertEquals(
            "2026-08-10 14:30",
            DateFieldValue.display(
                "2026-08-10T14:30:00+02:00",
                DateFieldMode.DATE_TIME,
                DateFormat.ISO,
                true,
                berlin,
                english
            )
        )
        assertEquals(
            "2026-08-10 2:30 PM",
            DateFieldValue.display(
                "2026-08-10T14:30:00+02:00",
                DateFieldMode.DATE_TIME,
                DateFormat.ISO,
                false,
                berlin,
                english
            )
        )
    }

    @Test
    fun `an opaque datetime field keeps a bare date when no time is picked`() {
        assertEquals(
            "2026-08-10",
            DateFieldValue.pick(date, null, DateFieldMode.DATE_TIME_OPTIONAL, zone = berlin)
        )
        assertEquals(
            "2026-08-10T09:05:00+02:00",
            DateFieldValue.pick(
                date,
                LocalTime.of(9, 5),
                DateFieldMode.DATE_TIME_OPTIONAL,
                zone = berlin
            )
        )
    }

    // --- reminders' UTC-midnight shape ---

    @Test
    fun `a reminder date writes UTC midnight and reads it back as a day`() {
        assertEquals(
            "2026-08-10T00:00:00Z",
            DateFieldValue.pick(date, null, DateFieldMode.DATE_UTC_MIDNIGHT)
        )
        val state = DateFieldValue.parse("2026-08-10T00:00:00Z", DateFieldMode.DATE_UTC_MIDNIGHT)
        assertEquals(date, state.date)
        assertNull(state.time)
    }

    @Test
    fun `date-only values are normalised to the UTC-midnight wire shape`() {
        // Go's time.Time cannot unmarshal a bare date, so every date-only path
        // that reaches Activity/Note/reminder storage is normalised here.
        assertEquals(
            "2026-08-10T00:00:00Z",
            DateFieldValue.normalizeCalendarDayInstant("2026-08-10")
        )
        assertEquals(
            "2026-08-10T14:00:00Z",
            DateFieldValue.normalizeCalendarDayInstant("2026-08-10T14:00:00Z")
        )
        assertNull(DateFieldValue.normalizeCalendarDayInstant("  "))
        assertNull(DateFieldValue.normalizeCalendarDayInstant(null))
    }

    // --- partial dates ---

    @Test
    fun `a full partial date round-trips`() {
        val state = DateFieldValue.parse("1990-06-15", DateFieldMode.PARTIAL)
        assertEquals(LocalDate.of(1990, 6, 15), state.date)
        assertEquals(false, state.yearUnknown)
        assertEquals("1990-06-15", DateFieldValue.pick(state.date!!, null, DateFieldMode.PARTIAL))
    }

    @Test
    fun `year-unknown drops the year and opens on the leap-year placeholder`() {
        // 29 Feb only exists in a leap year; the placeholder must keep it pickable.
        val state = DateFieldValue.parse("--02-29", DateFieldMode.PARTIAL)
        assertEquals(LocalDate.of(DateFieldValue.PARTIAL_PLACEHOLDER_YEAR, 2, 29), state.date)
        assertTrue(state.yearUnknown)
        assertEquals(
            "--02-29",
            DateFieldValue.pick(state.date!!, null, DateFieldMode.PARTIAL, yearUnknown = true)
        )
        assertEquals(
            "29.02.",
            DateFieldValue.display("--02-29", DateFieldMode.PARTIAL, DateFormat.EU, true)
        )
    }

    @Test
    fun `an unknown-year date stores only month and day`() {
        val picked = DateFieldValue.pick(date, null, DateFieldMode.PARTIAL, yearUnknown = true)
        assertEquals("--08-10", picked)
        // The year of the picker is deliberately discarded.
        assertEquals(
            "--08-10",
            DateFieldValue.parse(picked, DateFieldMode.PARTIAL).let {
                DateFieldValue.pick(
                    it.date!!,
                    null,
                    DateFieldMode.PARTIAL,
                    yearUnknown = it.yearUnknown
                )
            }
        )
    }

    @Test
    fun `a year-only legacy value is parsed and displayed as a year`() {
        val state = DateFieldValue.parse("2024", DateFieldMode.PARTIAL)
        assertEquals(2024, state.yearOnly)
        assertEquals(LocalDate.of(2024, 1, 1), state.date)
        assertEquals(
            "2024",
            DateFieldValue.display("2024", DateFieldMode.PARTIAL, DateFormat.EU, true)
        )
    }

    @Test
    fun `a year-month legacy value stays visible and round-trips untouched`() {
        val state = DateFieldValue.parse("2024-03", DateFieldMode.PARTIAL)
        assertEquals("2024-03", state.yearMonth)
        assertEquals(LocalDate.of(2024, 3, 1), state.date)
        assertEquals(
            "2024-03",
            DateFieldValue.display("2024-03", DateFieldMode.PARTIAL, DateFormat.EU, true)
        )
    }

    @Test
    fun `partialFieldValue covers full, yearless, year-month and year-only values`() {
        assertEquals("1990-06-15", DateFieldValue.partialFieldValue(PartialDate(1990, 6, 15)))
        assertEquals("--12-25", DateFieldValue.partialFieldValue(PartialDate(null, 12, 25)))
        assertEquals("2024-03", DateFieldValue.partialFieldValue(PartialDate(2024, 3, null)))
        assertEquals("2024", DateFieldValue.partialFieldValue(PartialDate(2024, null, null)))
        assertEquals("", DateFieldValue.partialFieldValue(null))
    }

    @Test
    fun `blank and unparseable values yield an empty state and display`() {
        assertEquals(DateFieldState(), DateFieldValue.parse("", DateFieldMode.DATE_TIME))
        assertEquals(DateFieldState(), DateFieldValue.parse("soon", DateFieldMode.DATE_TIME))
        assertEquals(
            "",
            DateFieldValue.display("soon", DateFieldMode.DATE_TIME, DateFormat.ISO, true)
        )
    }
}
