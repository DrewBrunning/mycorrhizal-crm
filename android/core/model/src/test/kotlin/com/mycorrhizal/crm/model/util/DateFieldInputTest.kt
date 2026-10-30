package com.mycorrhizal.crm.model.util

import java.time.LocalDate
import org.junit.Assert.assertEquals
import org.junit.Test

/** Issue #2207: typed-date parsing for the Android DateField, in each format. */
class DateFieldInputTest {
    private val day = LocalDate.of(2026, 8, 10)

    private fun full(date: LocalDate) = TypedDate.Day(date, yearUnknown = false)

    @Test
    fun `a valid date parses in every format's digit order and separator`() {
        val cases = mapOf(
            "eu" to "10.08.2026",
            "eu-mmm" to "10.08.2026",
            "eu-mmmm" to "10.08.2026",
            "us" to "08/10/2026",
            "us-mmm" to "08/10/2026",
            "us-mmmm" to "08/10/2026",
            "iso" to "2026-08-10",
            "ca" to "10/08/2026",
            "eu-hyphen" to "10-08-2026"
        )
        cases.forEach { (format, text) ->
            assertEquals(format, full(day), DateFieldInput.parse(text, format))
        }
    }

    @Test
    fun `an unknown format token falls back to eu like the display`() {
        assertEquals(full(day), DateFieldInput.parse("10.08.2026", "bogus"))
    }

    @Test
    fun `a pasted ISO date is accepted in every format`() {
        DateFormat.ALL_FORMATS.forEach { format ->
            assertEquals(format, full(day), DateFieldInput.parse("2026-08-10", format))
        }
    }

    @Test
    fun `single digit day and month and surrounding space are accepted`() {
        assertEquals(full(LocalDate.of(2026, 1, 5)), DateFieldInput.parse(" 5.1.2026 ", "eu"))
    }

    @Test
    fun `a far-past date parses`() {
        assertEquals(
            full(LocalDate.of(1950, 3, 7)),
            DateFieldInput.parse("07.03.1950", "eu")
        )
    }

    @Test
    fun `a calendar day that does not exist is invalid`() {
        assertEquals(TypedDate.Invalid, DateFieldInput.parse("31.02.2026", "eu"))
        assertEquals(TypedDate.Invalid, DateFieldInput.parse("02/29/2025", "us"))
        assertEquals(full(LocalDate.of(2024, 2, 29)), DateFieldInput.parse("02/29/2024", "us"))
    }

    @Test
    fun `wrong order, partial, non-digit and two-digit-year text is invalid`() {
        listOf(
            "13/10/2026" to "us",
            "10.08.26" to "eu",
            "10.08." to "eu",
            "10.08" to "eu",
            "2026" to "eu",
            "abc" to "eu",
            "10.08.20260" to "eu",
            "10-08-2026" to "eu",
            "10.08.2026.5" to "eu",
            "00.01.2026" to "eu",
            "10.08.0000" to "eu"
        ).forEach { (text, format) ->
            assertEquals("$text/$format", TypedDate.Invalid, DateFieldInput.parse(text, format))
        }
    }

    @Test
    fun `blank text is empty`() {
        assertEquals(TypedDate.Empty, DateFieldInput.parse("", "eu"))
        assertEquals(TypedDate.Empty, DateFieldInput.parse("   ", "us"))
    }

    @Test
    fun `a date before the lower bound is out of range unless it is the loaded value`() {
        val floor = LocalDate.of(2026, 8, 10)
        assertEquals(
            TypedDate.OutOfRange,
            DateFieldInput.parse("09.08.2026", "eu", notBefore = floor)
        )
        assertEquals(
            full(floor),
            DateFieldInput.parse("10.08.2026", "eu", notBefore = floor)
        )
        assertEquals(
            full(LocalDate.of(2026, 8, 1)),
            DateFieldInput.parse(
                "01.08.2026",
                "eu",
                notBefore = floor,
                alwaysAllowed = LocalDate.of(2026, 8, 1)
            )
        )
    }

