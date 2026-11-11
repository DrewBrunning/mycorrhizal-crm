package com.mycorrhizal.crm.feature.imports

import com.mycorrhizal.crm.model.network.ImportRowIssue
import com.mycorrhizal.crm.model.network.ImportRun
import com.mycorrhizal.crm.model.network.ImportRunDetail
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

// Issue #834 (web parity, issue #651): GET /contacts/import/history existed with zero Android caller.
class ImportHistoryViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val apiClient = mockk<ApiClient>()

    @Test
    fun `loads the history on init, newest first as returned by the server`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { apiClient.getImportHistory() } returns Result.success(
            listOf(
                ImportRun(
                    id = 2,
                    format = "csv",
                    created = 3,
                    updated = 1,
                    skipped = 0,
                    errorCount = 0,
                    createdAt = "2026-09-20T12:00:00Z"
                ),
                ImportRun(
                    id = 1,
                    format = "vcf",
                    created = 1,
                    updated = 0,
                    skipped = 2,
                    errorCount = 1,
                    createdAt = "2026-09-19T12:00:00Z"
                )
            )
        )

        val vm = ImportHistoryViewModel(apiClient)
        advanceUntilIdle()

        val state = vm.uiState.value
        assertEquals(2, state.runs.size)
        assertEquals(2L, state.runs[0].id)
        assertEquals("csv", state.runs[0].format)
        assertNull(state.error)
    }

    @Test
    fun `an empty history is represented as an empty list, not an error`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { apiClient.getImportHistory() } returns Result.success(emptyList())

        val vm = ImportHistoryViewModel(apiClient)
        advanceUntilIdle()

        assertTrue(vm.uiState.value.runs.isEmpty())
        assertNull(vm.uiState.value.error)
    }

    @Test
    fun `a load failure surfaces the error message and leaves the list empty`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { apiClient.getImportHistory() } returns Result.failure(
            ApiError.Client(500, "server error")
        )

        val vm = ImportHistoryViewModel(apiClient)
        advanceUntilIdle()

        assertEquals("server error", vm.uiState.value.error?.message)
        assertTrue(vm.uiState.value.runs.isEmpty())
    }

    @Test
    fun `load can be retried after a failure and clears the previous error`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { apiClient.getImportHistory() } returns Result.failure(
            ApiError.Client(500, "server error")
        )
        val vm = ImportHistoryViewModel(apiClient)
        advanceUntilIdle()
        assertEquals("server error", vm.uiState.value.error?.message)

        coEvery { apiClient.getImportHistory() } returns Result.success(
            listOf(ImportRun(id = 1, format = "csv"))
        )
        vm.load()
        advanceUntilIdle()

        assertNull(vm.uiState.value.error)
        assertEquals(1, vm.uiState.value.runs.size)
    }

    @Test
    fun `onErrorShown clears the error without touching the loaded runs`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { apiClient.getImportHistory() } returns Result.failure(
            ApiError.Client(500, "server error")
        )
        val vm = ImportHistoryViewModel(apiClient)
        advanceUntilIdle()

        vm.onErrorShown()

        assertNull(vm.uiState.value.error)
    }

    // Issue #2464/#2210: open the itemized report of a past import.
    private val detail = ImportRunDetail(
        id = 7,
        format = "csv",
        skipped = 1,
        issueCount = 1,
        issues = listOf(
            ImportRowIssue(row = 2, name = "Erin", status = "skipped", reason = "Invalid email")
        )
    )

    @Test
    fun `openReport loads the run detail by id and exposes its issues`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { apiClient.getImportHistory() } returns Result.success(emptyList())
        coEvery { apiClient.getImportRun(7) } returns Result.success(detail)
        val vm = ImportHistoryViewModel(apiClient)
        advanceUntilIdle()

        vm.openReport(7)
        advanceUntilIdle()

        coVerify { apiClient.getImportRun(7) }
        assertEquals(detail, vm.uiState.value.report)
        assertFalse(vm.uiState.value.isReportLoading)
        assertNull(vm.uiState.value.error)
    }

    @Test
    fun `a report load failure surfaces the error and shows no report`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { apiClient.getImportHistory() } returns Result.success(emptyList())
        coEvery { apiClient.getImportRun(9) } returns Result.failure(
            ApiError.Client(404, "Import run not found")
        )
        val vm = ImportHistoryViewModel(apiClient)
        advanceUntilIdle()

        vm.openReport(9)
        advanceUntilIdle()

        assertEquals("Import run not found", vm.uiState.value.error?.message)
        assertNull(vm.uiState.value.report)
        assertFalse(vm.uiState.value.isReportLoading)
        assertFalse(vm.uiState.value.reportOpen)
    }

    @Test
    fun `closeReport returns to the list`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { apiClient.getImportHistory() } returns Result.success(emptyList())
        coEvery { apiClient.getImportRun(7) } returns Result.success(detail)
        val vm = ImportHistoryViewModel(apiClient)
        advanceUntilIdle()
        vm.openReport(7)
        advanceUntilIdle()

        vm.closeReport()

        assertNull(vm.uiState.value.report)
        assertFalse(vm.uiState.value.reportOpen)
    }

    @Test
    fun `closeReport without an open report is a no-op`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { apiClient.getImportHistory() } returns Result.success(emptyList())
        val vm = ImportHistoryViewModel(apiClient)
        advanceUntilIdle()

        vm.closeReport()

        assertNull(vm.uiState.value.report)
        assertFalse(vm.uiState.value.reportOpen)
    }

    @Test
    fun `opening a second report replaces the first`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { apiClient.getImportHistory() } returns Result.success(emptyList())
        coEvery { apiClient.getImportRun(7) } returns Result.success(detail)
        coEvery { apiClient.getImportRun(8) } returns Result.success(detail.copy(id = 8))
        val vm = ImportHistoryViewModel(apiClient)
        advanceUntilIdle()

        vm.openReport(7)
        advanceUntilIdle()
        vm.openReport(8)
        advanceUntilIdle()

        assertEquals(8L, vm.uiState.value.report?.id)
    }
}
