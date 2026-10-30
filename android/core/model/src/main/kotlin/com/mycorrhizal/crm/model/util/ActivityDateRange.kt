package com.mycorrhizal.crm.model.util

import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import java.time.ZoneOffset
import java.util.Locale

/**
 * Issue #2006: pure date logic for an activity's optional multi-day `end_date`, a port of web's
 * `utils/activityRange.ts` so both clients render and validate a range identically.
 *
 * The end is a calendar day (an exact-UTC-midnight instant, ADR 0015), read as its own date in
 * every zone. The start may also carry a time of day when the editor added one; like the
 * single-day rows ([DateFormat.formatTimestamp]) such a real instant reads in the device zone.
 * Reminders, cadence and timeline sorting still use the start only: the end never creates a
 * second interaction date.
 */
object ActivityDateRange {

    private val MONTH_NAME_FORMATS =
        setOf(DateFormat.US_MMM, DateFormat.US_MMMM, DateFormat.EU_MMM, DateFormat.EU_MMMM)

    /** The calendar day the activity starts on, or null when [start] is absent or unparseable. */
    fun startDay(start: String?, zone: ZoneId = ZoneId.systemDefault()): LocalDate? {
        if (start.isNullOrBlank()) return null
        DateFormat.calendarDay(start)?.let { (y, m, d) -> return LocalDate.of(y, m, d) }
        return runCatching { Instant.parse(start).atZone(zone).toLocalDate() }.getOrNull()
    }

    /** The calendar day the activity ends on (its UTC date, never zone-shifted), or null. */
    private fun endDay(end: String?): LocalDate? {
        if (end.isNullOrBlank()) return null
        DateFormat.calendarDay(end)?.let { (y, m, d) -> return LocalDate.of(y, m, d) }
        return runCatching { Instant.parse(end).atOffset(ZoneOffset.UTC).toLocalDate() }.getOrNull()
    }

    /** True when [end] names a calendar day strictly before [start]'s (equal days are valid). */
    fun endBeforeStart(
        start: String?,
        end: String?,
        zone: ZoneId = ZoneId.systemDefault()
    ): Boolean {
        val s = startDay(start, zone) ?: return false
        val e = endDay(end) ?: return false
        return e.isBefore(s)
    }

    /**
     * The "start – end" text, or null when this is a single-day activity (no end, an end equal to
     * or before the start, or an unparseable value). In the month-name formats a range inside one
     * month collapses ("12–15 May, 2026" / "May 12–15, 2026"); the numeric formats always show
     * both ends in full, since a collapsed "12.05.2026" would be ambiguous.
     */
    fun rangeOrNull(
        start: String?,
        end: String?,
        format: String,
        zone: ZoneId = ZoneId.systemDefault(),
        locale: Locale = Locale.getDefault()
    ): String? {
        val s = startDay(start, zone) ?: return null
        val e = endDay(end) ?: return null
        if (!e.isAfter(s)) return null

        val sameMonth = s.year == e.year && s.month == e.month
        if (sameMonth && format in MONTH_NAME_FORMATS) {
            val endYearless = DateFormat.formatYearless(e.monthValue, e.dayOfMonth, format, locale)
            val startYearless =
                DateFormat.formatYearless(s.monthValue, s.dayOfMonth, format, locale)
            return when (format) {
                DateFormat.EU_MMM -> "${s.dayOfMonth}–$endYearless, ${s.year}"
                DateFormat.EU_MMMM -> "%02d–%s, %d".format(s.dayOfMonth, endYearless, s.year)
                else -> "$startYearless–${e.dayOfMonth}, ${s.year}"
            }
        }
        val from = DateFormat.formatFull(s.year, s.monthValue, s.dayOfMonth, format, locale)
        val to = DateFormat.formatFull(e.year, e.monthValue, e.dayOfMonth, format, locale)
        return "$from – $to"
    }

    /** An activity's date as shown in a row: the range when it spans days, else the start day. */
    fun format(
        start: String?,
        end: String?,
        format: String,
        zone: ZoneId = ZoneId.systemDefault(),
        locale: Locale = Locale.getDefault()
    ): String = rangeOrNull(start, end, format, zone, locale)
        ?: DateFormat.formatTimestamp(start, format, zone, locale)
}
