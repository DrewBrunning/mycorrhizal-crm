package com.mycorrhizal.crm.model.network

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** ADR 0025 (#1233): the shared (kind, entryId) period upsert helpers. */
class EntryPeriodsTest {

    @Test
    fun `yearTemporalRange needs at least one side and ignores non-numeric text`() {
        assertNull(yearTemporalRange("", ""))
        assertNull(yearTemporalRange("abc", ""))
        assertEquals(TemporalRange(start = PartialDate(year = 2019)), yearTemporalRange("2019", ""))
        assertEquals(TemporalRange(end = PartialDate(year = 2024)), yearTemporalRange("", "2024"))
        assertEquals(
            TemporalRange(start = PartialDate(year = 2019), end = PartialDate(year = 2024)),
            yearTemporalRange("2019", "2024"),
        )
    }

    @Test
    fun `upsertEntryPeriod adds when absent and preserves other periods`() {
        val existing = listOf(
            EntryPeriod(kind = "address", entryId = "addr-1", range = TemporalRange(start = PartialDate(year = 2000))),
        )
        val result = upsertEntryPeriod(
            existing,
            "organization",
            "org-1",
            TemporalRange(start = PartialDate(year = 2019)),
        )
        assertEquals(2, result.size)
        assertTrue(result.any { it.kind == "organization" && it.entryId == "org-1" })
        assertTrue(result.any { it.kind == "address" && it.entryId == "addr-1" })
    }

    @Test
    fun `upsertEntryPeriod replaces the matching period`() {
        val existing = listOf(
            EntryPeriod(kind = "organization", entryId = "org-1", range = TemporalRange(start = PartialDate(year = 2010))),
        )
        val result = upsertEntryPeriod(
            existing,
            "organization",
            "org-1",
            TemporalRange(start = PartialDate(year = 2019)),
        )
        assertEquals(1, result.size)
        assertEquals(2019, result.first().range.start?.year)
    }

    @Test
    fun `upsertEntryPeriod removes the matching period for a null range`() {
        val existing = listOf(
            EntryPeriod(kind = "organization", entryId = "org-1", range = TemporalRange(start = PartialDate(year = 2010))),
        )
        assertTrue(upsertEntryPeriod(existing, "organization", "org-1", null).isEmpty())
    }

    @Test
    fun `entryPeriodOf finds by kind and entry`() {
        val periods = listOf(
            EntryPeriod(kind = "address", entryId = "addr-1", range = TemporalRange()),
            EntryPeriod(kind = "organization", entryId = "org-1", range = TemporalRange(start = PartialDate(year = 2019))),
        )
        assertEquals("org-1", entryPeriodOf(periods, "organization", "org-1")?.entryId)
        assertNull(entryPeriodOf(periods, "title", "title-1"))
    }
}
