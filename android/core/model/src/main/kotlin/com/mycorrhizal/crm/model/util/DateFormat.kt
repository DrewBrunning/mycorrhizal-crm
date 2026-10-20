package com.mycorrhizal.crm.model.util

import com.mycorrhizal.crm.model.network.PartialDate
import java.time.Instant
import java.time.Month
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle
import java.time.format.TextStyle
import java.util.Locale

/**
 * Display formatting for dates following the backend's `date_format`
 * preference. The token set and the numeric/long-form outputs are a port of the
 * web's `DateFormatProvider.tsx` `formatFullDate` / `formatYearlessDate`
 * (issue #1752): **the token list below and web's `DateFormat` union and
 * `SUPPORTED_DATE_FORMATS` must be edited together**, as must the backend
 * `oneof` validator they mirror (frontend CLAUDE.md trap #4).
 *
 * Month names (`us-mmm`, `us-mmmm`, `eu-mmm`, `eu-mmmm`) follow the app/device
 * [Locale] — a deliberate difference from web, whose month tables are English
 * only. For an English locale the output is byte-identical to web's.
 */
object DateFormat {
    const val EU = "eu" // DD.MM.YYYY
    const val US = "us" // MM/DD/YYYY
    const val ISO = "iso" // YYYY-MM-DD
    const val CA = "ca" // DD/MM/YYYY
    const val EU_HYPHEN = "eu-hyphen" // DD-MM-YYYY
    const val US_MMM = "us-mmm" // MMM D, YYYY
    const val US_MMMM = "us-mmmm" // MMMM D, YYYY
    const val EU_MMM = "eu-mmm" // D MMM, YYYY
    const val EU_MMMM = "eu-mmmm" // DD MMMM, YYYY

    /** Every supported token, in the web's declaration order. */
    val ALL_FORMATS: List<String> = listOf(EU, US, ISO, CA, EU_HYPHEN, US_MMM, US_MMMM, EU_MMM, EU_MMMM)

    /**
     * Format a partial date for display:
     *  - full date: per [formatFull]
     *  - yearless month+day: per [formatYearless] (web parity)
     *  - year only: "2020"
     *  - unknown: ""
     */
    fun PartialDate.display(format: String, locale: Locale = Locale.getDefault()): String = when {
        year != null && hasMonthDay -> formatFull(year!!, month!!, day!!, format, locale)
        hasMonthDay -> formatYearless(month!!, day!!, format, locale)
        isYearOnly -> "$year"
        else -> ""
    }

    /**
     * Port of web's `formatFullDate`. An unknown token renders as `eu`, web's
     * stored-format fallback. Note `eu-mmmm` zero-pads the day while `eu-mmm`
     * does not: web's own (odd) outputs, kept so the platforms agree.
     */
    fun formatFull(
        year: Int,
        month: Int,
        day: Int,
        format: String,
        locale: Locale = Locale.getDefault(),
    ): String {
        val y = "%04d".format(year)
        val mm = "%02d".format(month)
        val dd = "%02d".format(day)
        return when (format) {
            US -> "$mm/$dd/$y"
            ISO -> "$y-$mm-$dd"
            CA -> "$dd/$mm/$y"
            EU_HYPHEN -> "$dd-$mm-$y"
            US_MMM -> "${monthNameShort(month, locale)} $day, $y"
            US_MMMM -> "${monthName(month, locale)} $day, $y"
            EU_MMM -> "$day ${monthNameShort(month, locale)}, $y"
            EU_MMMM -> "$dd ${monthName(month, locale)}, $y"
            else -> "$dd.$mm.$y"
        }
    }

