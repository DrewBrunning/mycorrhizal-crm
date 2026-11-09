package com.mycorrhizal.crm.feature.imports

import com.mycorrhizal.crm.model.network.DuplicateMatch
import com.mycorrhizal.crm.model.network.ImportRowIssue
import com.mycorrhizal.crm.model.network.ImportRowPreview
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

// Issues #2184/#2185/#2187 (Android parity, #2210): the pure decisions behind the review step.
class ImportReviewLogicTest {

    private val fresh = ImportRowPreview(rowIndex = 0)
    private val matched = ImportRowPreview(
        rowIndex = 1,
        duplicateMatch = DuplicateMatch(existingContactId = 7, matchReason = "email")
    )
    private val twin = ImportRowPreview(rowIndex = 2, batchDuplicateOf = 0)
    private val broken = ImportRowPreview(rowIndex = 3, validationErrors = listOf("name required"))
    private val brokenAndMatched = matched.copy(
        rowIndex = 4,
        validationErrors = listOf("bad email")
    )

    @Test
    fun `no filter shows every row`() {
        listOf(fresh, matched, twin, broken).forEach { assertTrue(it.matches(null)) }
    }

    @Test
    fun `new filter keeps only valid rows without a duplicate`() {
        assertTrue(fresh.matches(ImportReviewFilter.NEW))
        assertFalse(matched.matches(ImportReviewFilter.NEW))
        assertFalse(twin.matches(ImportReviewFilter.NEW))
        assertFalse(broken.matches(ImportReviewFilter.NEW))
    }

    @Test
    fun `duplicates filter keeps valid rows matching an existing contact or the batch`() {
        assertTrue(matched.matches(ImportReviewFilter.DUPLICATES))
        assertTrue(twin.matches(ImportReviewFilter.DUPLICATES))
        assertFalse(fresh.matches(ImportReviewFilter.DUPLICATES))
        // An errored row is an error first, never double counted.
        assertFalse(brokenAndMatched.matches(ImportReviewFilter.DUPLICATES))
    }

    @Test
    fun `errors filter keeps only rows with validation errors`() {
        assertTrue(broken.matches(ImportReviewFilter.ERRORS))
        assertTrue(brokenAndMatched.matches(ImportReviewFilter.ERRORS))
        assertFalse(fresh.matches(ImportReviewFilter.ERRORS))
    }

    @Test
    fun `a row without a duplicate offers only keep and skip`() {
        assertEquals(listOf("add", "skip"), fresh.availableActions())
    }

    @Test
    fun `a row matching an existing contact offers keep both, merge and discard`() {
        assertEquals(listOf("add", "update", "skip"), matched.availableActions())
    }

    @Test
    fun `a within-batch twin has nothing to merge into`() {
        assertEquals(listOf("add", "skip"), twin.availableActions())
        assertTrue(twin.isDuplicate())
    }

    @Test
    fun `the csv report has a header and one neutralised line per issue`() {
        val csv = importIssuesToCsv(
            listOf(
                ImportRowIssue(3, "=SUM(A1)", "a@b.c", "+49 1", "skipped", "Invalid email, fix it"),
                ImportRowIssue(5, "Dana \"D\" White", "", "", "failed", "Could not save")
            )
        )
        val lines = csv.trimEnd().split("\r\n")
        assertEquals("Row,Name,Email,Phone,Status,Reason", lines[0])
        assertEquals("3,'=SUM(A1),a@b.c,'+49 1,skipped,\"Invalid email, fix it\"", lines[1])
        assertEquals("5,\"Dana \"\"D\"\" White\",,,failed,Could not save", lines[2])
    }
}
