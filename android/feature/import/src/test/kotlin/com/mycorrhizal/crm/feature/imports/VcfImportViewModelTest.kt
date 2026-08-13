package com.mycorrhizal.crm.feature.imports

import com.mycorrhizal.crm.model.network.ImportPreviewResponse
import com.mycorrhizal.crm.model.network.ImportResult
import com.mycorrhizal.crm.model.network.ImportRowPreview
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
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

// M9 item 4: uploadVcfImport() existed with zero UI callers; this is its first real caller.
class VcfImportViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val apiClient = mockk<ApiClient>()

    // The primary size gate: the screen probes the provider's declared size and calls this
    // WITHOUT ever reading the file, so picking a multi-gigabyte file can't OOM the app.
    @Test
    fun `a file whose declared size is over 50MB is rejected before its bytes are read`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = VcfImportViewModel(apiClient)

            vm.onFileTooLarge()
            advanceUntilIdle()

            assertEquals(R.string.import_vcf_error_too_large, vm.uiState.value.errorRes)
            assertEquals(VcfImportStep.PICK, vm.uiState.value.step)
            coVerify(exactly = 0) { apiClient.uploadVcfImport(any(), any()) }
        }

    // The backstop, for content providers that declare no size at all.
    @Test
    fun `a file over 50MB is rejected client-side without calling the API`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = VcfImportViewModel(apiClient)

        vm.onFilePicked("big.vcf", ByteArray(VcfImportViewModel.MAX_VCF_SIZE_BYTES + 1))
        advanceUntilIdle()

        assertEquals(R.string.import_vcf_error_too_large, vm.uiState.value.errorRes)
        assertEquals(VcfImportStep.PICK, vm.uiState.value.step)
        coVerify(exactly = 0) { apiClient.uploadVcfImport(any(), any()) }
    }

    @Test
    fun `an empty file is rejected client-side without calling the API`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = VcfImportViewModel(apiClient)

        vm.onFilePicked("empty.vcf", ByteArray(0))
        advanceUntilIdle()

        assertEquals(R.string.import_vcf_error_invalid_file, vm.uiState.value.errorRes)
        coVerify(exactly = 0) { apiClient.uploadVcfImport(any(), any()) }
    }

    @Test
    fun `a successful upload moves to the preview step defaulted to each row's suggested action`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.uploadVcfImport(any(), "contacts.vcf") } returns Result.success(
                ImportPreviewResponse(
                    sessionId = "session-1",
                    rows = listOf(
                        ImportRowPreview(rowIndex = 0, suggestedAction = "add"),
                        ImportRowPreview(rowIndex = 1, suggestedAction = "update"),
                    ),
                ),
            )

            val vm = VcfImportViewModel(apiClient)
            vm.onFilePicked("contacts.vcf", byteArrayOf(1, 2, 3))
            advanceUntilIdle()

            val state = vm.uiState.value
            assertEquals(VcfImportStep.PREVIEW, state.step)
            assertEquals("add", state.rowActions[0])
            assertEquals("update", state.rowActions[1])
        }

    @Test
    fun `a row with validation errors is forced to skip regardless of its suggested action`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.uploadVcfImport(any(), any()) } returns Result.success(
                ImportPreviewResponse(
                    sessionId = "session-1",
                    rows = listOf(
                        ImportRowPreview(rowIndex = 0, suggestedAction = "add", validationErrors = listOf("missing name")),
                    ),
                ),
            )

            val vm = VcfImportViewModel(apiClient)
            vm.onFilePicked("contacts.vcf", byteArrayOf(1))
            advanceUntilIdle()

            assertEquals("skip", vm.uiState.value.rowActions[0])

            // setRowAction is a no-op for an errored row — it stays forced to skip.
            vm.setRowAction(0, "add")
            assertEquals("skip", vm.uiState.value.rowActions[0])
        }

    @Test
    fun `a row without errors can have its action overridden`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { apiClient.uploadVcfImport(any(), any()) } returns Result.success(
            ImportPreviewResponse(sessionId = "session-1", rows = listOf(ImportRowPreview(rowIndex = 0, suggestedAction = "add"))),
        )

        val vm = VcfImportViewModel(apiClient)
        vm.onFilePicked("contacts.vcf", byteArrayOf(1))
        advanceUntilIdle()

        vm.setRowAction(0, "skip")

        assertEquals("skip", vm.uiState.value.rowActions[0])
    }

    @Test
    fun `upload failure surfaces the error and stays on the pick step`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { apiClient.uploadVcfImport(any(), any()) } returns Result.failure(ApiError.Client(400, "bad vcf"))

        val vm = VcfImportViewModel(apiClient)
        vm.onFilePicked("contacts.vcf", byteArrayOf(1))
        advanceUntilIdle()

        assertEquals("bad vcf", vm.uiState.value.error)
        assertEquals(VcfImportStep.PICK, vm.uiState.value.step)
    }

    @Test
    fun `confirm sends the session id and every row's chosen action, moving to the result step`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.uploadVcfImport(any(), any()) } returns Result.success(
                ImportPreviewResponse(
                    sessionId = "session-1",
                    rows = listOf(
                        ImportRowPreview(rowIndex = 0, suggestedAction = "add"),
                        ImportRowPreview(rowIndex = 1, suggestedAction = "update"),
                    ),
                ),
            )
            coEvery {
                apiClient.confirmVcfImport(
                    match { it.sessionId == "session-1" && it.actions.size == 2 },
                )
            } returns Result.success(ImportResult(totalProcessed = 2, created = 1, updated = 1))

            val vm = VcfImportViewModel(apiClient)
            vm.onFilePicked("contacts.vcf", byteArrayOf(1))
            advanceUntilIdle()
            vm.setRowAction(1, "skip")
            vm.confirm()
            advanceUntilIdle()

            val state = vm.uiState.value
            assertEquals(VcfImportStep.RESULT, state.step)
            assertEquals(1, state.result?.created)
            coVerify {
                apiClient.confirmVcfImport(
                    match { request ->
                        request.actions.any { it.rowIndex == 0 && it.action == "add" } &&
                            request.actions.any { it.rowIndex == 1 && it.action == "skip" }
                    },
                )
            }
        }

    @Test
    fun `confirm failure surfaces the error and stays on the preview step`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { apiClient.uploadVcfImport(any(), any()) } returns Result.success(
            ImportPreviewResponse(sessionId = "session-1", rows = listOf(ImportRowPreview(rowIndex = 0, suggestedAction = "add"))),
        )
        coEvery { apiClient.confirmVcfImport(any()) } returns Result.failure(ApiError.Client(500, "server error"))

        val vm = VcfImportViewModel(apiClient)
        vm.onFilePicked("contacts.vcf", byteArrayOf(1))
        advanceUntilIdle()
        vm.confirm()
        advanceUntilIdle()

        assertEquals("server error", vm.uiState.value.error)
        assertEquals(VcfImportStep.PREVIEW, vm.uiState.value.step)
    }
}
