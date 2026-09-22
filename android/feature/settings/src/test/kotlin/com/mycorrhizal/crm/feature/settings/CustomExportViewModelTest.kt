package com.mycorrhizal.crm.feature.settings

import com.mycorrhizal.crm.domain.repository.ExportRepository
import com.mycorrhizal.crm.model.network.ExportLossPreflightResponse
import com.mycorrhizal.crm.model.network.ExportLossReport
import com.mycorrhizal.crm.model.network.ShareFieldSections
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

class CustomExportViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val exportRepository = mockk<ExportRepository>()

    @Test
    fun `non-sensitive sections are selected by default`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = CustomExportViewModel(exportRepository)

        val state = vm.uiState.value
        assertTrue(state.selected.contains("emails"))
        assertTrue(state.selected.contains("phones"))
        assertFalse(state.selected.contains("related_to"))
        assertFalse(state.selected.contains("personal_info"))
        assertFalse(state.selected.contains("custom_fields"))
        assertFalse(state.sensitiveRevealed)
        assertEquals(ExportFormatChoice.VCF4, state.format)
    }

    @Test
    fun `toggleSection adds and removes a token`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = CustomExportViewModel(exportRepository)

        vm.toggleSection("emails", false)
        assertFalse(vm.uiState.value.selected.contains("emails"))

        vm.toggleSection("related_to", true)
        assertTrue(vm.uiState.value.selected.contains("related_to"))
    }

    @Test
    fun `revealSensitive unlocks sensitive sections`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = CustomExportViewModel(exportRepository)

        assertFalse(vm.uiState.value.sensitiveRevealed)
        vm.revealSensitive()
        assertTrue(vm.uiState.value.sensitiveRevealed)
    }

    @Test
    fun `export sends includeSensitive false when revealed but nothing sensitive is checked`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { exportRepository.exportContactsVcf(null, any(), any()) } returns
                Result.success("vcf".toByteArray())
            val vm = CustomExportViewModel(exportRepository)

            vm.revealSensitive()
            vm.export()
            advanceUntilIdle()

            coVerify { exportRepository.exportContactsVcf(null, any(), includeSensitive = false) }
        }

    @Test
    fun `export sends includeSensitive true only when revealed and a sensitive section is checked`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { exportRepository.exportContactsVcf(null, any(), any()) } returns
                Result.success("vcf".toByteArray())
            val vm = CustomExportViewModel(exportRepository)

            vm.toggleSection("related_to", true)
            vm.export()
            advanceUntilIdle()
            coVerify { exportRepository.exportContactsVcf(null, any(), includeSensitive = false) }
            assertNull(vm.uiState.value.error)

            vm.revealSensitive()
            vm.export()
            advanceUntilIdle()
            coVerify { exportRepository.exportContactsVcf(null, any(), includeSensitive = true) }
        }

    @Test
    fun `export dispatches to the repository call matching the chosen format`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { exportRepository.exportContactsVcf(3, any(), any()) } returns
                Result.success("vcf3".toByteArray())
            coEvery { exportRepository.exportContactsJsContact(any(), any()) } returns
                Result.success("[]".toByteArray())
            val vm = CustomExportViewModel(exportRepository)

            vm.setFormat(ExportFormatChoice.VCF3)
            vm.export()
            advanceUntilIdle()
            assertEquals(DataExportKind.CUSTOM_VCF3, vm.uiState.value.exported?.kind)
            vm.onExportHandled()

            vm.setFormat(ExportFormatChoice.JSCONTACT)
            vm.export()
            advanceUntilIdle()
            assertEquals(DataExportKind.CUSTOM_JSCONTACT, vm.uiState.value.exported?.kind)
            coVerify { exportRepository.exportContactsJsContact(any(), any()) }
        }

    @Test
    fun `export failure surfaces the error and clears isExporting`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { exportRepository.exportContactsVcf(null, any(), any()) } returns
            Result.failure(ApiError.Client(500, "boom"))
        val vm = CustomExportViewModel(exportRepository)

        vm.export()
        advanceUntilIdle()

        assertEquals("boom", vm.uiState.value.error)
        assertNull(vm.uiState.value.exported)
        assertFalse(vm.uiState.value.isExporting)
    }

    @Test
    fun `export is a no-op with an empty selection`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = CustomExportViewModel(exportRepository)
        ShareFieldSections.ALL.forEach { vm.toggleSection(it.token, false) }

        vm.export()
        advanceUntilIdle()

        assertNull(vm.uiState.value.exported)
        assertFalse(vm.uiState.value.isExporting)
    }

    @Test
    fun `checkLossReport populates the report on success`() = runTest(mainDispatcherRule.testDispatcher) {
        val report = ExportLossPreflightResponse(
            format = "vcard4",
            contactCount = 2,
            diagnostics = listOf(
                ExportLossReport(
                    format = "vcard4",
                    contactId = 1,
                    contactName = "Alice",
                    vcardUid = "alice-uid",
                    severity = "warn",
                    concept = "custom_field",
                    bucket = "unsupported",
                    reason = "no vcard4 home",
                    message = "Custom field 'Favorite color' has no vCard 4.0 home",
                ),
            ),
        )
        coEvery { exportRepository.preflight("vcard4", any(), any()) } returns Result.success(report)
        val vm = CustomExportViewModel(exportRepository)

        vm.checkLossReport()
        advanceUntilIdle()

        assertEquals(report, vm.uiState.value.lossReport)
        assertFalse(vm.uiState.value.isCheckingLoss)

        vm.onLossReportShown()
        assertNull(vm.uiState.value.lossReport)
    }

    @Test
    fun `checkLossReport failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { exportRepository.preflight("vcard4", any(), any()) } returns
            Result.failure(ApiError.Client(500, "preflight failed"))
        val vm = CustomExportViewModel(exportRepository)

        vm.checkLossReport()
        advanceUntilIdle()

        assertEquals("preflight failed", vm.uiState.value.error)
        assertNull(vm.uiState.value.lossReport)
    }
}
