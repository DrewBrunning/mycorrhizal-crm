package com.mycorrhizal.crm.feature.contacts

import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * `formatFileSize` (B/KB/MB/GB/TB formatting for a file picker row's size
 * subtitle) is a standalone pure function with zero extraction work needed,
 * flagged in the coverage-analysis triage as completely untested despite
 * that. Covers the 0/negative/unit-boundary edge cases.
 */
class SeafileFilePickerDialogTest {

    @Test
    fun `zero bytes formats as 0 B`() {
        assertEquals("0 B", formatFileSize(0L))
    }

    @Test
    fun `a negative size formats as 0 B`() {
        // Defensive: a picker/provider should never report a negative size,
        // but the function must not render nonsense (e.g. a negative KB) if
        // one ever does.
        assertEquals("0 B", formatFileSize(-1L))
    }

    @Test
    fun `a small byte count has no decimal and no unit conversion`() {
        assertEquals("1 B", formatFileSize(1L))
        assertEquals("1023 B", formatFileSize(1023L))
    }

    @Test
    fun `the KB boundary converts at exactly 1024 bytes`() {
        assertEquals("1.0 KB", formatFileSize(1024L))
    }

    @Test
    fun `a value between KB steps rounds to one decimal`() {
        assertEquals("1.5 KB", formatFileSize(1536L))
    }

    @Test
    fun `the MB boundary converts at exactly 1024 KB`() {
        assertEquals("1.0 MB", formatFileSize(1024L * 1024))
    }

    @Test
    fun `the GB boundary converts at exactly 1024 MB`() {
        assertEquals("1.0 GB", formatFileSize(1024L * 1024 * 1024))
    }

    @Test
    fun `the TB boundary converts at exactly 1024 GB`() {
        assertEquals("1.0 TB", formatFileSize(1024L * 1024 * 1024 * 1024))
    }

    @Test
    fun `a size beyond TB stays in TB rather than overflowing to an unknown unit`() {
        // TB is the largest unit in the table; the exponent is coerced to
        // units.lastIndex rather than indexing past the array.
        val bytes = 1024L * 1024 * 1024 * 1024 * 1024 // 1 PB
        assertEquals("1024.0 TB", formatFileSize(bytes))
    }
}
