package com.mycorrhizal.crm.feature.imports

import com.mycorrhizal.crm.model.network.ColumnMapping
import com.mycorrhizal.crm.model.network.ImportPreviewRequest
import com.mycorrhizal.crm.model.network.ImportPreviewResponse
import com.mycorrhizal.crm.model.network.ImportResult
import com.mycorrhizal.crm.model.network.ImportRowPreview
import com.mycorrhizal.crm.model.network.ImportUploadResponse
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import com.mycorrhizal.crm.ui.R
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

// Issue #834: uploadCsvImport()/previewCsvImport()/confirmImport() existed with zero UI callers.
class CsvImportViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val apiClient = mockk<ApiClient>()

    @Test
    fun `a file over 20MB is rejected client-side without calling the API`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = CsvImportViewModel(apiClient)

        vm.onFilePicked("big.csv", ByteArray(CsvImportViewModel.MAX_CSV_SIZE_BYTES + 1))
        advanceUntilIdle()

        assertEquals(R.string.import_csv_error_too_large, vm.uiState.value.errorRes)
        assertEquals(CsvImportStep.PICK, vm.uiState.value.step)
        coVerify(exactly = 0) { apiClient.uploadCsvImport(any(), any()) }
    }

    @Test
    fun `a file whose declared size is over 20MB is rejected before its bytes are read`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = CsvImportViewModel(apiClient)

            vm.onFileTooLarge()
            advanceUntilIdle()

            assertEquals(R.string.import_csv_error_too_large, vm.uiState.value.errorRes)
            assertEquals(CsvImportStep.PICK, vm.uiState.value.step)
            coVerify(exactly = 0) { apiClient.uploadCsvImport(any(), any()) }
        }

    @Test
    fun `an empty file is rejected client-side without calling the API`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = CsvImportViewModel(apiClient)

        vm.onFilePicked("empty.csv", ByteArray(0))
        advanceUntilIdle()

        assertEquals(R.string.import_csv_error_invalid_file, vm.uiState.value.errorRes)
        coVerify(exactly = 0) { apiClient.uploadCsvImport(any(), any()) }
    }

    @Test
    fun `a successful upload seeds column fields from suggested mappings and moves to the mapping step`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.uploadCsvImport(any(), "contacts.csv") } returns Result.success(
                ImportUploadResponse(
                    sessionId = "session-1",
                    headers = listOf("Name", "Email"),
                    suggestedMappings = listOf(
                        ColumnMapping(csvColumn = "Name", contactField = "firstname", group = 0),
                        ColumnMapping(csvColumn = "Email", contactField = "email", group = 0),
                    ),
                    rowCount = 2,
                    sampleData = listOf(listOf("Dana White", "dana@example.com")),
                ),
            )

            val vm = CsvImportViewModel(apiClient)
            vm.onFilePicked("contacts.csv", byteArrayOf(1, 2, 3))
            advanceUntilIdle()

            val state = vm.uiState.value
            assertEquals(CsvImportStep.MAPPING, state.step)
            assertEquals("firstname", state.columnFields["Name"])
            assertEquals("email", state.columnFields["Email"])
        }

    @Test
    fun `upload failure surfaces the error and stays on the pick step`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { apiClient.uploadCsvImport(any(), any()) } returns Result.failure(ApiError.Client(400, "bad csv"))

        val vm = CsvImportViewModel(apiClient)
        vm.onFilePicked("contacts.csv", byteArrayOf(1))
        advanceUntilIdle()

        assertEquals("bad csv", vm.uiState.value.error)
        assertEquals(CsvImportStep.PICK, vm.uiState.value.step)
    }

    @Test
    fun `changing a column field overrides the suggested mapping`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { apiClient.uploadCsvImport(any(), any()) } returns Result.success(
            ImportUploadResponse(
                sessionId = "session-1",
                headers = listOf("Name"),
                suggestedMappings = listOf(ColumnMapping(csvColumn = "Name", contactField = "firstname", group = 0)),
                rowCount = 1,
            ),
        )

        val vm = CsvImportViewModel(apiClient)
        vm.onFilePicked("contacts.csv", byteArrayOf(1))
        advanceUntilIdle()

        vm.setColumnField("Name", "lastname")

        assertEquals("lastname", vm.uiState.value.columnFields["Name"])
    }

    @Test
    fun `submitting the mapping sends only non-ignored columns and moves to the preview step`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.uploadCsvImport(any(), any()) } returns Result.success(
                ImportUploadResponse(
                    sessionId = "session-1",
                    headers = listOf("Name", "Notes"),
                    suggestedMappings = listOf(
                        ColumnMapping(csvColumn = "Name", contactField = "firstname", group = 0),
                        ColumnMapping(csvColumn = "Notes", contactField = "", group = 0),
                    ),
                    rowCount = 1,
                ),
            )
            coEvery {
                apiClient.previewCsvImport(
                    match { it.sessionId == "session-1" && it.mappings.size == 1 && it.mappings[0].csvColumn == "Name" },
                )
            } returns Result.success(
                ImportPreviewResponse(
                    sessionId = "session-1",
                    rows = listOf(ImportRowPreview(rowIndex = 0, suggestedAction = "add")),
                ),
            )

            val vm = CsvImportViewModel(apiClient)
            vm.onFilePicked("contacts.csv", byteArrayOf(1))
            advanceUntilIdle()
            // "Notes" is left ignored (suggested contactField == ""); only "Name" should be sent.
            vm.submitMapping()
            advanceUntilIdle()

            val state = vm.uiState.value
            assertEquals(CsvImportStep.PREVIEW, state.step)
            assertEquals("add", state.rowActions[0])
            coVerify {
                apiClient.previewCsvImport(
                    match { request ->
                        request.mappings.none { it.csvColumn == "Notes" }
                    },
                )
            }
        }

    @Test
    fun `a preview row with validation errors is forced to skip regardless of its suggested action`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.uploadCsvImport(any(), any()) } returns Result.success(
                ImportUploadResponse(
                    sessionId = "session-1",
                    headers = listOf("Name"),
                    suggestedMappings = listOf(ColumnMapping(csvColumn = "Name", contactField = "firstname", group = 0)),
                    rowCount = 1,
                ),
            )
            coEvery { apiClient.previewCsvImport(any<ImportPreviewRequest>()) } returns Result.success(
                ImportPreviewResponse(
                    sessionId = "session-1",
                    rows = listOf(
                        ImportRowPreview(rowIndex = 0, suggestedAction = "add", validationErrors = listOf("missing name")),
                    ),
                ),
            )

            val vm = CsvImportViewModel(apiClient)
            vm.onFilePicked("contacts.csv", byteArrayOf(1))
            advanceUntilIdle()
            vm.submitMapping()
            advanceUntilIdle()

            assertEquals("skip", vm.uiState.value.rowActions[0])
            vm.setRowAction(0, "add")
            assertEquals("skip", vm.uiState.value.rowActions[0])
        }

    @Test
    fun `mapping preview failure surfaces the error and stays on the mapping step`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.uploadCsvImport(any(), any()) } returns Result.success(
                ImportUploadResponse(sessionId = "session-1", headers = listOf("Name"), rowCount = 1),
            )
            coEvery { apiClient.previewCsvImport(any<ImportPreviewRequest>()) } returns Result.failure(ApiError.Client(400, "bad mapping"))

            val vm = CsvImportViewModel(apiClient)
            vm.onFilePicked("contacts.csv", byteArrayOf(1))
            advanceUntilIdle()
            vm.submitMapping()
            advanceUntilIdle()

            assertEquals("bad mapping", vm.uiState.value.error)
            assertEquals(CsvImportStep.MAPPING, vm.uiState.value.step)
        }

    @Test
    fun `resolveAll sets every valid row to its suggested action and leaves errored rows skipped`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.uploadCsvImport(any(), any()) } returns Result.success(
                ImportUploadResponse(sessionId = "session-1", headers = listOf("Name"), rowCount = 3),
            )
            coEvery { apiClient.previewCsvImport(any<ImportPreviewRequest>()) } returns Result.success(
                ImportPreviewResponse(
                    sessionId = "session-1",
                    rows = listOf(
                        ImportRowPreview(rowIndex = 0, suggestedAction = "update"),
                        ImportRowPreview(rowIndex = 1, suggestedAction = "add"),
                        ImportRowPreview(rowIndex = 2, validationErrors = listOf("missing name"), suggestedAction = "skip"),
                    ),
                ),
            )

            val vm = CsvImportViewModel(apiClient)
            vm.onFilePicked("contacts.csv", byteArrayOf(1))
            advanceUntilIdle()
            vm.submitMapping()
            advanceUntilIdle()

            vm.setRowAction(0, "skip")
            vm.setRowAction(1, "skip")
            vm.resolveAll()

            val actions = vm.uiState.value.rowActions
            assertEquals("update", actions[0])
            assertEquals("add", actions[1])
            assertEquals("skip", actions[2])
        }

    @Test
    fun `confirm sends the session id and every row's chosen action via confirmImport, moving to the result step`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.uploadCsvImport(any(), any()) } returns Result.success(
                ImportUploadResponse(sessionId = "session-1", headers = listOf("Name"), rowCount = 2),
            )
            coEvery { apiClient.previewCsvImport(any<ImportPreviewRequest>()) } returns Result.success(
                ImportPreviewResponse(
                    sessionId = "session-1",
                    rows = listOf(
                        ImportRowPreview(rowIndex = 0, suggestedAction = "add"),
                        ImportRowPreview(rowIndex = 1, suggestedAction = "update"),
                    ),
                ),
            )
            coEvery {
                apiClient.confirmImport(match { it.sessionId == "session-1" && it.actions.size == 2 })
            } returns Result.success(ImportResult(totalProcessed = 2, created = 1, updated = 1))

            val vm = CsvImportViewModel(apiClient)
            vm.onFilePicked("contacts.csv", byteArrayOf(1))
            advanceUntilIdle()
            vm.submitMapping()
            advanceUntilIdle()
            vm.setRowAction(1, "skip")
            vm.confirm()
            advanceUntilIdle()

            val state = vm.uiState.value
            assertEquals(CsvImportStep.RESULT, state.step)
            assertEquals(1, state.result?.created)
            coVerify(exactly = 0) { apiClient.confirmVcfImport(any()) }
        }

    @Test
    fun `confirm failure surfaces the error and stays on the preview step`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { apiClient.uploadCsvImport(any(), any()) } returns Result.success(
            ImportUploadResponse(sessionId = "session-1", headers = listOf("Name"), rowCount = 1),
        )
        coEvery { apiClient.previewCsvImport(any<ImportPreviewRequest>()) } returns Result.success(
            ImportPreviewResponse(sessionId = "session-1", rows = listOf(ImportRowPreview(rowIndex = 0, suggestedAction = "add"))),
        )
        coEvery { apiClient.confirmImport(any()) } returns Result.failure(ApiError.Client(500, "server error"))

        val vm = CsvImportViewModel(apiClient)
        vm.onFilePicked("contacts.csv", byteArrayOf(1))
        advanceUntilIdle()
        vm.submitMapping()
        advanceUntilIdle()
        vm.confirm()
        advanceUntilIdle()

        assertEquals("server error", vm.uiState.value.error)
        assertEquals(CsvImportStep.PREVIEW, vm.uiState.value.step)
    }

    @Test
    fun `columns with an empty suggested field default to ignored and are excluded from the mapping request`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.uploadCsvImport(any(), any()) } returns Result.success(
                ImportUploadResponse(
                    sessionId = "session-1",
                    headers = listOf("Unknown Column"),
                    suggestedMappings = listOf(ColumnMapping(csvColumn = "Unknown Column", contactField = "", group = 0)),
                    rowCount = 1,
                ),
            )

            val vm = CsvImportViewModel(apiClient)
            vm.onFilePicked("contacts.csv", byteArrayOf(1))
            advanceUntilIdle()

            assertTrue(vm.uiState.value.columnFields["Unknown Column"].isNullOrBlank())
        }
}
