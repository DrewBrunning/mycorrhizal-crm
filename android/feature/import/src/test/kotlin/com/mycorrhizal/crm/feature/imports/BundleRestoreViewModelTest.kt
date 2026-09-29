package com.mycorrhizal.crm.feature.imports

import com.mycorrhizal.crm.model.network.ImportConfirmRequest
import com.mycorrhizal.crm.model.network.ImportRowPreview
import com.mycorrhizal.crm.model.network.MycorrhizalBundleCounts
import com.mycorrhizal.crm.model.network.MycorrhizalUploadResponse
import com.mycorrhizal.crm.model.network.RowImportAction
import com.mycorrhizal.crm.model.network.SourceImportPreviewResponse
import com.mycorrhizal.crm.model.network.SourceImportResult
import com.mycorrhizal.crm.model.network.SourceImportStatus
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import com.mycorrhizal.crm.ui.R
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import io.mockk.slot
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

/** Issue #1264: the upload → fetch → poll → preview → confirm → poll restore state machine. */
class BundleRestoreViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val apiClient = mockk<ApiClient>()

    private fun newVm() = BundleRestoreViewModel(apiClient).also { it.pollIntervalMillis = 10L }

    private fun status(phase: String, error: String? = null, result: SourceImportResult? = null) =
        SourceImportStatus(sessionId = "s1", phase = phase, error = error, result = result)

    private fun row(index: Int, action: String = "add", errors: List<String> = emptyList()) =
        ImportRowPreview(rowIndex = index, suggestedAction = action, validationErrors = errors)

    private val preview = SourceImportPreviewResponse(
        sessionId = "s1",
        rows = listOf(row(0), row(1, action = "update"), row(2, errors = listOf("bad"))),
        totalRows = 3,
    )

    private fun stubUploadAndPrepare(vararg phases: SourceImportStatus) {
        coEvery { apiClient.uploadMycorrhizalBundle(any(), any()) } returns Result.success(
            MycorrhizalUploadResponse(sessionId = "s1", version = 1, totals = MycorrhizalBundleCounts(contacts = 3)),
        )
        coEvery { apiClient.startMycorrhizalFetch("s1") } returns Result.success(Unit)
        coEvery { apiClient.getMycorrhizalImportStatus("s1") } returnsMany phases.map { Result.success(it) }
        coEvery { apiClient.getMycorrhizalImportPreview("s1") } returns Result.success(preview)
    }

    @Test
    fun `a picked bundle is uploaded, prepared by polling, and lands on review with suggested actions`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubUploadAndPrepare(status("building_preview"), status("building_preview"), status("ready"))
            val vm = newVm()

            vm.onFilePicked("b.json", "{}".toByteArray())
            advanceUntilIdle()

            val state = vm.uiState.value
            assertEquals(BundleRestoreStep.REVIEW, state.step)
            assertEquals(3, state.totals?.contacts)
            assertEquals("b.json", state.fileName)
            assertEquals(false, state.isImporting)
            // Valid rows take their suggested action; the errored row is forced to skip.
            assertEquals(mapOf(0 to "add", 1 to "update", 2 to "skip"), state.rowActions)
            coVerify(exactly = 3) { apiClient.getMycorrhizalImportStatus("s1") }
        }

    @Test
    fun `an empty file is rejected without calling the server`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = newVm()

        vm.onFilePicked("b.json", ByteArray(0))

        assertEquals(R.string.bundle_restore_error_invalid_file, vm.uiState.value.errorRes)
        assertEquals(BundleRestoreStep.PICK, vm.uiState.value.step)
        coVerify(exactly = 0) { apiClient.uploadMycorrhizalBundle(any(), any()) }
    }

    @Test
    fun `a file over the 64 MiB cap is rejected without calling the server`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = newVm()

            vm.onFilePicked("b.json", ByteArray(BundleRestoreViewModel.MAX_BUNDLE_SIZE_BYTES + 1))

            assertEquals(R.string.bundle_restore_error_too_large, vm.uiState.value.errorRes)
            coVerify(exactly = 0) { apiClient.uploadMycorrhizalBundle(any(), any()) }
        }

    @Test
    fun `onFileTooLarge surfaces the too-large error`() {
        val vm = newVm()

        vm.onFileTooLarge()

        assertEquals(R.string.bundle_restore_error_too_large, vm.uiState.value.errorRes)
    }

    @Test
    fun `an upload failure returns to the picker with the server message`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.uploadMycorrhizalBundle(any(), any()) } returns
                Result.failure(ApiError.Client(422, "Unsupported bundle version"))
            val vm = newVm()

            vm.onFilePicked("b.json", "{}".toByteArray())
            advanceUntilIdle()

            assertEquals(BundleRestoreStep.PICK, vm.uiState.value.step)
            assertEquals(false, vm.uiState.value.isImporting)
            assertTrue(vm.uiState.value.error.orEmpty().contains("Unsupported bundle version"))
        }

    @Test
    fun `a failed mapping phase returns to the picker with the phase error`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubUploadAndPrepare(status("building_preview"), status("failed", error = "corrupt bundle"))
            val vm = newVm()

            vm.onFilePicked("b.json", "{}".toByteArray())
            advanceUntilIdle()

            assertEquals(BundleRestoreStep.PICK, vm.uiState.value.step)
            assertEquals("corrupt bundle", vm.uiState.value.error)
            coVerify(exactly = 0) { apiClient.getMycorrhizalImportPreview(any()) }
        }

    @Test
    fun `a failed status poll falls back to the generic error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { apiClient.uploadMycorrhizalBundle(any(), any()) } returns
            Result.success(MycorrhizalUploadResponse(sessionId = "s1"))
        coEvery { apiClient.startMycorrhizalFetch("s1") } returns Result.success(Unit)
        coEvery { apiClient.getMycorrhizalImportStatus("s1") } returns Result.failure(ApiError.Server(500, "x"))
        val vm = newVm()

        vm.onFilePicked("b.json", "{}".toByteArray())
        advanceUntilIdle()

        assertEquals(BundleRestoreStep.PICK, vm.uiState.value.step)
        assertEquals(R.string.bundle_restore_error_failed, vm.uiState.value.errorRes)
    }

    @Test
    fun `a failed fetch start returns to the picker`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { apiClient.uploadMycorrhizalBundle(any(), any()) } returns
            Result.success(MycorrhizalUploadResponse(sessionId = "s1"))
        coEvery { apiClient.startMycorrhizalFetch("s1") } returns Result.failure(ApiError.Client(409, "busy"))
        val vm = newVm()

        vm.onFilePicked("b.json", "{}".toByteArray())
        advanceUntilIdle()

        assertEquals(BundleRestoreStep.PICK, vm.uiState.value.step)
        coVerify(exactly = 0) { apiClient.getMycorrhizalImportStatus(any()) }
    }

    @Test
    fun `a failed preview fetch returns to the picker`() = runTest(mainDispatcherRule.testDispatcher) {
        stubUploadAndPrepare(status("ready"))
        coEvery { apiClient.getMycorrhizalImportPreview("s1") } returns Result.failure(ApiError.Client(400, "nope"))
        val vm = newVm()

        vm.onFilePicked("b.json", "{}".toByteArray())
        advanceUntilIdle()

        assertEquals(BundleRestoreStep.PICK, vm.uiState.value.step)
        assertNull(vm.uiState.value.preview)
    }

    @Test
    fun `a second pick while working is ignored`() = runTest(mainDispatcherRule.testDispatcher) {
        stubUploadAndPrepare(status("ready"))
        val vm = newVm()

        vm.onFilePicked("b.json", "{}".toByteArray())
        vm.onFilePicked("c.json", "{}".toByteArray())
        advanceUntilIdle()

        coVerify(exactly = 1) { apiClient.uploadMycorrhizalBundle(any(), any()) }
    }

    @Test
    fun `setRowAction overrides a valid row and ignores an errored one`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubUploadAndPrepare(status("ready"))
            val vm = newVm()
            vm.onFilePicked("b.json", "{}".toByteArray())
            advanceUntilIdle()

            vm.setRowAction(0, "skip")
            vm.setRowAction(2, "add")
            vm.setRowAction(99, "add")

            assertEquals("skip", vm.uiState.value.rowActions[0])
            assertEquals("skip", vm.uiState.value.rowActions[2])
            assertNull(vm.uiState.value.rowActions[99])
        }

    @Test
    fun `resolveAll restores every valid row to its suggested action`() = runTest(mainDispatcherRule.testDispatcher) {
        stubUploadAndPrepare(status("ready"))
        val vm = newVm()
        vm.onFilePicked("b.json", "{}".toByteArray())
        advanceUntilIdle()
        vm.setRowAction(0, "skip")
        vm.setRowAction(1, "add")

        vm.resolveAll()

        assertEquals(mapOf(0 to "add", 1 to "update", 2 to "skip"), vm.uiState.value.rowActions)
    }

    @Test
    fun `confirm sends the row actions, polls to done, and shows the result`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubUploadAndPrepare(status("ready"))
            val vm = newVm()
            vm.onFilePicked("b.json", "{}".toByteArray())
            advanceUntilIdle()
            val sent = slot<ImportConfirmRequest>()
            coEvery { apiClient.confirmMycorrhizalImport(capture(sent)) } returns Result.success(Unit)
            val result = SourceImportResult(created = 2, updated = 1, skipped = 1)
            coEvery { apiClient.getMycorrhizalImportStatus("s1") } returnsMany listOf(
                Result.success(status("importing")),
                Result.success(status("importing_photos")),
                Result.success(status("done", result = result)),
            )

            vm.confirm()
            advanceUntilIdle()

            assertEquals("s1", sent.captured.sessionId)
            assertEquals(
                setOf(RowImportAction(0, "add"), RowImportAction(1, "update"), RowImportAction(2, "skip")),
                sent.captured.actions.toSet(),
            )
            assertEquals(BundleRestoreStep.RESULT, vm.uiState.value.step)
            assertEquals(result, vm.uiState.value.result)
            assertEquals(false, vm.uiState.value.isImporting)
        }

    @Test
    fun `a failed import returns to review with the error and no result`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubUploadAndPrepare(status("ready"))
            val vm = newVm()
            vm.onFilePicked("b.json", "{}".toByteArray())
            advanceUntilIdle()
            coEvery { apiClient.confirmMycorrhizalImport(any()) } returns Result.success(Unit)
            coEvery { apiClient.getMycorrhizalImportStatus("s1") } returns
                Result.success(status("failed", error = "disk full"))

            vm.confirm()
            advanceUntilIdle()

            assertEquals(BundleRestoreStep.REVIEW, vm.uiState.value.step)
            assertEquals("disk full", vm.uiState.value.error)
            assertNull(vm.uiState.value.result)
        }

    @Test
    fun `a cancelled import returns to review`() = runTest(mainDispatcherRule.testDispatcher) {
        stubUploadAndPrepare(status("ready"))
        val vm = newVm()
        vm.onFilePicked("b.json", "{}".toByteArray())
        advanceUntilIdle()
        coEvery { apiClient.confirmMycorrhizalImport(any()) } returns Result.success(Unit)
        coEvery { apiClient.getMycorrhizalImportStatus("s1") } returns Result.success(status("cancelled"))

        vm.confirm()
        advanceUntilIdle()

        assertEquals(BundleRestoreStep.REVIEW, vm.uiState.value.step)
        assertEquals(R.string.bundle_restore_error_failed, vm.uiState.value.errorRes)
    }

    @Test
    fun `a rejected confirm returns to review without polling`() = runTest(mainDispatcherRule.testDispatcher) {
        stubUploadAndPrepare(status("ready"))
        val vm = newVm()
        vm.onFilePicked("b.json", "{}".toByteArray())
        advanceUntilIdle()
        coEvery { apiClient.confirmMycorrhizalImport(any()) } returns Result.failure(ApiError.Client(400, "expired"))

        vm.confirm()
        advanceUntilIdle()

        assertEquals(BundleRestoreStep.REVIEW, vm.uiState.value.step)
        coVerify(exactly = 1) { apiClient.getMycorrhizalImportStatus("s1") }
    }

    @Test
    fun `confirm before a preview exists does nothing`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = newVm()

        vm.confirm()
        advanceUntilIdle()

        coVerify(exactly = 0) { apiClient.confirmMycorrhizalImport(any()) }
    }

    @Test
    fun `cancel drops the server session once a preview exists but not after the result`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubUploadAndPrepare(status("ready"))
            coEvery { apiClient.cancelMycorrhizalImport("s1") } returns Result.success(Unit)
            val vm = newVm()

            vm.cancel()
            advanceUntilIdle()
            coVerify(exactly = 0) { apiClient.cancelMycorrhizalImport(any()) }

            vm.onFilePicked("b.json", "{}".toByteArray())
            advanceUntilIdle()
            vm.cancel()
            advanceUntilIdle()
            coVerify(exactly = 1) { apiClient.cancelMycorrhizalImport("s1") }
        }

    @Test
    fun `onErrorShown clears both error channels`() {
        val vm = newVm()
        vm.onFileTooLarge()

        vm.onErrorShown()

        assertNull(vm.uiState.value.errorRes)
        assertNull(vm.uiState.value.error)
    }
}
