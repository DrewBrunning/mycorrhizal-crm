package com.mycorrhizal.crm.feature.imports

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.AppVersion
import com.mycorrhizal.crm.model.network.ImportRowIssue
import com.mycorrhizal.crm.model.network.ImportRun
import com.mycorrhizal.crm.model.network.ImportRunDetail
import com.mycorrhizal.crm.model.util.DateFormat
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.ui.LocalDateFormat
import com.mycorrhizal.crm.ui.LocalIs24Hour
import com.mycorrhizal.crm.ui.LocalServerVersion
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.mockk
import java.util.TimeZone
import org.junit.After
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issue #834 (web parity, issue #651): mounts the real top-level
 * [ImportHistoryScreen] against an [ImportHistoryViewModel] constructed
 * directly with a mocked [ApiClient] — same construction style as
 * `ImportContactsScreenTest` (the screen accepts an explicit `viewModel`,
 * bypassing Hilt in tests).
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ImportHistoryScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val originalZone = TimeZone.getDefault()

    @Before
    fun pinZone() = TimeZone.setDefault(TimeZone.getTimeZone("UTC"))

    @After
    fun restoreZone() = TimeZone.setDefault(originalZone)

    private fun setScreen(
        apiClient: ApiClient,
        serverVersion: AppVersion? = null,
        dateFormat: String = DateFormat.EU,
        is24Hour: Boolean = true
    ) {
        composeTestRule.setContent {
            CompositionLocalProvider(
                LocalServerVersion provides serverVersion,
                LocalDateFormat provides dateFormat,
                LocalIs24Hour provides is24Hour
            ) {
                MycorrhizalTheme {
                    ImportHistoryScreen(onBack = {}, viewModel = ImportHistoryViewModel(apiClient))
                }
            }
        }
    }

    @Test
    fun `renders each run's format and counts`() {
        val apiClient = mockk<ApiClient>()
        coEvery { apiClient.getImportHistory() } returns Result.success(
            listOf(
                ImportRun(
                    id = 1,
                    format = "csv",
                    created = 3,
                    updated = 1,
                    skipped = 0,
                    errorCount = 0,
                    createdAt = "2026-09-20T12:00:00Z"
                )
            )
        )

        setScreen(apiClient)

        composeTestRule.onNodeWithTag("import-history-row-1").assertIsDisplayed()
        composeTestRule.onNodeWithText("CSV").assertIsDisplayed()
    }

    // Issue #2618: the run time follows the user's date format and hour cycle, with no seconds.
    @Test
    fun `a run's time follows the date format preference and shows no seconds`() {
        val apiClient = mockk<ApiClient>()
        coEvery { apiClient.getImportHistory() } returns Result.success(
            listOf(ImportRun(id = 1, format = "csv", createdAt = "2026-09-20T13:05:42Z"))
        )

        setScreen(apiClient, dateFormat = DateFormat.EU, is24Hour = true)

        composeTestRule.onNodeWithText("20.09.2026 13:05").assertIsDisplayed()
    }

    @Test
    fun `a run's time honours the iso date format and the 12-hour clock`() {
        val apiClient = mockk<ApiClient>()
        coEvery { apiClient.getImportHistory() } returns Result.success(
            listOf(ImportRun(id = 1, format = "csv", createdAt = "2026-09-20T13:05:42Z"))
        )

        setScreen(apiClient, dateFormat = DateFormat.ISO, is24Hour = false)

        composeTestRule.onNodeWithText("2026-09-20 1:05", substring = true).assertIsDisplayed()
    }

    @Test
    fun `renders the created updated skipped and error counts`() {
        val apiClient = mockk<ApiClient>()
        coEvery { apiClient.getImportHistory() } returns Result.success(
            listOf(
                ImportRun(
                    id = 1,
                    format = "vcf",
                    created = 3,
                    updated = 1,
                    skipped = 2,
                    errorCount = 1
                )
            )
        )

        setScreen(apiClient)

        composeTestRule.onNodeWithText("Created: 3").assertIsDisplayed()
        composeTestRule.onNodeWithText("Updated: 1").assertIsDisplayed()
        composeTestRule.onNodeWithText("Skipped: 2").assertIsDisplayed()
        composeTestRule.onNodeWithText("Errors: 1").assertIsDisplayed()
    }

    @Test
    fun `renders the empty state when there is no history`() {
        val apiClient = mockk<ApiClient>()
        coEvery { apiClient.getImportHistory() } returns Result.success(emptyList())

        setScreen(apiClient)

        composeTestRule.onNodeWithText("No imports yet").assertIsDisplayed()
    }

    @Test
    fun `a load failure surfaces the error as a snackbar without crashing`() {
        val apiClient = mockk<ApiClient>()
        coEvery { apiClient.getImportHistory() } returns Result.failure(
            ApiError.Client(500, "server error")
        )

        setScreen(apiClient)

        composeTestRule.onNodeWithText("server error").assertIsDisplayed()
    }

    // Issue #2464/#2210: a run with stored issues opens its itemized report.
    @Test
    fun `the View report action shows only for a run with issues`() {
        val apiClient = mockk<ApiClient>()
        coEvery { apiClient.getImportHistory() } returns Result.success(
            listOf(
                ImportRun(id = 1, format = "csv", skipped = 2, issueCount = 2),
                ImportRun(id = 2, format = "vcf", created = 3, issueCount = 0)
            )
        )

        setScreen(apiClient)

        composeTestRule.onAllNodesWithText("View report").assertCountEquals(1)
        composeTestRule.onNodeWithTag("import-history-report-1").assertIsDisplayed()
    }

    @Test
    fun `View report opens the itemized report with each reason`() {
        val apiClient = mockk<ApiClient>()
        coEvery { apiClient.getImportHistory() } returns Result.success(
            listOf(ImportRun(id = 1, format = "csv", skipped = 1, issueCount = 1))
        )
        coEvery { apiClient.getImportRun(1) } returns Result.success(
            ImportRunDetail(
                id = 1,
                format = "csv",
                skipped = 1,
                issueCount = 1,
                issues = listOf(
                    ImportRowIssue(
                        row = 5,
                        name = "Erin",
                        status = "skipped",
                        reason = "Invalid email"
                    )
                )
            )
        )
        setScreen(apiClient)

        composeTestRule.onNodeWithText("View report").performClick()

        composeTestRule.onNodeWithText("Row 5 · Erin").assertIsDisplayed()
        composeTestRule.onNodeWithText("Skipped: Invalid email").assertIsDisplayed()
        composeTestRule.onNodeWithText("Share as CSV").assertIsDisplayed()
    }

    // Issue #2621: viewing a past report is not confirming anything; the button closes it.
    @Test
    fun `the history report's button says Close and returns to the list`() {
        val apiClient = mockk<ApiClient>()
        coEvery { apiClient.getImportHistory() } returns Result.success(
            listOf(ImportRun(id = 1, format = "csv", skipped = 1, issueCount = 1))
        )
        coEvery { apiClient.getImportRun(1) } returns Result.success(
            ImportRunDetail(
                id = 1,
                format = "csv",
                skipped = 1,
                issueCount = 1,
                issues = listOf(ImportRowIssue(row = 5, name = "Erin", status = "skipped"))
            )
        )
        setScreen(apiClient)
        composeTestRule.onNodeWithText("View report").performClick()

        composeTestRule.onAllNodesWithText("Confirm").assertCountEquals(0)
        composeTestRule.onNodeWithText("Close").performClick()

        composeTestRule.onNodeWithTag("import-history-list").assertIsDisplayed()
    }

    @Test
    fun `a report load failure shows the error and stays on the list`() {
        val apiClient = mockk<ApiClient>()
        coEvery { apiClient.getImportHistory() } returns Result.success(
            listOf(ImportRun(id = 1, format = "csv", skipped = 1, issueCount = 1))
        )
        coEvery { apiClient.getImportRun(1) } returns Result.failure(
            ApiError.Client(500, "server error")
        )
        setScreen(apiClient)

        composeTestRule.onNodeWithText("View report").performClick()

        composeTestRule.onNodeWithText("server error").assertIsDisplayed()
        composeTestRule.onNodeWithTag("import-history-list").assertIsDisplayed()
    }

    // Issue #2527: the per-row report (`GET /contacts/import/history/:id`, `issue_count`) needs a
    // v1.7.0 server, so the link is hidden on an older one even if a row claims issues.
    @Test
    fun `a v1_6_0 server offers no View report link`() {
        val apiClient = mockk<ApiClient>()
        coEvery { apiClient.getImportHistory() } returns Result.success(
            listOf(ImportRun(id = 1, format = "csv", skipped = 2, issueCount = 2))
        )

        setScreen(apiClient, serverVersion = AppVersion(1, 6, 0))

        composeTestRule.onNodeWithText("csv", substring = true, ignoreCase = true).assertExists()
        composeTestRule.onAllNodesWithText("View report").assertCountEquals(0)
    }

    @Test
    fun `a v1_7_0 server offers the View report link`() {
        val apiClient = mockk<ApiClient>()
        coEvery { apiClient.getImportHistory() } returns Result.success(
            listOf(ImportRun(id = 1, format = "csv", skipped = 2, issueCount = 2))
        )

        setScreen(apiClient, serverVersion = AppVersion(1, 7, 0))

        composeTestRule.onAllNodesWithText("View report").assertCountEquals(1)
    }
}
