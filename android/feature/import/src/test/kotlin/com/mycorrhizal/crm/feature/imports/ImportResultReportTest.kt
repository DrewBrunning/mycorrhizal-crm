package com.mycorrhizal.crm.feature.imports

import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.network.ImportResult
import com.mycorrhizal.crm.model.network.ImportRowIssue
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
}