    /** Port of web's `formatYearlessDate` (the eu family keeps its trailing dot). */
    fun formatYearless(
        month: Int,
        day: Int,
        format: String,
        locale: Locale = Locale.getDefault(),
    ): String {
        val mm = "%02d".format(month)
        val dd = "%02d".format(day)
        return when (format) {
            US -> "$mm/$dd"
            ISO -> "$mm-$dd"
            CA -> "$dd/$mm"
            EU_HYPHEN -> "$dd-$mm"
            US_MMM -> "${monthNameShort(month, locale)} $day"
            US_MMMM -> "${monthName(month, locale)} $day"
            EU_MMM -> "$day ${monthNameShort(month, locale)}"
            EU_MMMM -> "$dd ${monthName(month, locale)}"
            else -> "$dd.$mm."
        }
    }

    fun monthName(month: Int, locale: Locale = Locale.getDefault()): String =
        Month.of(month).getDisplayName(TextStyle.FULL, locale)

    fun monthNameShort(month: Int, locale: Locale = Locale.getDefault()): String =
        Month.of(month).getDisplayName(TextStyle.SHORT, locale)

    // The suffixes (after the 10-char date) of an RFC 3339 instant at exactly
    // UTC midnight. Mirrors web's UTC_MIDNIGHT_SUFFIXES in utils/localDate.ts;
    // keep the two lists identical.
    private val UTC_MIDNIGHT_SUFFIXES = setOf(
        "T00:00:00Z",
        "T00:00:00.000Z",
        "T00:00:00+00:00",
        "T00:00:00.000+00:00",
    )
    private val DATE_ONLY = Regex("""\d{4}-\d{2}-\d{2}""")

    /**
     * The (year, month, day) of [value] when it is a *calendar day*: a
     * date-only `YYYY-MM-DD`, or an exact-UTC-midnight instant (how the API and
     * the clients store a picked day, e.g. activity/note `date`, reminder
     * `remind_at`). Null for a real instant. Port of web's `calendarDateOnly`
     * (ADR 0015 rule 2: a date-only value is never zone-shifted).
     */
    fun calendarDay(value: String): Triple<Int, Int, Int>? {
        val head = value.take(10)
        if (!DATE_ONLY.matches(head)) return null
        val rest = value.drop(10)
        if (rest.isNotEmpty() && rest !in UTC_MIDNIGHT_SUFFIXES) return null
        return Triple(head.substring(0, 4).toInt(), head.substring(5, 7).toInt(), head.substring(8, 10).toInt())
    }

    /**
     * Formats a server date value as a date in [format], or "" when
     * unparseable/absent. The single shared implementation for server-derived
     * dates (cadence next-due/last-interaction, briefing dates, activity/note/
     * reminder days), so every screen renders the same value the same way.
     *
     * Two kinds of value, matching web's `formatDateWithFormat` +
     * `calendarDateOnly`:
     *  - **Calendar days** (date-only, or exactly `T00:00:00Z`) render their
     *    own calendar date, in every device zone. Shifting them to the device
     *    zone shows a day logged "today" as yesterday west of UTC.
     *  - **Real instants** render their calendar date in [zone] (the device
     *    zone by default), like web's local-zone rendering (#962 / #1050).
     */
    fun formatTimestamp(
        iso: String?,
        format: String,
        zone: ZoneId = ZoneId.systemDefault(),
        locale: Locale = Locale.getDefault(),
    ): String {
        if (iso.isNullOrBlank()) return ""
        calendarDay(iso)?.let { (y, m, d) -> return formatFull(y, m, d, format, locale) }
        return runCatching {
            val local = Instant.parse(iso).atZone(zone)
            formatFull(local.year, local.monthValue, local.dayOfMonth, format, locale)
        }.getOrDefault("")
    }

    /** The time of day of a real instant in [zone], locale-short (no seconds). */
    fun formatTime(
        iso: String?,
        zone: ZoneId = ZoneId.systemDefault(),
        locale: Locale = Locale.getDefault(),
    ): String {
        if (iso.isNullOrBlank()) return ""
        return runCatching {
            DateTimeFormatter.ofLocalizedTime(FormatStyle.SHORT).withLocale(locale)
                .format(Instant.parse(iso).atZone(zone))
        }.getOrDefault("")
    }
}
