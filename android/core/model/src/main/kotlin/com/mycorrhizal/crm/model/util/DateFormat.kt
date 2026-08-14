package com.mycorrhizal.crm.model.util

import com.mycorrhizal.crm.model.network.PartialDate
import java.time.Instant
import java.time.ZoneOffset

/**
 * Display formatting for dates following the backend's `date_format`
 * preference. Tokens mirror the frontend's DateFormatProvider values
 * (`eu`, `us`, `iso`, `ca`, `eu-hyphen`, `us-mmm`, ...). Phase-1 scope
 * covers the full and yearless renderings used on the detail screen.
 */
object DateFormat {
    val EU = "eu"
    val US = "us"
    val ISO = "iso"

    private val months = listOf(
        "January", "February", "March", "April", "May", "June",
        "July", "August", "September", "October", "November", "December",
    )

    private val monthsShort = listOf(
        "Jan", "Feb", "Mar", "Apr", "May", "Jun",
        "Jul", "Aug", "Sep", "Oct", "Nov", "Dec",
    )

    /**
     * Format a partial date for display:
     *  - full date: "15 June 1990" (eu), "June 15, 1990" (us), "1990-06-15" (iso)
     *  - yearless month+day: "25 December"
     *  - year only: "2020"
     *  - unknown: ""
     */
    fun PartialDate.display(format: String): String = when {
        year != null && hasMonthDay -> formatFull(year!!, month!!, day!!, format)
        hasMonthDay -> "${day} ${months[month!! - 1]}" // yearless
        isYearOnly -> "$year"
        else -> ""
    }

    fun formatFull(year: Int, month: Int, day: Int, format: String): String = when (format) {
        US -> "${months[month - 1]} $day, $year"
        ISO -> "%04d-%02d-%02d".format(year, month, day)
        EU -> "$day ${months[month - 1]} $year"
        else -> "$day ${months[month - 1]} $year"
    }

    fun monthName(month: Int): String = months[month - 1]

    fun monthNameShort(month: Int): String = monthsShort[month - 1]

    /**
     * Formats an ISO-8601 timestamp as a date in [format], or "" when
     * unparseable/absent. The single shared implementation for server-derived
     * calendar timestamps (cadence next-due/last-interaction, briefing dates) —
     * M11's prep view and M12's cadence panel must render the same value the
     * same way, so both call this rather than formatting inline.
     *
     * **UTC, not the device zone.** The web's formatDateWithFormat reads
     * getUTCDate()/getUTCMonth()/getUTCFullYear(), so a stored instant like
     * `2026-09-10T01:00:00Z` renders "2026-09-10" no matter where the user sits.
     * Converting to the device zone shifts the date for anyone west of UTC —
     * the same value rendered as two different days across clients (a real bug
     * M11's review pass fixed in the prep view).
     */
    fun formatTimestamp(iso: String?, format: String): String {
        if (iso.isNullOrBlank()) return ""
        return runCatching {
            val zoned = Instant.parse(iso).atZone(ZoneOffset.UTC)
            formatFull(zoned.year, zoned.monthValue, zoned.dayOfMonth, format)
        }.getOrDefault("")
    }
}
