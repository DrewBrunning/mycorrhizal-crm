package com.mycorrhizal.crm.feature.imports

import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.network.ImportResult
import com.mycorrhizal.crm.model.network.ImportRowIssue
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

// Issue #2187 (Android parity, #2210): the itemized final report.
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ImportResultReportTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val issues = listOf(
        ImportRowIssue(3, "Erin", "e@x.io", "", "skipped", "Invalid email"),
        ImportRowIssue(7, "", "", "", "failed", "Could not save")
    )

    private fun show(result: ImportResult, onShare: (List<ImportRowIssue>) -> Unit = {}) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ImportResultReport(result = result, onShareCsv = onShare, onDone = {})
            }
        }
    }

    @Test
    fun `lists each skipped or failed row with its reason`() {
        show(ImportResult(totalProcessed = 5, created = 3, skipped = 2, issues = issues))

        composeTestRule.onNodeWithText("Rows not imported").assertIsDisplayed()
        composeTestRule.onNodeWithText("Row 3 · Erin").assertIsDisplayed()
        composeTestRule.onNodeWithText("Skipped: Invalid email").assertIsDisplayed()
        composeTestRule.onNodeWithText("Row 7 · —").assertIsDisplayed()
        composeTestRule.onNodeWithText("Failed: Could not save").assertIsDisplayed()
    }

    @Test
    fun `the share button hands the issues to the callback`() {
        var shared: List<ImportRowIssue>? = null
        show(ImportResult(skipped = 2, issues = issues), onShare = { shared = it })

        composeTestRule.onNodeWithText("Share as CSV").performClick()

        assertEquals(issues, shared)
    }

    @Test
    fun `a clean import shows no report section`() {
        show(ImportResult(totalProcessed = 2, created = 2))

        composeTestRule.onAllNodesWithText("Rows not imported").assertCountEquals(0)
        composeTestRule.onAllNodesWithText("Share as CSV").assertCountEquals(0)
    }

    // Issue #2621: nothing is pending once the import ran, so the button is "Done", not "Confirm".
    @Test
    fun `the closing button says Done with or without a report`() {
        show(ImportResult(totalProcessed = 2, created = 2))
        composeTestRule.onNodeWithText("Done").assertIsDisplayed()
        composeTestRule.onAllNodesWithText("Confirm").assertCountEquals(0)
    }

    @Test
    fun `the closing button says Done beneath a list of issues too`() {
        show(ImportResult(skipped = 2, issues = issues))
        composeTestRule.onNodeWithText("Done").assertIsDisplayed()
        composeTestRule.onAllNodesWithText("Confirm").assertCountEquals(0)
    }

    // Issue #2746: warning issues are rows that DID import.
    private val warning = ImportRowIssue(
        1,
        "Ada",
        "a@x.io",
        "",
        "warning",
        "birthday not imported: invalid value",
        concept = "birthday"
    )

    @Test
    fun `a warning issue reads as imported with a localized reason, not as skipped`() {
        show(ImportResult(totalProcessed = 1, created = 1, issues = listOf(warning)))

        composeTestRule.onNodeWithText("Row 1 · Ada").assertIsDisplayed()
        composeTestRule.onNodeWithText(
            "Imported, but the birthday is not a valid date so it was left out"
        ).assertIsDisplayed()
        composeTestRule.onAllNodesWithText("birthday not imported", substring = true)
            .assertCountEquals(0)
        composeTestRule.onAllNodesWithText("Skipped", substring = true).assertCountEquals(0)
    }

    @Test
    fun `an anniversary warning is localized too`() {
        show(
            ImportResult(
                created = 1,
                issues = listOf(warning.copy(concept = "anniversary", reason = "x"))
            )
        )

        composeTestRule.onNodeWithText(
            "Imported, but the anniversary is not a valid date so it was left out"
        ).assertIsDisplayed()
    }

    @Test
    fun `a warning with an unknown concept falls back to the server reason`() {
        show(
            ImportResult(
                created = 1,
                issues = listOf(warning.copy(concept = "zzz", reason = "Odd"))
            )
        )

        composeTestRule.onNodeWithText("Imported with a warning: Odd").assertIsDisplayed()
    }

    @Test
    fun `an unknown status is a neutral note, not skipped or failed`() {
        show(
            ImportResult(
                created = 1,
                issues = listOf(ImportRowIssue(2, "Cy", "", "", "quarantined", "Held back"))
            )
        )

        composeTestRule.onNodeWithText("Note: Held back").assertIsDisplayed()
        composeTestRule.onAllNodesWithText("Skipped", substring = true).assertCountEquals(0)
        composeTestRule.onAllNodesWithText("Failed", substring = true).assertCountEquals(0)
    }

    @Test
    fun `the title depends on whether rows were left out`() {
        assertEquals(
            R.string.import_report_title,
            importReportTitleRes(issues)
        )
        assertEquals(
            R.string.import_report_title_warnings,
            importReportTitleRes(listOf(warning))
        )
        assertEquals(
            R.string.import_report_title_mixed,
            importReportTitleRes(issues + warning)
        )
    }

    @Test
    fun `a report with only warnings is titled as imported with warnings`() {
        show(ImportResult(created = 1, issues = listOf(warning)))

        composeTestRule.onNodeWithText("Imported with warnings").assertIsDisplayed()
        composeTestRule.onAllNodesWithText("Rows not imported").assertCountEquals(0)
    }
}
