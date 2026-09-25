package com.mycorrhizal.crm.model.network

/**
 * ADR 0025 (#1233) period helpers, shared by the contact form ViewModel and
 * the address editor. Keeping the (kind, entryId) upsert in one place means the
 * "periods are keyed by neutral element ID" rule can't drift between callers.
 */

/** The period attached to (kind, entryId), or null. */
fun entryPeriodOf(periods: List<EntryPeriod>, kind: String, entryId: String): EntryPeriod? =
    periods.firstOrNull { it.kind == kind && it.entryId == entryId }

/**
 * Replace the (kind, entryId) period with one holding [range], or remove it
 * when [range] is null. Every other period is preserved.
 */
fun upsertEntryPeriod(
    periods: List<EntryPeriod>,
    kind: String,
    entryId: String,
    range: TemporalRange?,
): List<EntryPeriod> {
    val rest = periods.filterNot { it.kind == kind && it.entryId == entryId }
    return if (range == null) rest else rest + EntryPeriod(kind = kind, entryId = entryId, range = range)
}

/**
 * Build a whole-year [TemporalRange] from text fields, or null when both are
 * blank. A non-numeric side is treated as blank.
 */
fun yearTemporalRange(start: String, end: String): TemporalRange? {
    val startYear = start.trim().toIntOrNull()
    val endYear = end.trim().toIntOrNull()
    if (startYear == null && endYear == null) return null
    return TemporalRange(
        start = startYear?.let { PartialDate(year = it) },
        end = endYear?.let { PartialDate(year = it) },
    )
}
