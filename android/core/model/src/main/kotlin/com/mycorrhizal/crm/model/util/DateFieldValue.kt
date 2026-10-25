package com.mycorrhizal.crm.model.util

import com.mycorrhizal.crm.model.network.PartialDate
import java.time.Instant
import java.time.LocalDate
import java.time.LocalTime
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.Locale

/**
 * How a date-entry field stores its value on the wire (issue #1726).
 *
 * The modes exist because the clients have to write more than one date shape:
 *  - [DATE_ONLY] — a plain `YYYY-MM-DD` string (list filters, the gift form's
 *    date — giftDateToIso then sends it as that day at `00:00:00Z` — and a
 *    custom field of type `date`).
 *  - [DATE_UTC_MIDNIGHT] — a calendar day stored as the exact UTC-midnight
 *    instant (a reminder's `remind_at`, ADR 0015 category 2).
 *  - [DATE_TIME] — an optional time; no time writes UTC midnight
 *    (`Activity.Date` / `Note.Date`, whose Go DTOs are `time.Time` and cannot
 *    unmarshal a bare date).
 *  - [DATE_TIME_OPTIONAL] — an optional time in an opaque string field; no
 *    time writes a plain `YYYY-MM-DD` (a custom field of type `datetime`).
 *  - [PARTIAL] — a birthday/anniversary/life-event partial date: `YYYY-MM-DD`
 *    or the year-less `--MM-DD` (ADR 0015 category 3).
 */
enum class DateFieldMode(val allowsTime: Boolean) {
    DATE_ONLY(false),
    DATE_UTC_MIDNIGHT(false),
    DATE_TIME(true),
    DATE_TIME_OPTIONAL(true),
    PARTIAL(false)
}

/**
 * The editor state decoded from a stored date value. [date] is a picker
 * position, not necessarily a value to store: for a year-less partial it is a
 * leap-year placeholder (see [DateFieldValue.PARTIAL_PLACEHOLDER_YEAR]), and
 * for a year-only value it is 1 January of that year with [yearOnly] set.
 */
data class DateFieldState(
    val date: LocalDate? = null,
    val time: LocalTime? = null,
    val yearUnknown: Boolean = false,
    val yearOnly: Int? = null,
    /** A year+month partial (`YYYY-MM`, no day) — kept visible and untouched on save. */
    val yearMonth: String? = null
)

/**
 * Pure value mapping for the shared Android date field (issue #1726). Kept out
 * of the composable so the round trips the ticket calls for — date-only,
 * date+time, year-unknown partial, and the reverse — are unit-testable without
 * driving the Material date picker.
 */
object DateFieldValue {
    /** The leap year a `--MM-DD` value opens on, so 29 Feb stays selectable. */
    const val PARTIAL_PLACEHOLDER_YEAR = 2000

    private val PARTIAL_YEARLESS = Regex("""^--(\d{2})-(\d{2})$""")
    private val PARTIAL_FULL = Regex("""^(\d{4})-(\d{2})-(\d{2})$""")
    private val PARTIAL_YEAR_MONTH = Regex("""^(\d{4})-(\d{2})$""")
    private val YEAR_ONLY = Regex("""^(\d{4})$""")
    private val RFC3339_OFFSET = DateTimeFormatter.ofPattern("yyyy-MM-dd'T'HH:mm:ssXXX")

    /** Decode [value] for [mode]. Unparseable or absent values yield an empty state. */
    fun parse(
        value: String?,
        mode: DateFieldMode,
        zone: ZoneId = ZoneId.systemDefault()
    ): DateFieldState {
        val raw = value?.trim().orEmpty()
        if (raw.isEmpty()) return DateFieldState()
        return when (mode) {
            DateFieldMode.PARTIAL -> parsePartial(raw)

            DateFieldMode.DATE_ONLY,
            DateFieldMode.DATE_UTC_MIDNIGHT
            -> parseDay(raw, zone, allowTime = false)

            DateFieldMode.DATE_TIME,
            DateFieldMode.DATE_TIME_OPTIONAL
            -> parseDay(raw, zone, allowTime = true)
        }
    }

    private fun parsePartial(raw: String): DateFieldState {
        PARTIAL_YEARLESS.matchEntire(raw)?.let { match ->
            val month = match.groupValues[1].toInt()
            val day = match.groupValues[2].toInt()
            val date = runCatching {
                LocalDate.of(
                    PARTIAL_PLACEHOLDER_YEAR,
                    month,
                    day
                )
            }.getOrNull()
                ?: return DateFieldState()
            return DateFieldState(date = date, yearUnknown = true)
        }
        PARTIAL_FULL.matchEntire(raw)?.let { match ->
            val date = runCatching {
                LocalDate.of(
                    match.groupValues[1].toInt(),
                    match.groupValues[2].toInt(),
                    match.groupValues[3].toInt()
                )
            }.getOrNull() ?: return DateFieldState()
            return DateFieldState(date = date)
        }
        PARTIAL_YEAR_MONTH.matchEntire(raw)?.let { match ->
            val year = match.groupValues[1].toInt()
            val month = match.groupValues[2].toInt()
            val date = runCatching { LocalDate.of(year, month, 1) }.getOrNull()
                ?: return DateFieldState()
            return DateFieldState(date = date, yearMonth = raw)
        }
        YEAR_ONLY.matchEntire(raw)?.let { match ->
            val year = match.groupValues[1].toInt()
            val date = runCatching { LocalDate.of(year, 1, 1) }.getOrNull()
                ?: return DateFieldState()
            return DateFieldState(date = date, yearOnly = year)
        }
        return DateFieldState()
    }

