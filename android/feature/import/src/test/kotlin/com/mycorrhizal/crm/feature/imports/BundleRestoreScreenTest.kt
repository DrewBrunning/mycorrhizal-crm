package com.mycorrhizal.crm.feature.imports

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.network.ImportRowPreview
import com.mycorrhizal.crm.model.network.SourceImportIssue
import com.mycorrhizal.crm.model.network.SourceImportPreviewResponse
import com.mycorrhizal.crm.model.network.SourceImportResult
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/** Issue #1264: each step of the restore flow renders real content (reachability proof). */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class BundleRestoreScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(
        uiState: BundleRestoreUiState,
        onConfirm: () -> Unit = {},
        onDone: () -> Unit = {},
        onBack: () -> Unit = {},
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                BundleRestoreScreenContent(uiState = uiState, onConfirm = onConfirm, onDone = onDone, onBack = onBack)
            }
        }
    }

    private fun reviewState(loss: List<SourceImportIssue> = emptyList()) = BundleRestoreUiState(
        step = BundleRestoreStep.REVIEW,
        preview = SourceImportPreviewResponse(
            sessionId = "s1",
            rows = listOf(
                ImportRowPreview(
                    rowIndex = 0,
                    parsedContact = mapOf("firstname" to "Ada", "lastname" to "Lovelace"),
                    suggestedAction = "add",
                ),
            ),
            totalRows = 1,
            validRows = 1,
            lossReport = loss,
        ),
        rowActions = mapOf(0 to "add"),
    )

    @Test
    fun `pick step offers the file chooser`() {
        setContent(BundleRestoreUiState(step = BundleRestoreStep.PICK))

        composeTestRule.onNodeWithText("Restore from bundle").assertIsDisplayed()
        composeTestRule.onNodeWithTag("bundle-restore-pick").assertIsDisplayed()
        composeTestRule.onNodeWithText("Choose bundle file").assertIsDisplayed()
    }

    @Test
    fun `working step shows the progress message`() {
        setContent(BundleRestoreUiState(step = BundleRestoreStep.WORKING, isImporting = true))

        composeTestRule.onNodeWithTag("bundle-restore-working").assertIsDisplayed()
    }

    @Test
    fun `review step renders the rows and no loss notice when nothing is lost`() {
        setContent(reviewState())

        composeTestRule.onNodeWithText("Ada Lovelace").assertIsDisplayed()
        composeTestRule.onNodeWithTag("bundle-restore-loss").assertDoesNotExist()
    }

    @Test
    fun `review step surfaces the loss report count`() {
        setContent(reviewState(loss = listOf(SourceImportIssue(message = "a"), SourceImportIssue(message = "b"))))

        composeTestRule.onNodeWithText("2 items cannot be restored exactly").assertIsDisplayed()
    }

    @Test
    fun `confirming from the review step invokes the callback`() {
        var confirmed = 0
        setContent(reviewState(), onConfirm = { confirmed++ })

        composeTestRule.onNodeWithText("Apply decisions (1)").performClick()

        assertEquals(1, confirmed)
    }

    @Test
    fun `result step shows the counts and Done finishes`() {
        var done = 0
        setContent(
            BundleRestoreUiState(
                step = BundleRestoreStep.RESULT,
                result = SourceImportResult(created = 5, updated = 1, skipped = 2),
            ),
            onDone = { done++ },
        )

        composeTestRule.onNodeWithText("Restored: 5 created, 1 updated, 2 skipped").assertIsDisplayed()
        composeTestRule.onNodeWithText("Confirm").performClick()

        assertEquals(1, done)
    }
}
