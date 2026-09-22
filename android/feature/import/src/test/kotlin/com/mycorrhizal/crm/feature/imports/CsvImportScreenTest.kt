package com.mycorrhizal.crm.feature.imports

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.network.ColumnMapping
import com.mycorrhizal.crm.model.network.ImportPreviewResponse
import com.mycorrhizal.crm.model.network.ImportResult
import com.mycorrhizal.crm.model.network.ImportRowPreview
import com.mycorrhizal.crm.model.network.ImportUploadResponse
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

// Issue #834: "reachability" proof for the CSV import screen — the real CsvImportScreenContent
// (not a placeholder) renders each step of the flow, mirrors VcfImportScreenTest.
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class CsvImportScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(
        uiState: CsvImportUiState,
        onColumnFieldChange: (String, String) -> Unit = { _, _ -> },
        onSubmitMapping: () -> Unit = {},
        onRowActionChange: (Int, String) -> Unit = { _, _ -> },
        onConfirm: () -> Unit = {},
        onDone: () -> Unit = {},
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                CsvImportScreenContent(
                    uiState = uiState,
                    onColumnFieldChange = onColumnFieldChange,
                    onSubmitMapping = onSubmitMapping,
                    onRowActionChange = onRowActionChange,
                    onConfirm = onConfirm,
                    onDone = onDone,
                )
            }
        }
    }

    @Test
    fun `pick step renders the picker button, not a placeholder`() {
        setContent(CsvImportUiState(step = CsvImportStep.PICK))
        composeTestRule.onNodeWithText("Select a .csv file").assertIsDisplayed()
    }

    @Test
    fun `mapping step renders each column header and its suggested field`() {
        setContent(
            CsvImportUiState(
                step = CsvImportStep.MAPPING,
                upload = ImportUploadResponse(
                    sessionId = "session-1",
                    headers = listOf("Full Name", "Email Address"),
                    suggestedMappings = listOf(
                        ColumnMapping(csvColumn = "Full Name", contactField = "firstname"),
                        ColumnMapping(csvColumn = "Email Address", contactField = "email"),
                    ),
                    rowCount = 5,
                    sampleData = listOf(listOf("Dana White", "dana@example.com")),
                ),
                columnFields = mapOf("Full Name" to "firstname", "Email Address" to "email"),
            ),
        )
        composeTestRule.onNodeWithText("Full Name").assertIsDisplayed()
        composeTestRule.onNodeWithText("Email Address").assertIsDisplayed()
        composeTestRule.onNodeWithText("Dana White").assertIsDisplayed()
        composeTestRule.onNodeWithTag("csv-mapping-continue").assertIsDisplayed()
    }

    @Test
    fun `an unmapped column defaults to the ignore label`() {
        setContent(
            CsvImportUiState(
                step = CsvImportStep.MAPPING,
                upload = ImportUploadResponse(
                    sessionId = "session-1",
                    headers = listOf("Notes"),
                    suggestedMappings = listOf(ColumnMapping(csvColumn = "Notes", contactField = "")),
                    rowCount = 1,
                ),
                columnFields = mapOf("Notes" to ""),
            ),
        )
        composeTestRule.onNodeWithText("-- Ignore --").assertIsDisplayed()
    }

    @Test
    fun `tapping continue on the mapping step invokes the submit-mapping callback`() {
        var submitted = false
        setContent(
            CsvImportUiState(
                step = CsvImportStep.MAPPING,
                upload = ImportUploadResponse(sessionId = "session-1", headers = listOf("Name"), rowCount = 1),
                columnFields = mapOf("Name" to "firstname"),
            ),
            onSubmitMapping = { submitted = true },
        )
        composeTestRule.onNodeWithTag("csv-mapping-continue").performClick()
        assertEquals(true, submitted)
    }

    @Test
    fun `preview step renders each row's content, reusing the shared review step`() {
        setContent(
            CsvImportUiState(
                step = CsvImportStep.PREVIEW,
                preview = ImportPreviewResponse(
                    sessionId = "session-1",
                    rows = listOf(
                        ImportRowPreview(rowIndex = 0, parsedContact = mapOf("firstname" to "Dana", "lastname" to "White"), suggestedAction = "add"),
                    ),
                ),
                rowActions = mapOf(0 to "add"),
            ),
        )
        composeTestRule.onNodeWithText("Dana White").assertIsDisplayed()
        composeTestRule.onNodeWithText("Apply decisions (1)").assertIsDisplayed()
    }

    @Test
    fun `tapping confirm import invokes the confirm callback`() {
        var confirmed = false
        setContent(
            CsvImportUiState(
                step = CsvImportStep.PREVIEW,
                preview = ImportPreviewResponse(sessionId = "session-1", rows = listOf(ImportRowPreview(rowIndex = 0))),
                rowActions = mapOf(0 to "add"),
            ),
            onConfirm = { confirmed = true },
        )
        composeTestRule.onNodeWithText("Apply decisions (1)").performClick()
        assertEquals(true, confirmed)
    }

    @Test
    fun `result step renders the created updated skipped counts`() {
        setContent(
            CsvImportUiState(step = CsvImportStep.RESULT, result = ImportResult(totalProcessed = 3, created = 2, updated = 1, skipped = 0)),
        )
        composeTestRule.onNodeWithText("Import complete: 2 created, 1 updated, 0 skipped.").assertIsDisplayed()
    }

    @Test
    fun `tapping done after the result invokes the done callback`() {
        var done = false
        setContent(
            CsvImportUiState(step = CsvImportStep.RESULT, result = ImportResult(totalProcessed = 1, created = 1)),
            onDone = { done = true },
        )
        composeTestRule.onNodeWithText("Confirm").performClick()
        assertEquals(true, done)
    }

    @Test
    fun `shows a loading skeleton while loading`() {
        setContent(CsvImportUiState(isLoading = true))
        composeTestRule.onNodeWithTag("csv-import-loading").assertIsDisplayed()
    }
}