    @Test
    fun `a yearless entry is invalid unless year-less input is allowed`() {
        assertEquals(TypedDate.Invalid, DateFieldInput.parse("15.01.", "eu"))
        assertEquals(TypedDate.Invalid, DateFieldInput.parse("15.01", "eu"))
    }

    @Test
    fun `year-less entry is day and month in the format's order when allowed`() {
        val jan15 = TypedDate.Day(LocalDate.of(DateFieldValue.PARTIAL_PLACEHOLDER_YEAR, 1, 15), true)
        mapOf(
            "eu" to "15.01.",
            "eu-mmm" to "15.01",
            "us" to "01/15",
            "us-mmmm" to "01/15",
            "iso" to "01-15",
            "ca" to "15/01",
            "eu-hyphen" to "15-01"
        ).forEach { (format, text) ->
            assertEquals(format, jan15, DateFieldInput.parse(text, format, allowYearless = true))
        }
    }

    @Test
    fun `year-less 29 February is real and 30 February is not`() {
        assertEquals(
            TypedDate.Day(LocalDate.of(2000, 2, 29), true),
            DateFieldInput.parse("29.02.", "eu", allowYearless = true)
        )
        assertEquals(
            TypedDate.Invalid,
            DateFieldInput.parse("30.02.", "eu", allowYearless = true)
        )
    }

    @Test
    fun `a full date still parses when year-less input is allowed`() {
        assertEquals(
            full(LocalDate.of(1984, 6, 15)),
            DateFieldInput.parse("15.06.1984", "eu", allowYearless = true)
        )
    }

    @Test
    fun `editText is numeric in the format's order, even for month-name formats`() {
        assertEquals("10.08.2026", DateFieldInput.editText(day, false, "eu-mmmm"))
        assertEquals("08/10/2026", DateFieldInput.editText(day, false, "us-mmm"))
        assertEquals("2026-08-10", DateFieldInput.editText(day, false, "iso"))
        assertEquals("10/08/2026", DateFieldInput.editText(day, false, "ca"))
        assertEquals("10-08-2026", DateFieldInput.editText(day, false, "eu-hyphen"))
        assertEquals("07.03.1950", DateFieldInput.editText(LocalDate.of(1950, 3, 7), false, "eu"))
        assertEquals("15.01.", DateFieldInput.editText(LocalDate.of(2000, 1, 15), true, "eu"))
        assertEquals("01/15", DateFieldInput.editText(LocalDate.of(2000, 1, 15), true, "us-mmm"))
        assertEquals("01-15", DateFieldInput.editText(LocalDate.of(2000, 1, 15), true, "iso"))
    }

    @Test
    fun `editText output parses back to the same date`() {
        DateFormat.ALL_FORMATS.forEach { format ->
            val text = DateFieldInput.editText(day, false, format)
            assertEquals(format, full(day), DateFieldInput.parse(text, format))
            val yearless = DateFieldInput.editText(LocalDate.of(2000, 8, 10), true, format)
            assertEquals(
                format,
                TypedDate.Day(LocalDate.of(2000, 8, 10), true),
                DateFieldInput.parse(yearless, format, allowYearless = true)
            )
        }
    }

    @Test
    fun `pattern names the typed layout`() {
        assertEquals("DD.MM.YYYY", DateFieldInput.pattern("eu-mmm"))
        assertEquals("MM/DD/YYYY", DateFieldInput.pattern("us-mmmm"))
        assertEquals("YYYY-MM-DD", DateFieldInput.pattern("iso"))
        assertEquals("DD/MM/YYYY", DateFieldInput.pattern("ca"))
        assertEquals("DD-MM-YYYY", DateFieldInput.pattern("eu-hyphen"))
        assertEquals("DD.MM.YYYY or DD.MM.", DateFieldInput.pattern("eu", allowYearless = true))
        assertEquals("MM/DD/YYYY or MM/DD", DateFieldInput.pattern("us", allowYearless = true))
    }
}
