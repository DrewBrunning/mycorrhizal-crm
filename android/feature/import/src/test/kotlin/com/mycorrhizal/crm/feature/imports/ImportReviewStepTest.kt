package com.mycorrhizal.crm.feature.imports

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertIsSelected
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextClearance
import androidx.compose.ui.test.performTextInput
import com.mycorrhizal.crm.model.AppVersion
import com.mycorrhizal.crm.model.network.DuplicateMatch
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
        serverVersion: AppVersion? = null
    ) {
        composeTestRule.setContent {
            CompositionLocalProvider(LocalServerVersion provides serverVersion) {
                MycorrhizalTheme {
                    ImportReviewStep(
                        rows = rows,
                        rowActions = rows.associate { it.rowIndex to it.suggestedAction },
                        onRowActionChange = onRowActionChange,
                        onResolveAll = {},
                        onConfirm = {},
                        rowEdits = rowEdits,
                        onRowEditSave = onRowEditSave
                    )
                }
            }
        }
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
}
