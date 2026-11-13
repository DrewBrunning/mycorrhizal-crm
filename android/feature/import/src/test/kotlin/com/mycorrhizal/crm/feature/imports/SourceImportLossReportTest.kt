package com.mycorrhizal.crm.feature.imports

import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithTag
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.network.SourceImportIssue
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

private fun issue(category: String, record: String, message: String, field: String = "") =
    SourceImportIssue(record = record, field = field, category = category, message = message)

/** Issue #2524: the loss report lists issues grouped by category in web's order. */
class LossReportGroupingTest {

    @Test
    fun `groups come out most consequential first regardless of input order`() {
        val groups = groupLossReport(
            listOf(
                issue("skipped", "a", "1"),
                issue("lossy", "b", "2"),
                issue("unsupported", "c", "3"),
                issue("transformed", "d", "4"),
                issue("invalid", "e", "5")
            )
        )
        assertEquals(
            listOf("unsupported", "invalid", "lossy", "transformed", "skipped"),
            groups.map { it.category }
        )
    }

    @Test
    fun `issues keep their input order inside a group and empty groups are omitted`() {
        val groups = groupLossReport(
            listOf(
                issue("lossy", "second-in-input", "x"),
                issue("invalid", "only", "y"),
                issue("lossy", "third-in-input", "z")
            )
        )
        assertEquals(listOf("invalid", "lossy"), groups.map { it.category })
        assertEquals(
            listOf("second-in-input", "third-in-input"),
            groups[1].issues.map { it.record }
        )
    }

    @Test
    fun `an unrecognised category is kept after the known ones rather than hidden`() {
        val groups = groupLossReport(
            listOf(issue("brand-new", "a", "m"), issue("skipped", "b", "n"))
        )
        assertEquals(listOf("skipped", "brand-new"), groups.map { it.category })
    }

    @Test
    fun `no issues means no groups`() {
        assertEquals(emptyList<LossGroup>(), groupLossReport(emptyList()))
    }
}

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class SourceImportLossReportTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val issues = listOf(
        issue("skipped", "Grace", "Skipped note", field = ""),
        issue("lossy", "Ada", "Photo dropped", field = "photo"),
        issue("lossy", "Alan", "Nickname truncated", field = "nickname"),
        issue("unsupported", "Edsger", "Custom field unsupported", field = "custom")
    )

    private fun setContent() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                SourceImportLossReport(issues = issues, headerText = "4 items", tagPrefix = "t")
            }
        }
    }

    @Test
    fun `categories are collapsed initially, show localized label and count, in order`() {
        setContent()
        composeTestRule.onNodeWithTag("t-loss").assertIsDisplayed()
        composeTestRule.onNodeWithTag("t-loss-group-unsupported").assertIsDisplayed()
        composeTestRule.onNodeWithTag("t-loss-group-lossy").assertIsDisplayed()
        composeTestRule.onNodeWithTag("t-loss-group-skipped").assertIsDisplayed()
        composeTestRule.onNodeWithTag("t-loss-group-invalid").assertDoesNotExist()
        composeTestRule.onNodeWithTag("t-loss-group-lossy")
            .assert(hasText("Imported with some detail lost", substring = true))
            .assert(hasText("2", substring = true))
        composeTestRule.onNodeWithText("Photo dropped").assertDoesNotExist()
    }

    @Test
    fun `each category header announces whether it is expanded or collapsed`() {
        setContent()
        val lossy = composeTestRule.onNodeWithTag("t-loss-group-lossy")
        val skipped = composeTestRule.onNodeWithTag("t-loss-group-skipped")
        lossy.assert(SemanticsMatcher.expectValue(SemanticsProperties.Role, Role.Button))
        lossy.assert(
            SemanticsMatcher.expectValue(SemanticsProperties.StateDescription, "Collapsed")
        )
        skipped.assert(
            SemanticsMatcher.expectValue(SemanticsProperties.StateDescription, "Collapsed")
        )

        lossy.performClick()

        lossy.assert(SemanticsMatcher.expectValue(SemanticsProperties.StateDescription, "Expanded"))
        // Only the tapped header changes.
        skipped.assert(
            SemanticsMatcher.expectValue(SemanticsProperties.StateDescription, "Collapsed")
        )

        lossy.performClick()
        lossy.assert(
            SemanticsMatcher.expectValue(SemanticsProperties.StateDescription, "Collapsed")
        )
    }

    @Test
    fun `expanding a category shows record, field and message only for that category`() {
        setContent()
        composeTestRule.onNodeWithTag("t-loss-group-lossy").performClick()
        composeTestRule.onNodeWithText("Photo dropped").assertIsDisplayed()
        composeTestRule.onNodeWithText("Ada · photo").assertIsDisplayed()
        composeTestRule.onNodeWithText("Nickname truncated").assertIsDisplayed()
        composeTestRule.onNodeWithText("Custom field unsupported").assertDoesNotExist()
        composeTestRule.onNodeWithText("Skipped note").assertDoesNotExist()
    }

    @Test
    fun `an issue without a field shows just the record and a second tap collapses`() {
        setContent()
        composeTestRule.onNodeWithTag("t-loss-group-skipped").performClick()
        composeTestRule.onNodeWithText("Grace").assertIsDisplayed()
        composeTestRule.onNodeWithTag("t-loss-group-skipped").performClick()
        composeTestRule.onNodeWithText("Skipped note").assertDoesNotExist()
        assertEquals(
            0,
            composeTestRule.onAllNodesWithTag("t-loss-item-skipped-0").fetchSemanticsNodes().size
        )
    }
}
