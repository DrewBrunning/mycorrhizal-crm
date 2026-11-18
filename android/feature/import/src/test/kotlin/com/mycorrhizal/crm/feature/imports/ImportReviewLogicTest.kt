package com.mycorrhizal.crm.feature.imports

import com.mycorrhizal.crm.model.network.DuplicateMatch
import com.mycorrhizal.crm.model.network.ImportDiagnostic
import com.mycorrhizal.crm.model.network.ImportRowEdit
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

    // Issue #2611: a saved fix takes the row out of the error group.
    @Test
    fun `a row with a saved fix is no longer an error row`() {
        val edits = mapOf(
            broken.rowIndex to ImportRowEdit(rowIndex = broken.rowIndex, email = "a@b.c")
        )
        assertFalse(broken.matches(ImportReviewFilter.ERRORS, edits))
        assertTrue(broken.matches(ImportReviewFilter.NEW, edits))
        assertTrue(broken.matches(null, edits))
        // An edit for another row changes nothing.
        val other = mapOf(99 to ImportRowEdit(rowIndex = 99, email = "a@b.c"))
        assertTrue(broken.matches(ImportReviewFilter.ERRORS, other))
        assertFalse(broken.matches(ImportReviewFilter.NEW, other))
    }

    @Test
    fun `applying a fix shows the edited values and an empty value removes the entry`() {
        val row = ImportRowPreview(
            rowIndex = 1,
            parsedContact = mapOf(
                "firstname" to "Beta",
                "lastname" to "Tester",
                "email" to "not-an-email",
                "phone" to "123",
                "birthday" to "1990-01-01"
            )
        )
        val fixed = row.withEdit(
            ImportRowEdit(rowIndex = 1, firstname = "Bea", email = "bea@example.com", phone = "")
        )
        assertEquals("Bea", fixed.parsedContact["firstname"])
        assertEquals("Tester", fixed.parsedContact["lastname"])
        assertEquals("bea@example.com", fixed.parsedContact["email"])
        assertFalse(fixed.parsedContact.containsKey("phone"))
        // Fields the fix form cannot touch are carried over.
        assertEquals("1990-01-01", fixed.parsedContact["birthday"])
        assertEquals(row, row.withEdit(null))
    }

    // Issue #2610: only the name/email/phone errors can be fixed in the form.
    @Test
    fun `birthday anniversary and parse errors cannot be fixed in the form`() {
        fun rowWith(vararg errors: String) =
            ImportRowPreview(rowIndex = 0, validationErrors = errors.toList())
        assertTrue(
            rowWith("Invalid birthday format (expected YYYY-MM-DD or --MM-DD)").hasUnfixableError()
        )
        assertTrue(
            rowWith(
                "Invalid anniversary format (expected YYYY-MM-DD or --MM-DD)"
            ).hasUnfixableError()
        )
        assertTrue(rowWith("Failed to parse vCard: unexpected end of name").hasUnfixableError())
        assertTrue(rowWith("Invalid email format", "Invalid birthday format").hasUnfixableError())
        assertFalse(rowWith("First name is required").hasUnfixableError())
        assertFalse(rowWith("Invalid email format", "Invalid phone format").hasUnfixableError())
        assertFalse(fresh.hasUnfixableError())
    }

    // Issue #2653: a birthday that cannot be parsed is now a warning on a valid row.
    @Test
    fun `a row with only a birthday warning is valid, new and not unfixable`() {
        val warned = ImportRowPreview(
            rowIndex = 0,
            diagnostics = listOf(
                ImportDiagnostic("warn", "birthday", "birthday not imported: invalid value")
            )
        )

        assertFalse(warned.hasUnfixableError())
        assertTrue(warned.matches(ImportReviewFilter.NEW))
        assertFalse(warned.matches(ImportReviewFilter.ERRORS))
    }
}