    private fun parseDay(raw: String, zone: ZoneId, allowTime: Boolean): DateFieldState {
        DateFormat.calendarDay(raw)?.let { (year, month, day) ->
            return DateFieldState(date = LocalDate.of(year, month, day))
        }
        val instant = runCatching { Instant.parse(raw) }.getOrNull() ?: return DateFieldState()
        val local = instant.atZone(zone)
        return DateFieldState(
            date = local.toLocalDate(),
            time = if (allowTime) local.toLocalTime().withSecond(0).withNano(0) else null
        )
    }

    /** What the read-only field shows: the stored value in the user's date/time preferences. */
    fun display(
        value: String?,
        mode: DateFieldMode,
        dateFormat: String,
        is24Hour: Boolean,
        zone: ZoneId = ZoneId.systemDefault(),
        locale: Locale = Locale.getDefault()
    ): String {
        val state = parse(value, mode, zone)
        state.yearOnly?.let { return "$it" }
        state.yearMonth?.let { return it }
        val date = state.date ?: return ""
        val dateText = if (state.yearUnknown) {
            DateFormat.formatYearless(date.monthValue, date.dayOfMonth, dateFormat, locale)
        } else {
            DateFormat.formatFull(date.year, date.monthValue, date.dayOfMonth, dateFormat, locale)
        }
        return if (mode.allowsTime && state.time != null) {
            "$dateText ${timeLabel(state.time, is24Hour, locale)}"
        } else {
            dateText
        }
    }

    /** Encode the picker's [date]/[time] for [mode]'s wire shape. */
    fun pick(
        date: LocalDate,
        time: LocalTime?,
        mode: DateFieldMode,
        yearUnknown: Boolean = false,
        zone: ZoneId = ZoneId.systemDefault()
    ): String = when (mode) {
        DateFieldMode.PARTIAL -> if (yearUnknown) {
            "--%02d-%02d".format(date.monthValue, date.dayOfMonth)
        } else {
            date.toString()
        }

        DateFieldMode.DATE_ONLY -> date.toString()

        DateFieldMode.DATE_UTC_MIDNIGHT -> "${date}T00:00:00Z"

        DateFieldMode.DATE_TIME -> if (time != null) {
            instant(
                date,
                time,
                zone
            )
        } else {
            "${date}T00:00:00Z"
        }

        DateFieldMode.DATE_TIME_OPTIONAL -> if (time != null) {
            instant(
                date,
                time,
                zone
            )
        } else {
            date.toString()
        }
    }

    /** An RFC 3339 instant with seconds and the device zone's offset. */
    private fun instant(date: LocalDate, time: LocalTime, zone: ZoneId): String =
        date.atTime(time).atZone(zone).format(RFC3339_OFFSET)

    /**
     * Normalise a date-only string for a `time.Time` wire field: a calendar
     * day becomes its exact UTC-midnight instant, anything else is returned
     * unchanged. DateField already emits the instant, but this keeps a
     * date-only value from any other caller (a nav-arg prefill, an older
     * persisted draft) acceptable to the Go DTO.
     */
    fun normalizeCalendarDayInstant(value: String?): String? {
        val trimmed = value?.trim().orEmpty()
        if (trimmed.isEmpty()) return null
        return if (Regex("""^\d{4}-\d{2}-\d{2}$""").matches(trimmed)) {
            "${trimmed}T00:00:00Z"
        } else {
            trimmed
        }
    }

    /** The time-of-day label, honouring the user's 12/24-hour preference. */
    fun timeLabel(
        time: LocalTime,
        is24Hour: Boolean,
        locale: Locale = Locale.getDefault()
    ): String = DateTimeFormatter
        .ofPattern(if (is24Hour) "HH:mm" else "h:mm a", locale)
        .format(time)

    /**
     * The wire/editor string for a [PartialDate] (birthdays, anniversaries,
     * life events): full, `--MM-DD`, `YYYY-MM`, or a bare year for reduced
     * precision. Unlike [PartialDate.formatForEdit] this keeps year-only and
     * year-month values visible.
     */
    fun partialFieldValue(partial: PartialDate?): String {
        if (partial == null) return ""
        val month = partial.month
        val day = partial.day
        return when {
            month != null && day != null && partial.year != null ->
                "%04d-%02d-%02d".format(partial.year, month, day)

            month != null && day != null -> "--%02d-%02d".format(month, day)

            partial.year != null && month != null -> "%04d-%02d".format(partial.year, month)

            partial.year != null -> "${partial.year}"

            else -> ""
        }
    }
}
