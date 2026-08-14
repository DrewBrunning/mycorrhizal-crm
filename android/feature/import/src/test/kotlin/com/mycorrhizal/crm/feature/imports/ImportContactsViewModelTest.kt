package com.mycorrhizal.crm.feature.imports

import android.content.ContentResolver
import android.content.Context
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.model.network.ImportPreviewResponse
import com.mycorrhizal.crm.model.network.ImportResult
import com.mycorrhizal.crm.model.network.ImportRowPreview
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test

// T96: the device-contacts flow now routes the selected contacts through the
// server's import preview (submitSelected) and the per-row merge review
// (setRowAction / resolveAll / confirmImport) instead of creating each one
// unconditionally.
class ImportContactsViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val apiClient = mockk<ApiClient>()
    private val contactRepository = mockk<ContactRepository>()

    private fun vm(contacts: List<DeviceContact>): ImportContactsViewModel {
        coEvery { contactRepository.findByEmail(any()) } returns null
        coEvery { contactRepository.findByPhone(any()) } returns null
        return ImportContactsViewModel(
            apiClient = apiClient,
            contactRepository = contactRepository,
            appContext = mockk<Context>(relaxed = true),
        ).apply {
            readDeviceContacts = { contacts }
            ioDispatcher = mainDispatcherRule.testDispatcher
        }
    }

    private fun preview(vararg rows: ImportRowPreview) = ImportPreviewResponse(
        sessionId = "session-device",
        rows = rows.toList(),
    )

    @Test
    fun `submitSelected sends the selected device contacts to the server preview and moves to review`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val device = DeviceContact(
                contactId = 1,
                lookupKey = "lk-1",
                displayName = "Jane Smith",
                phones = listOf("+15559998888" to 1),
                emails = listOf("jane@example.com"),
                addresses = emptyList(),
                organization = null,
                birthday = null,
            )
            coEvery { apiClient.uploadImportRecords(any()) } returns Result.success(
                preview(
                    ImportRowPreview(rowIndex = 0, suggestedAction = "update"),
                ),
            )

            val vm = vm(listOf(device))
            vm.load(contentResolver = mockk<ContentResolver>(relaxed = true))
            advanceUntilIdle()
            vm.toggle(1)
            vm.submitSelected()
            advanceUntilIdle()

            val state = vm.uiState.value
            assertEquals(ImportStep.REVIEW, state.step)
            assertEquals("update", state.rowActions[0])
            coVerify { apiClient.uploadImportRecords(any()) }
        }

    @Test
    fun `submitSelected with no selection is a no-op`() = runTest(mainDispatcherRule.testDispatcher) {
        val device = DeviceContact(1, "lk-1", "Jane Smith", emptyList(), emptyList(), emptyList(), null, null)
        val vm = vm(listOf(device))
        vm.load(contentResolver = mockk<ContentResolver>(relaxed = true))
        advanceUntilIdle()

        vm.submitSelected()
        advanceUntilIdle()

        assertEquals(ImportStep.LIST, vm.uiState.value.step)
        coVerify(exactly = 0) { apiClient.uploadImportRecords(any()) }
    }

    @Test
    fun `a within-batch duplicate defaults to skip on the review step`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val dup = DeviceContact(1, "lk-1", "Jane Smith", emptyList(), listOf("jane@example.com"), emptyList(), null, null)
            coEvery { apiClient.uploadImportRecords(any()) } returns Result.success(
                preview(
                    ImportRowPreview(rowIndex = 0, suggestedAction = "add"),
                    ImportRowPreview(rowIndex = 1, suggestedAction = "skip", batchDuplicateOf = 0),
                ),
            )

            val vm = vm(listOf(dup, dup.copy(contactId = 2)))
            vm.load(contentResolver = mockk<ContentResolver>(relaxed = true))
            advanceUntilIdle()
            vm.toggle(1)
            vm.toggle(2)
            vm.submitSelected()
            advanceUntilIdle()

            val state = vm.uiState.value
            assertEquals(ImportStep.REVIEW, state.step)
            assertEquals("add", state.rowActions[0])
            assertEquals("a within-batch duplicate must default to discard", "skip", state.rowActions[1])        }

    @Test
    fun `confirmImport sends every chosen action and moves to the result step`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val device = DeviceContact(1, "lk-1", "Jane Smith", emptyList(), listOf("jane@example.com"), emptyList(), null, null)
            coEvery { apiClient.uploadImportRecords(any()) } returns Result.success(
                preview(
                    ImportRowPreview(rowIndex = 0, suggestedAction = "update"),
                    ImportRowPreview(rowIndex = 1, suggestedAction = "add"),
                ),
            )
            coEvery { apiClient.confirmVcfImport(any()) } returns Result.success(ImportResult(created = 1, updated = 1))

            val vm = vm(listOf(device, device.copy(contactId = 2)))
            vm.load(contentResolver = mockk<ContentResolver>(relaxed = true))
            advanceUntilIdle()
            vm.toggle(1)
            vm.toggle(2)
            vm.submitSelected()
            advanceUntilIdle()

            vm.setRowAction(1, "skip")
            vm.confirmImport()
            advanceUntilIdle()

            val state = vm.uiState.value
            assertEquals(ImportStep.RESULT, state.step)
            assertEquals(2, state.importedCount)
            coVerify {
                apiClient.confirmVcfImport(
                    match { request ->
                        request.sessionId == "session-device" &&
                            request.actions.any { it.rowIndex == 0 && it.action == "update" } &&
                            request.actions.any { it.rowIndex == 1 && it.action == "skip" }
                    },
                )
            }
        }

    @Test
    fun `resolveAll sets every valid row to its suggested action`() = runTest(mainDispatcherRule.testDispatcher) {
        val device = DeviceContact(1, "lk-1", "Jane Smith", emptyList(), listOf("jane@example.com"), emptyList(), null, null)
        coEvery { apiClient.uploadImportRecords(any()) } returns Result.success(
            preview(
                ImportRowPreview(rowIndex = 0, suggestedAction = "update"),
                ImportRowPreview(rowIndex = 1, validationErrors = listOf("missing name"), suggestedAction = "skip"),
            ),
        )

        val vm = vm(listOf(device, device.copy(contactId = 2)))
        vm.load(contentResolver = mockk<ContentResolver>(relaxed = true))
        advanceUntilIdle()
        vm.toggle(1)
        vm.toggle(2)
        vm.submitSelected()
        advanceUntilIdle()

        vm.setRowAction(0, "skip")
        vm.resolveAll()
        advanceUntilIdle()

        val actions = vm.uiState.value.rowActions
        assertEquals("update", actions[0])
        assertEquals("an errored row stays forced to skip", "skip", actions[1])
    }

    @Test
    fun `confirmImport failure surfaces the error and stays on the review step`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val device = DeviceContact(1, "lk-1", "Jane Smith", emptyList(), emptyList(), emptyList(), null, null)
            coEvery { apiClient.uploadImportRecords(any()) } returns Result.success(
                preview(ImportRowPreview(rowIndex = 0, suggestedAction = "add")),
            )
            coEvery { apiClient.confirmVcfImport(any()) } returns Result.failure(Exception("boom"))

            val vm = vm(listOf(device))
            vm.load(contentResolver = mockk<ContentResolver>(relaxed = true))
            advanceUntilIdle()
            vm.toggle(1)
            vm.submitSelected()
            advanceUntilIdle()
            vm.confirmImport()
            advanceUntilIdle()

            assertNull(vm.uiState.value.importedCount.takeIf { it > 0 })
            assertEquals(ImportStep.REVIEW, vm.uiState.value.step)
        }
}
