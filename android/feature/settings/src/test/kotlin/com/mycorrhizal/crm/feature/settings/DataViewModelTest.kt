package com.mycorrhizal.crm.feature.settings

import com.mycorrhizal.crm.domain.backup.BundleBackupStatus
import com.mycorrhizal.crm.domain.repository.BundleBackupRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.RelationshipEdgeRepository
import com.mycorrhizal.crm.domain.repository.ExportRepository
import com.mycorrhizal.crm.model.network.ApplyContactAddressSuggestionInput
import com.mycorrhizal.crm.model.network.ContactAddressSuggestion
import com.mycorrhizal.crm.model.network.RelationshipEdge
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import com.mycorrhizal.crm.ui.R
import io.mockk.coEvery
import io.mockk.Runs
import io.mockk.coVerify
import io.mockk.every
import io.mockk.just
import io.mockk.mockk
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class DataViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val contactRepository = mockk<ContactRepository>()
    private val relationshipEdgeRepository = mockk<RelationshipEdgeRepository>()
    private val exportRepository = mockk<ExportRepository>()
    private val backupStatus = MutableStateFlow(BundleBackupStatus())
    private val bundleBackupRepository = mockk<BundleBackupRepository> {
        every { observeStatus() } returns backupStatus
        coEvery { recordExport(any()) } just Runs
    }

    private val suggestion = ContactAddressSuggestion(
        contactVCardUid = "alice-uid",
        contactName = "Alice",
        sourceKind = "relationship",
        sourceId = "bob-uid",
        sourceName = "Bob",
        relationType = "spouse_of",
        addressKey = "key1",
    )

    @Test
    fun `suggestRelationships records the count of newly created edges`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { relationshipEdgeRepository.suggest() } returns Result.success(listOf(RelationshipEdge(id = "e1")))
            val vm = DataViewModel(contactRepository, relationshipEdgeRepository, exportRepository, bundleBackupRepository)

            vm.suggestRelationships()
            advanceUntilIdle()

            coVerify { relationshipEdgeRepository.suggest() }
            assertEquals(1, vm.uiState.value.suggestedRelationshipCount)
            assertNull(vm.uiState.value.error)
        }

    @Test
    fun `suggestRelationships failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { relationshipEdgeRepository.suggest() } returns Result.failure(ApiError.Client(500, "boom"))
        val vm = DataViewModel(contactRepository, relationshipEdgeRepository, exportRepository, bundleBackupRepository)

        vm.suggestRelationships()
        advanceUntilIdle()

        assertEquals("boom", vm.uiState.value.error)
        assertNull(vm.uiState.value.suggestedRelationshipCount)
    }

    @Test
    fun `scanAddressSuggestions loads the suggestions`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { contactRepository.suggestContactAddresses() } returns Result.success(listOf(suggestion))
        val vm = DataViewModel(contactRepository, relationshipEdgeRepository, exportRepository, bundleBackupRepository)

        vm.scanAddressSuggestions()
        advanceUntilIdle()

        coVerify { contactRepository.suggestContactAddresses() }
        assertEquals(listOf(suggestion), vm.uiState.value.addressSuggestions)
        assertTrue(vm.uiState.value.suggestionsLoaded)
    }

    @Test
    fun `scanAddressSuggestions failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { contactRepository.suggestContactAddresses() } returns Result.failure(ApiError.Client(500, "boom"))
        val vm = DataViewModel(contactRepository, relationshipEdgeRepository, exportRepository, bundleBackupRepository)

        vm.scanAddressSuggestions()
        advanceUntilIdle()

        assertEquals("boom", vm.uiState.value.error)
        assertTrue(vm.uiState.value.addressSuggestions.isEmpty())
    }

    @Test
    fun `applySuggestion removes the row and reports success`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { contactRepository.suggestContactAddresses() } returns Result.success(listOf(suggestion))
        coEvery { contactRepository.applyContactAddressSuggestion(any()) } returns Result.success(Unit)
        val vm = DataViewModel(contactRepository, relationshipEdgeRepository, exportRepository, bundleBackupRepository)

        vm.scanAddressSuggestions()
        advanceUntilIdle()
        assertEquals(1, vm.uiState.value.addressSuggestions.size)

        vm.applySuggestion(suggestion)
        advanceUntilIdle()

        coVerify {
            contactRepository.applyContactAddressSuggestion(
                ApplyContactAddressSuggestionInput(
                    contactVCardUid = "alice-uid",
                    sourceKind = "relationship",
                    sourceId = "bob-uid",
                    addressKey = "key1",
                ),
            )
        }
        assertTrue(vm.uiState.value.addressSuggestions.isEmpty())
        assertEquals(R.string.data_address_applied, vm.uiState.value.infoRes)
    }

    @Test
    fun `applySuggestion failure keeps the row and surfaces the error`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { contactRepository.suggestContactAddresses() } returns Result.success(listOf(suggestion))
            coEvery { contactRepository.applyContactAddressSuggestion(any()) } returns
                Result.failure(ApiError.Client(409, "stale"))
            val vm = DataViewModel(contactRepository, relationshipEdgeRepository, exportRepository, bundleBackupRepository)

            vm.scanAddressSuggestions()
            advanceUntilIdle()

            vm.applySuggestion(suggestion)
            advanceUntilIdle()

            assertEquals("stale", vm.uiState.value.error)
            assertEquals(1, vm.uiState.value.addressSuggestions.size)
            assertNull(vm.uiState.value.infoRes)
        }

    @Test
    fun `export CSV fetches the csv bytes and exposes them once`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { exportRepository.exportDataCsv() } returns Result.success("csv".toByteArray())
            val vm = DataViewModel(contactRepository, relationshipEdgeRepository, exportRepository, bundleBackupRepository)

            vm.export(DataExportKind.CSV)
            advanceUntilIdle()

            val exported = vm.uiState.value.exported
            assertEquals(DataExportKind.CSV, exported?.kind)
            assertEquals("csv", exported?.bytes?.decodeToString())
            assertEquals("mycorrhizal-export.csv", exported?.fileName)
            assertTrue(!vm.uiState.value.isExporting)

            vm.onExportHandled()
            assertNull(vm.uiState.value.exported)
        }

    @Test
    fun `export routes each kind to the matching repository call`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { exportRepository.exportContactsVcf(any()) } returns Result.success("vcf".toByteArray())
            coEvery { exportRepository.exportContactsJsContact() } returns Result.success("[]".toByteArray())
            coEvery { exportRepository.exportAuditLogCsv() } returns Result.success("a,b".toByteArray())

            val vm = DataViewModel(contactRepository, relationshipEdgeRepository, exportRepository, bundleBackupRepository)

            vm.export(DataExportKind.VCF3)
            advanceUntilIdle()
            assertEquals("mycorrhizal-contacts-v3.vcf", vm.uiState.value.exported?.fileName)
            coVerify { exportRepository.exportContactsVcf(3) }
            vm.onExportHandled()

            vm.export(DataExportKind.VCF4)
            advanceUntilIdle()
            coVerify { exportRepository.exportContactsVcf(null) }
            vm.onExportHandled()

            vm.export(DataExportKind.JSCONTACT)
            advanceUntilIdle()
            coVerify { exportRepository.exportContactsJsContact() }
            vm.onExportHandled()

            vm.export(DataExportKind.AUDIT_CSV)
            advanceUntilIdle()
            coVerify { exportRepository.exportAuditLogCsv() }
            vm.onExportHandled()
        }

    @Test
    fun `export failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { exportRepository.exportContactsVcf(any()) } returns
            Result.failure(ApiError.Client(500, "export failed"))
        val vm = DataViewModel(contactRepository, relationshipEdgeRepository, exportRepository, bundleBackupRepository)

        vm.export(DataExportKind.VCF4)
        advanceUntilIdle()

        assertEquals("export failed", vm.uiState.value.error)
        assertNull(vm.uiState.value.exported)
        assertTrue(!vm.uiState.value.isExporting)
    }

    // --- Issue #1264: account bundle export + restore eligibility ---------------

    private fun newVm() =
        DataViewModel(contactRepository, relationshipEdgeRepository, exportRepository, bundleBackupRepository)
            .also { it.nowMillis = { 1_800_000_000_000L } }

    @Test
    fun `accountBundleFileName embeds the ISO date`() {
        val previous = java.util.TimeZone.getDefault()
        java.util.TimeZone.setDefault(java.util.TimeZone.getTimeZone("UTC"))
        try {
            // 1_800_000_000 s == 2027-01-15T08:00:00Z.
            assertEquals("mycorrhizal-account-2027-01-15.json", accountBundleFileName(1_800_000_000_000L))
        } finally {
            java.util.TimeZone.setDefault(previous)
        }
    }

    @Test
    fun `exportAccountBundle hands the bytes to the writer then records the export`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val bytes = "{\"format\":\"mycorrhizal-account\"}".toByteArray()
            coEvery { exportRepository.exportAccountBundle() } returns Result.success(bytes)
            var written: ByteArray? = null
            val vm = newVm()

            vm.exportAccountBundle { written = it; Result.success(Unit) }
            advanceUntilIdle()

            assertTrue(bytes.contentEquals(written))
            coVerify(exactly = 1) { bundleBackupRepository.recordExport(1_800_000_000_000L) }
            assertEquals(R.string.data_bundle_exported, vm.uiState.value.infoRes)
            assertEquals(false, vm.uiState.value.isExporting)
        }

    @Test
    fun `a failed file write does not record the export`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { exportRepository.exportAccountBundle() } returns Result.success(byteArrayOf(1))
        val vm = newVm()

        vm.exportAccountBundle { Result.failure(java.io.IOException("disk full")) }
        advanceUntilIdle()

        coVerify(exactly = 0) { bundleBackupRepository.recordExport(any()) }
        assertEquals(R.string.data_bundle_write_failed, vm.uiState.value.infoRes)
        assertEquals(false, vm.uiState.value.isExporting)
    }

    @Test
    fun `a failed fetch surfaces the error and never calls the writer`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { exportRepository.exportAccountBundle() } returns Result.failure(ApiError.Client(500, "boom"))
        var wrote = false
        val vm = newVm()

        vm.exportAccountBundle { wrote = true; Result.success(Unit) }
        advanceUntilIdle()

        assertEquals(false, wrote)
        coVerify(exactly = 0) { bundleBackupRepository.recordExport(any()) }
        assertEquals("boom", vm.uiState.value.error)
    }

    @Test
    fun `a second export while one is in flight is ignored`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { exportRepository.exportAccountBundle() } returns Result.success(byteArrayOf(1))
        val vm = newVm()
        var writes = 0

        vm.exportAccountBundle { writes++; Result.success(Unit) }
        vm.exportAccountBundle { writes++; Result.success(Unit) }
        advanceUntilIdle()

        assertEquals(1, writes)
    }

    @Test
    fun `restore is offered only on an empty local profile`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { contactRepository.listContacts(limit = 1) } returns
            Result.success(com.mycorrhizal.crm.domain.repository.ContactsPage(emptyList(), null, 1, null))
        backupStatus.value = BundleBackupStatus(isLocalProfile = true)
        val vm = newVm()
        advanceUntilIdle()

        assertTrue(vm.uiState.value.canRestoreBundle)

        backupStatus.value = BundleBackupStatus(isLocalProfile = false)
        advanceUntilIdle()

        assertEquals(false, vm.uiState.value.canRestoreBundle)
    }

    @Test
    fun `restore is hidden on a local profile that already has contacts`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { contactRepository.listContacts(limit = 1) } returns Result.success(
                com.mycorrhizal.crm.domain.repository.ContactsPage(
                    listOf(com.mycorrhizal.crm.model.network.ContactSummary(id = 1)), null, 1, null,
                ),
            )
            backupStatus.value = BundleBackupStatus(isLocalProfile = true)
            val vm = newVm()
            advanceUntilIdle()

            assertEquals(false, vm.uiState.value.canRestoreBundle)
        }

    @Test
    fun `restore stays hidden when the emptiness probe fails`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { contactRepository.listContacts(limit = 1) } returns Result.failure(ApiError.Client(500, "x"))
        backupStatus.value = BundleBackupStatus(isLocalProfile = true)
        val vm = newVm()
        advanceUntilIdle()

        assertEquals(false, vm.uiState.value.canRestoreBundle)
    }
}
