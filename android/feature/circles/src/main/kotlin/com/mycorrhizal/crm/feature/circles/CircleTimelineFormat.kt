package com.mycorrhizal.crm.feature.circles

import androidx.compose.ui.graphics.Color
import com.mycorrhizal.crm.model.network.CircleTimelineItem
import com.mycorrhizal.crm.model.util.DateFormat
import java.time.LocalDate
import java.util.Locale

private val SUMMARY_KEYS = listOf("content", "title", "message", "name", "description", "type")

/**
 * A one-line summary of a timeline entry: the first non-blank text field (mirrors the web dialog).
 * The `type` fallback is a stored token (`married`), so it goes through [tokenLabel] (issue #2626).
 */
internal fun CircleTimelineItem.summary(tokenLabel: (String) -> String = { it }): String {
    val values = data ?: return ""
    for (key in SUMMARY_KEYS) {
        val value = values[key]
        if (value is String && value.isNotBlank()) {
            return if (key == "type") tokenLabel(value) else value
        }
    }
    return ""
}

private fun calendarDay(iso: String): LocalDate? =
    runCatching { LocalDate.parse(iso.take(ISO_DATE_LENGTH)) }.getOrNull()

private const val ISO_DATE_LENGTH = 10

/**
 * Calendar date of [iso] (its first 10 characters, so an offset never shifts the day — web's
 * `calendarDateOnly`) in the user's date-format preference [dateFormat] (issue #2618). A later
 * [endIso] day renders "start – end" (activities). An unparseable [iso] falls back to the raw
 * string.
 */
internal fun formatTimelineDate(
    iso: String,
    endIso: String?,
    dateFormat: String,
    locale: Locale = Locale.getDefault()
): String {
    val start = calendarDay(iso) ?: return iso
    fun LocalDate.show() = DateFormat.formatFull(year, monthValue, dayOfMonth, dateFormat, locale)
    val end = endIso?.let(::calendarDay)
    return if (end != null && end.isAfter(start)) {
        "${start.show()} – ${end.show()}"
    } else {
        start.show()
    }
}

/**
 * Display colour for a `CircleColors` token, or null for "" / an unknown token. The backend
 * stores only the token; these mid-tone values mirror the web `CIRCLE_COLOR_HEX`.
 */
internal fun circleColorOrNull(token: String): Color? = when (token) {
    "red" -> Color(0xFFE53935)
    "orange" -> Color(0xFFFB8C00)
    "yellow" -> Color(0xFFFDD835)
    "green" -> Color(0xFF43A047)
    "teal" -> Color(0xFF00897B)
    "blue" -> Color(0xFF1E88E5)
    "purple" -> Color(0xFF8E24AA)
    "pink" -> Color(0xFFD81B60)
    "grey" -> Color(0xFF757575)
    else -> null
}
