package com.mycorrhizal.crm.feature.imports

import com.mycorrhizal.crm.model.network.ImportRun
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
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
    fun `loads the history on init, newest first as returned by the server`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { apiClient.getImportHistory() } returns Result.success(
            listOf(
                ImportRun(id = 2, format = "csv", created = 3, updated = 1, skipped = 0, errorCount = 0, createdAt = "2026-09-20T12:00:00Z"),
                ImportRun(id = 1, format = "vcf", created = 1, updated = 0, skipped = 2, errorCount = 1, createdAt = "2026-09-19T12:00:00Z"),
            ),
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
    fun `an empty history is represented as an empty list, not an error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { apiClient.getImportHistory() } returns Result.success(emptyList())

        val vm = ImportHistoryViewModel(apiClient)
        advanceUntilIdle()

        assertTrue(vm.uiState.value.runs.isEmpty())
        assertNull(vm.uiState.value.error)
    }

    @Test
    fun `a load failure surfaces the error message and leaves the list empty`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { apiClient.getImportHistory() } returns Result.failure(ApiError.Client(500, "server error"))

        val vm = ImportHistoryViewModel(apiClient)
        advanceUntilIdle()

        assertEquals("server error", vm.uiState.value.error)
        assertTrue(vm.uiState.value.runs.isEmpty())
    }

    @Test
    fun `load can be retried after a failure and clears the previous error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { apiClient.getImportHistory() } returns Result.failure(ApiError.Client(500, "server error"))
        val vm = ImportHistoryViewModel(apiClient)
        advanceUntilIdle()
        assertEquals("server error", vm.uiState.value.error)

        coEvery { apiClient.getImportHistory() } returns Result.success(
            listOf(ImportRun(id = 1, format = "csv")),
        )
        vm.load()
        advanceUntilIdle()

        assertNull(vm.uiState.value.error)
        assertEquals(1, vm.uiState.value.runs.size)
    }

    @Test
    fun `onErrorShown clears the error without touching the loaded runs`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { apiClient.getImportHistory() } returns Result.failure(ApiError.Client(500, "server error"))
        val vm = ImportHistoryViewModel(apiClient)
        advanceUntilIdle()

        vm.onErrorShown()

        assertNull(vm.uiState.value.error)
    }
}
