package com.mycorrhizal.crm.feature.imports

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import com.mycorrhizal.crm.model.network.ImportRun
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.mockk
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

    private fun setScreen(apiClient: ApiClient) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ImportHistoryScreen(onBack = {}, viewModel = ImportHistoryViewModel(apiClient))
            }
        }
    }

    @Test
    fun `renders each run's format and counts`() {
        val apiClient = mockk<ApiClient>()
        coEvery { apiClient.getImportHistory() } returns Result.success(
            listOf(
                ImportRun(id = 1, format = "csv", created = 3, updated = 1, skipped = 0, errorCount = 0, createdAt = "2026-09-20T12:00:00Z"),
            ),
        )

        setScreen(apiClient)

        composeTestRule.onNodeWithTag("import-history-row-1").assertIsDisplayed()
        composeTestRule.onNodeWithText("CSV").assertIsDisplayed()
    }

    @Test
    fun `renders the created updated skipped and error counts`() {
        val apiClient = mockk<ApiClient>()
        coEvery { apiClient.getImportHistory() } returns Result.success(
            listOf(ImportRun(id = 1, format = "vcf", created = 3, updated = 1, skipped = 2, errorCount = 1)),
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

        composeTestRule.onNodeWithText("No imports yet.").assertIsDisplayed()
    }

    @Test
    fun `a load failure surfaces the error as a snackbar without crashing`() {
        val apiClient = mockk<ApiClient>()
        coEvery { apiClient.getImportHistory() } returns Result.failure(ApiError.Client(500, "server error"))

        setScreen(apiClient)

        composeTestRule.onNodeWithText("server error").assertIsDisplayed()
    }
}
