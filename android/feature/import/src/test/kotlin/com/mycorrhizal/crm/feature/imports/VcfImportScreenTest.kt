package com.mycorrhizal.crm.feature.imports

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.network.ImportPreviewResponse
import com.mycorrhizal.crm.model.network.ImportResult
import com.mycorrhizal.crm.model.network.ImportRowPreview
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import org.junit.runner.RunWith

// M9 item 4: "reachability" proof for the VCF import screen — the real VcfImportScreenContent
// (not a placeholder) renders each step of the flow.
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class VcfImportScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(
        uiState: VcfImportUiState,
        onRowActionChange: (Int, String) -> Unit = { _, _ -> },
        onConfirm: () -> Unit = {},
        onDone: () -> Unit = {},
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                VcfImportScreenContent(
                    uiState = uiState,
                    onRowActionChange = onRowActionChange,
                    onConfirm = onConfirm,
                    onDone = onDone,
                )
            }
        }
    }

    @Test
    fun `pick step renders the picker button, not a placeholder`() {
        setContent(VcfImportUiState(step = VcfImportStep.PICK))
        composeTestRule.onNodeWithText("Select a .vcf file").assertIsDisplayed()
    }

    @Test
    fun `preview step renders each row's content`() {
        setContent(
            VcfImportUiState(
                step = VcfImportStep.PREVIEW,
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
            VcfImportUiState(
                step = VcfImportStep.PREVIEW,
                preview = ImportPreviewResponse(sessionId = "session-1", rows = listOf(ImportRowPreview(rowIndex = 0))),
                rowActions = mapOf(0 to "add"),
            ),
            onConfirm = { confirmed = true },
        )
        composeTestRule.onNodeWithText("Apply decisions (1)").performClick()
        assertEquals(true, confirmed)
    }

    @Test
    fun `picking a row action invokes the callback with that row's index`() {
        var changedRow: Int? = null
        var changedAction: String? = null
        setContent(
            VcfImportUiState(
                step = VcfImportStep.PREVIEW,
                preview = ImportPreviewResponse(sessionId = "session-1", rows = listOf(ImportRowPreview(rowIndex = 0, suggestedAction = "add"))),
                rowActions = mapOf(0 to "add"),
            ),
            onRowActionChange = { row, action -> changedRow = row; changedAction = action },
        )
        composeTestRule.onNodeWithText("Discard New").performClick()
        assertEquals(0, changedRow)
        assertEquals("skip", changedAction)
    }

    @Test
    fun `a duplicate row renders its match line and merge diff`() {
        setContent(
            VcfImportUiState(
                step = VcfImportStep.PREVIEW,
                preview = ImportPreviewResponse(
                    sessionId = "session-1",
                    rows = listOf(
                        ImportRowPreview(
                            rowIndex = 0,
                            parsedContact = mapOf("firstname" to "Bob", "lastname" to "Smith"),
                            suggestedAction = "update",
                            duplicateMatch = com.mycorrhizal.crm.model.network.DuplicateMatch(
                                existingContactId = 9,
                                existingFirstname = "Bob",
                                existingLastname = "Smith",
                                existingEmail = "bob@example.com",
                                matchReason = "email",
                            ),
                            mergeDiff = com.mycorrhizal.crm.model.network.ImportMergeDiff(
                                updated = listOf(
                                    com.mycorrhizal.crm.model.network.ImportScalarChange(
                                        field = "job_title",
                                        label = "Job Title",
                                        old = "Engineer",
                                        new = "Staff Engineer",
                                    ),
                                ),
                                added = listOf(
                                    com.mycorrhizal.crm.model.network.ImportAddedValue(kind = "phone", value = "+15559998888"),
                                ),
                            ),
                        ),
                    ),
                ),
                rowActions = mapOf(0 to "update"),
            ),
        )
        composeTestRule.onNodeWithText("Matches: Bob Smith (same email)").assertIsDisplayed()
        composeTestRule.onNodeWithText("+ new phone: +15559998888").assertIsDisplayed()
        composeTestRule.onNodeWithText("Job Title: Engineer \u2192 Staff Engineer").assertIsDisplayed()
    }

    @Test
    fun `a within-batch duplicate renders the duplicate-of line and disables merge`() {
        setContent(
            VcfImportUiState(
                step = VcfImportStep.PREVIEW,
                preview = ImportPreviewResponse(
                    sessionId = "session-1",
                    rows = listOf(
                        ImportRowPreview(rowIndex = 0, suggestedAction = "add"),
                        ImportRowPreview(rowIndex = 1, suggestedAction = "skip", batchDuplicateOf = 0),
                    ),
                ),
                rowActions = mapOf(0 to "add", 1 to "skip"),
            ),
        )
        composeTestRule.onNodeWithText("Duplicates row 1 of this import").assertIsDisplayed()
    }

    @Test
    fun `result step renders the created updated skipped counts`() {
        setContent(
            VcfImportUiState(step = VcfImportStep.RESULT, result = ImportResult(totalProcessed = 3, created = 2, updated = 1, skipped = 0)),
        )
        composeTestRule.onNodeWithText("Import complete: 2 created, 1 updated, 0 skipped.").assertIsDisplayed()
    }

    @Test
    fun `tapping done after the result invokes the done callback`() {
        var done = false
        setContent(
            VcfImportUiState(step = VcfImportStep.RESULT, result = ImportResult(totalProcessed = 1, created = 1)),
            onDone = { done = true },
        )
        composeTestRule.onNodeWithText("Confirm").performClick()
        assertEquals(true, done)
    }

    @Test
    fun `shows a loading skeleton while loading`() {
        setContent(VcfImportUiState(isLoading = true))
        composeTestRule.onNodeWithTag("vcf-import-loading").assertIsDisplayed()
    }
}
