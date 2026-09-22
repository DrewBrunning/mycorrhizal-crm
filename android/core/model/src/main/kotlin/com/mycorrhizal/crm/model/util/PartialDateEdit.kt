package com.mycorrhizal.crm.model.util

import com.mycorrhizal.crm.model.network.PartialDate

/**
 * Issue #832: round-trip-editable rendering of a [PartialDate] — "1990-06-15"
 * (full) or "--06-15" (yearless), matching the mask the birthday text field
 * has always used. Blank when month/day are unknown (a year-only or empty
 * partial has nothing editable here; the caller preserves it on save rather
 * than clearing it). Extracted from the birthday-only logic that used to live
 * privately in `ContactFormViewModel` so the new anniversaries list editor
 * can share it — this is a display/parse pair for the edit-mode text field,
 * NOT `DateFormat.display()`, which is a localized read-only renderer that
 * doesn't round-trip (e.g. "15 June 1990" can't be parsed back).
 */
fun PartialDate.formatForEdit(): String {
    val monthStr = month?.toString()?.padStart(2, '0')
    val dayStr = day?.toString()?.padStart(2, '0')
    if (monthStr == null || dayStr == null) return ""
    return if (year == null) "--$monthStr-$dayStr" else "$year-$monthStr-$dayStr"
}

/** Parse the same round-trip format back into a [PartialDate]. */
fun parsePartialDateForEdit(value: String): PartialDate {
    val clean = value.trim()
    val (year, month, day) = if (clean.startsWith("--")) {
        val parts = clean.substring(2).split("-")
        Triple(null, parts.getOrNull(0)?.toIntOrNull(), parts.getOrNull(1)?.toIntOrNull())
    } else {
        val parts = clean.split("-")
        Triple(parts.getOrNull(0)?.toIntOrNull(), parts.getOrNull(1)?.toIntOrNull(), parts.getOrNull(2)?.toIntOrNull())
    }
    return PartialDate(year = year, month = month, day = day)
}
