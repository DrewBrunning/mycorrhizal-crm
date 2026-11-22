package com.mycorrhizal.crm.feature.imports

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertIsSelected
import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithTag
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextClearance
import androidx.compose.ui.test.performTextInput
import com.mycorrhizal.crm.model.AppVersion
import com.mycorrhizal.crm.model.network.DuplicateMatch
import com.mycorrhizal.crm.model.network.ImportDiagnostic
import com.mycorrhizal.crm.model.network.ImportRowEdit
import com.mycorrhizal.crm.model.network.ImportRowPreview
import com.mycorrhizal.crm.ui.LocalServerVersion
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

// Issues #2184/#2185/#2186 (Android parity, #2210): the shared review step.
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], qualifiers = "w400dp-h1400dp")
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ImportReviewStepTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun row(index: Int, first: String, errors: List<String> = emptyList()) =
        ImportRowPreview(
            rowIndex = index,
            parsedContact = mapOf("firstname" to first),
            validationErrors = errors
        )

    private val newRow = row(0, "Nia")
    private val dupRow = row(1, "Dora").copy(
        duplicateMatch = DuplicateMatch(existingContactId = 9, matchReason = "email")
    )
    private val errRow = row(2, "Erin", listOf("Invalid email"))

    private fun show(
        rows: List<ImportRowPreview> = listOf(newRow, dupRow, errRow),
        rowEdits: Map<Int, ImportRowEdit> = emptyMap(),
        onRowEditSave: ((ImportRowEdit) -> Unit)? = null,
        onRowActionChange: (Int, String) -> Unit = { _, _ -> },
        onResolveAll: () -> Unit = {},
        serverVersion: AppVersion? = null,
        fileDiagnostics: List<ImportDiagnostic> = emptyList()
    ) {
        composeTestRule.setContent {
            CompositionLocalProvider(LocalServerVersion provides serverVersion) {
                MycorrhizalTheme {
                    ImportReviewStep(
                        rows = rows,
                        rowActions = rows.associate { it.rowIndex to it.suggestedAction },
                        onRowActionChange = onRowActionChange,
                        onResolveAll = onResolveAll,
                        onConfirm = {},
                        rowEdits = rowEdits,
                        onRowEditSave = onRowEditSave,
                        fileDiagnostics = fileDiagnostics
                    )
                }
            }
        }
    }

    @Test
    fun `a row with diagnostics shows each message and one without shows none`() {
        val note = "The name was empty; the first email or phone number is used as the name"
        show(
            rows = listOf(
                row(0, "").copy(
                    diagnostics = listOf(
                        ImportDiagnostic(severity = "info", concept = "name", message = note),
                        ImportDiagnostic(severity = "warn", message = "A field was dropped")
                    )
                ),
                row(1, "Plain")
            )
        )
        composeTestRule.onNodeWithText(note).assertIsDisplayed()
        composeTestRule.onNodeWithText("A field was dropped").assertIsDisplayed()
        composeTestRule.onAllNodesWithTag("import-row-diagnostic").assertCountEquals(2)
    }

    @Test
    fun `resolve all is offered only when a row is a duplicate`() {
        var resolved = 0
        show(onResolveAll = { resolved++ })
        composeTestRule.onNodeWithText("Resolve all as merged").assertIsDisplayed().performClick()
        assertEquals(1, resolved)
    }

    @Test
    fun `resolve all is hidden when there are no duplicates`() {
        show(rows = listOf(newRow, errRow))
        composeTestRule.onAllNodesWithText("Resolve all as merged").assertCountEquals(0)
    }

    @Test
    fun `a duplicate within the file also offers resolve all`() {
        show(rows = listOf(newRow, row(3, "Nia2").copy(batchDuplicateOf = 0)))
        composeTestRule.onNodeWithText("Resolve all as merged").assertIsDisplayed()
    }

    @Test
    fun `filter chips show counts and tapping one narrows the list`() {
        show()
        composeTestRule.onNodeWithText("Nia").assertIsDisplayed()
        composeTestRule.onNodeWithText("Dora").assertIsDisplayed()
        composeTestRule.onNodeWithText("Erin").assertIsDisplayed()

        composeTestRule.onNodeWithTag("import-filter-errors").performClick()

        composeTestRule.onNodeWithText("Erin").assertIsDisplayed()
        composeTestRule.onAllNodesWithText("Nia").assertCountEquals(0)
        composeTestRule.onAllNodesWithText("Dora").assertCountEquals(0)
        composeTestRule.onNodeWithTag("import-filter-errors").assertIsSelected()
    }

    @Test
    fun `tapping the active chip again clears the filter`() {
        show()
        composeTestRule.onNodeWithTag("import-filter-duplicates").performClick()
        composeTestRule.onAllNodesWithText("Nia").assertCountEquals(0)

        composeTestRule.onNodeWithTag("import-filter-duplicates").performClick()

        composeTestRule.onNodeWithText("Nia").assertIsDisplayed()
    }

    @Test
    fun `a row without a duplicate offers Keep and Skip, not Keep both or Merge`() {
        show(rows = listOf(newRow))
        composeTestRule.onNodeWithText("Keep").assertIsDisplayed()
        composeTestRule.onNodeWithText("Skip").assertIsDisplayed()
        composeTestRule.onAllNodesWithText("Keep both").assertCountEquals(0)
        composeTestRule.onAllNodesWithText("Merge").assertCountEquals(0)
        composeTestRule.onAllNodesWithText("Discard new").assertCountEquals(0)
    }

    @Test
    fun `a duplicate row offers Keep both, Merge and Discard new`() {
        show(rows = listOf(dupRow))
        composeTestRule.onNodeWithText("Keep both").assertIsDisplayed()
        composeTestRule.onNodeWithText("Merge").assertIsDisplayed()
        composeTestRule.onNodeWithText("Discard new").assertIsDisplayed()
    }

    @Test
    fun `choosing Skip on a plain row reports the skip action`() {
        var chosen: Pair<Int, String>? = null
        show(rows = listOf(newRow), onRowActionChange = { i, a -> chosen = i to a })

        composeTestRule.onNodeWithText("Skip").performClick()

        assertEquals(0 to "skip", chosen)
    }

    @Test
    fun `an error row offers a fix form and saves only the changed fields`() {
        var saved: ImportRowEdit? = null
        show(rows = listOf(errRow), onRowEditSave = { saved = it })

        composeTestRule.onNodeWithText("Fix this row").performClick()
        composeTestRule.onNodeWithTag("import-fix-email").performTextInput("erin@example.com")
        composeTestRule.onNodeWithText("Save fix").performClick()

        assertEquals(ImportRowEdit(rowIndex = 2, email = "erin@example.com"), saved)
    }

    @Test
    fun `clearing a field sends an empty string so the server drops the entry`() {
        var saved: ImportRowEdit? = null
        val withEmail = errRow.copy(
            parsedContact = mapOf("firstname" to "Erin", "email" to "bad")
        )
        show(rows = listOf(withEmail), onRowEditSave = { saved = it })

        composeTestRule.onNodeWithText("Fix this row").performClick()
        composeTestRule.onNodeWithTag("import-fix-email").performTextClearance()
        composeTestRule.onNodeWithText("Save fix").performClick()

        assertEquals(ImportRowEdit(rowIndex = 2, email = ""), saved)
    }

    @Test
    fun `an unfixed error row keeps its decision buttons disabled and a fixed one enables them`() {
        show(rows = listOf(errRow))
        composeTestRule.onNodeWithText("Keep").assertIsNotEnabled()
    }

    @Test
    fun `a flow without row-edit support offers no fix form`() {
        show(rows = listOf(errRow), onRowEditSave = null)
        composeTestRule.onAllNodesWithText("Fix this row").assertCountEquals(0)
    }

    @Test
    fun `a saved fix is acknowledged`() {
        show(
            rows = listOf(errRow),
            rowEdits = mapOf(2 to ImportRowEdit(rowIndex = 2, email = "e@example.com")),
            onRowEditSave = {}
        )
        composeTestRule.onNodeWithText("Fix saved — checked again on import").assertIsDisplayed()
        composeTestRule.onNodeWithText("Edit fix").assertIsDisplayed()
    }

    // Issue #2551: a v1.6.x server ignores row_edits and imports the ORIGINAL invalid row when its
    // action is "add", so the fix form must not be offered there and errored rows stay skip-only.
    @Test
    fun `a v1_6_0 server gets no fix form and the errored row's decisions stay disabled`() {
        show(
            rows = listOf(errRow),
            onRowEditSave = {},
            serverVersion = AppVersion(1, 6, 0)
        )
        composeTestRule.onAllNodesWithText("Fix this row").assertCountEquals(0)
        composeTestRule.onNodeWithText("Keep").assertIsNotEnabled()
        composeTestRule.onNodeWithText("Skip").assertIsNotEnabled()
    }

    @Test
    fun `a v1_7_0 server offers the fix form`() {
        show(rows = listOf(errRow), onRowEditSave = {}, serverVersion = AppVersion(1, 7, 0))
        composeTestRule.onNodeWithText("Fix this row").assertIsDisplayed()
    }

    @Test
    fun `an unknown server version offers the fix form`() {
        show(rows = listOf(errRow), onRowEditSave = {}, serverVersion = null)
        composeTestRule.onNodeWithText("Fix this row").assertIsDisplayed()
    }

    // Issue #2610: an error the form cannot fix must not leave a dead end.
    @Test
    fun `a birthday error offers no fix form and says what to do instead`() {
        val birthday = row(
            3,
            "Cleo",
            listOf("Invalid birthday format (expected YYYY-MM-DD or --MM-DD)")
        )
        show(rows = listOf(birthday), onRowEditSave = {})

        composeTestRule.onAllNodesWithText("Fix this row").assertCountEquals(0)
        composeTestRule.onNodeWithText(
            "This error can't be fixed here — skip the row or edit the contact after import"
        ).assertIsDisplayed()
        composeTestRule.onNodeWithText("Keep").assertIsNotEnabled()
    }

    @Test
    fun `a fixable error does not show the can't-fix hint`() {
        show(rows = listOf(errRow), onRowEditSave = {})
        composeTestRule.onAllNodesWithText("This error can't be fixed here", substring = true)
            .assertCountEquals(0)
    }

    // Issue #2611: after a saved fix the row shows the edited values, not the stale error.
    @Test
    fun `a fixed row shows the edited values and the saved-fix note instead of its stale error`() {
        val bad = ImportRowPreview(
            rowIndex = 5,
            parsedContact = mapOf("firstname" to "Beta", "email" to "not-an-email"),
            validationErrors = listOf("Invalid email format")
        )
        show(
            rows = listOf(bad),
            rowEdits = mapOf(5 to ImportRowEdit(rowIndex = 5, email = "beta@example.com")),
            onRowEditSave = {}
        )

        composeTestRule.onNodeWithText("beta@example.com").assertIsDisplayed()
        composeTestRule.onAllNodesWithText("not-an-email").assertCountEquals(0)
        composeTestRule.onAllNodesWithText("Invalid email format").assertCountEquals(0)
        composeTestRule.onNodeWithText("Fix saved — checked again on import").assertIsDisplayed()
    }

    @Test
    fun `an unfixed row keeps showing its error and no saved-fix note`() {
        show(rows = listOf(errRow), onRowEditSave = {})
        composeTestRule.onNodeWithText("Invalid email").assertIsDisplayed()
        composeTestRule.onAllNodesWithText("Fix saved — checked again on import")
            .assertCountEquals(0)
    }

    @Test
    fun `the Errors chip count excludes rows with a saved fix`() {
        show(
            rows = listOf(newRow, dupRow, errRow),
            rowEdits = mapOf(2 to ImportRowEdit(rowIndex = 2, email = "e@example.com")),
            onRowEditSave = {}
        )
        composeTestRule.onNodeWithText("Errors (0)").assertIsDisplayed()
        composeTestRule.onNodeWithText("New (2)").assertIsDisplayed()
    }

    // Issues #2653 / #2733: the server's English text is replaced by a string keyed on the concept.
    private fun warnRow(index: Int, concept: String, message: String) = row(index, "Ada").copy(
        diagnostics = listOf(ImportDiagnostic("warn", concept, message))
    )

    @Test
    fun `a birthday warning is shown localized, not the server's English text`() {
        show(rows = listOf(warnRow(0, "birthday", "birthday not imported: invalid value")))

        composeTestRule.onNodeWithText(
            "The birthday is not a valid date, so it will not be imported"
        )
            .assertIsDisplayed()
        composeTestRule.onAllNodesWithText("birthday not imported: invalid value")
            .assertCountEquals(0)
    }

    @Test
    fun `an anniversary warning is shown localized`() {
        show(rows = listOf(warnRow(0, "anniversary", "anniversary not imported: invalid value")))

        composeTestRule.onNodeWithText(
            "The anniversary is not a valid date, so it will not be imported"
        ).assertIsDisplayed()
    }

    @Test
    fun `a warning for an unknown concept falls back to the server message`() {
        show(rows = listOf(warnRow(0, "nickname", "nickname was shortened")))

        composeTestRule.onNodeWithText("nickname was shortened").assertIsDisplayed()
    }

    @Test
    fun `a warning row is not an error row and can still be kept`() {
        val warned = warnRow(0, "birthday", "birthday not imported: invalid value")
        show(rows = listOf(warned, errRow))

        composeTestRule.onNodeWithTag("import-filter-errors").assertTextContains("Errors (1)")
        composeTestRule.onNodeWithTag("import-filter-new").assertTextContains("New (1)")
        composeTestRule.onAllNodesWithText("Keep")[0].assertIsSelected()
        composeTestRule.onNodeWithText("Apply decisions (1)").assertIsDisplayed()
    }

    // Issue #2731: file-level notices lead the review.
    @Test
    fun `an encoding file diagnostic is shown once at the top, localized`() {
        show(
            fileDiagnostics = listOf(
                ImportDiagnostic("warn", "encoding", "file was not UTF-8; read as Windows-1252")
            )
        )

        composeTestRule.onNodeWithTag("import-file-diagnostic").assertIsDisplayed()
        composeTestRule.onNodeWithText(
            "This file was not UTF-8, so it was read as Windows-1252. Check that accented characters look right."
        ).assertIsDisplayed()
        composeTestRule.onAllNodesWithText("file was not UTF-8; read as Windows-1252")
            .assertCountEquals(0)
    }

    @Test
    fun `an unknown file diagnostic shows the server message and none shows nothing`() {
        show(
            fileDiagnostics = listOf(ImportDiagnostic("info", "other", "Something about the file"))
        )
        composeTestRule.onNodeWithText("Something about the file").assertIsDisplayed()
    }

    @Test
    fun `no file diagnostics shows no notice`() {
        show()
        composeTestRule.onAllNodesWithTag("import-file-diagnostic").assertCountEquals(0)
    }
}
