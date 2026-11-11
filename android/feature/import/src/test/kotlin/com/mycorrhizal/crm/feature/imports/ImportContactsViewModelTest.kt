package com.mycorrhizal.crm.feature.imports

import android.content.ContentResolver
import android.content.Context
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.model.network.ImportConfirmRequest
import com.mycorrhizal.crm.model.network.ImportPreviewResponse
import com.mycorrhizal.crm.model.network.ImportResult
import com.mycorrhizal.crm.model.network.ImportRowEdit
import com.mycorrhizal.crm.model.network.ImportRowIssue
import com.mycorrhizal.crm.model.network.ImportRowPreview
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import io.mockk.slot
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
            appContext = mockk<Context>(relaxed = true)
        ).apply {
            readDeviceContacts = { contacts }
            ioDispatcher = mainDispatcherRule.testDispatcher
        }
    }

    private fun preview(vararg rows: ImportRowPreview) = ImportPreviewResponse(
        sessionId = "session-device",
        rows = rows.toList()
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
                birthday = null
            )
            coEvery { apiClient.uploadImportRecords(any()) } returns Result.success(
                preview(
                    ImportRowPreview(rowIndex = 0, suggestedAction = "update")
                )
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
    fun `submitSelected with no selection is a no-op`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        val device = DeviceContact(
            1,
            "lk-1",
            "Jane Smith",
            emptyList(),
            emptyList(),
            emptyList(),
            null,
            null
        )
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
            val dup = DeviceContact(
                1,
                "lk-1",
                "Jane Smith",
                emptyList(),
                listOf("jane@example.com"),
                emptyList(),
                null,
                null
            )
            coEvery { apiClient.uploadImportRecords(any()) } returns Result.success(
                preview(
                    ImportRowPreview(rowIndex = 0, suggestedAction = "add"),
                    ImportRowPreview(rowIndex = 1, suggestedAction = "skip", batchDuplicateOf = 0)
                )
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
            assertEquals(
                "a within-batch duplicate must default to discard",
                "skip",
                state.rowActions[1]
            )
        }

    @Test
    fun `confirmImport sends every chosen action and moves to the result step`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val device = DeviceContact(
                1,
                "lk-1",
                "Jane Smith",
                emptyList(),
                listOf("jane@example.com"),
                emptyList(),
                null,
                null
            )
            coEvery { apiClient.uploadImportRecords(any()) } returns Result.success(
                preview(
                    ImportRowPreview(rowIndex = 0, suggestedAction = "update"),
                    ImportRowPreview(rowIndex = 1, suggestedAction = "add")
                )
            )
            coEvery {
                apiClient.confirmVcfImport(
                    any()
                )
            } returns Result.success(ImportResult(created = 1, updated = 1))

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
            assertEquals(ImportResult(created = 1, updated = 1), state.result)
            coVerify {
                apiClient.confirmVcfImport(
                    match { request ->
                        request.sessionId == "session-device" &&
                            request.actions.any { it.rowIndex == 0 && it.action == "update" } &&
                            request.actions.any { it.rowIndex == 1 && it.action == "skip" }
                    }
                )
            }
        }

    @Test
    fun `resolveAll sets every valid row to its suggested action`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        val device = DeviceContact(
            1,
            "lk-1",
            "Jane Smith",
            emptyList(),
            listOf("jane@example.com"),
            emptyList(),
            null,
            null
        )
        coEvery { apiClient.uploadImportRecords(any()) } returns Result.success(
            preview(
                ImportRowPreview(rowIndex = 0, suggestedAction = "update"),
                ImportRowPreview(
                    rowIndex = 1,
                    validationErrors = listOf("missing name"),
                    suggestedAction = "skip"
                )
            )
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
            val device = DeviceContact(
                1,
                "lk-1",
                "Jane Smith",
                emptyList(),
                emptyList(),
                emptyList(),
                null,
                null
            )
            coEvery { apiClient.uploadImportRecords(any()) } returns Result.success(
                preview(ImportRowPreview(rowIndex = 0, suggestedAction = "add"))
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

            assertNull(vm.uiState.value.result)
            assertEquals(ImportStep.REVIEW, vm.uiState.value.step)
        }

    @Test
    fun `startOver returns to the list step and reloads the device contacts`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val device = DeviceContact(
                1,
                "lk-1",
                "Jane Smith",
                emptyList(),
                listOf("jane@example.com"),
                emptyList(),
                null,
                null
            )
            coEvery { apiClient.uploadImportRecords(any()) } returns Result.success(
                preview(ImportRowPreview(rowIndex = 0, suggestedAction = "add"))
            )
            coEvery {
                apiClient.confirmVcfImport(
                    any()
                )
            } returns Result.success(ImportResult(created = 1))

            val vm = vm(listOf(device))
            vm.load(contentResolver = mockk<ContentResolver>(relaxed = true))
            advanceUntilIdle()
            vm.toggle(1)
            vm.submitSelected()
            advanceUntilIdle()
            vm.confirmImport()
            advanceUntilIdle()
            assertEquals(ImportStep.RESULT, vm.uiState.value.step)

            vm.startOver()
            advanceUntilIdle()

            val state = vm.uiState.value
            assertEquals(ImportStep.LIST, state.step)
            assertNull(state.preview)
            assertNull(state.result)
            assertEquals(emptyMap<Int, ImportRowEdit>(), state.rowEdits)
            assertEquals(1, state.contacts.size)
        }

    // Issue #1750: a refused ContactsProvider query must become state, not a crash.
    @Test
    fun `load maps a SecurityException from the device read to permissionRequired`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = vm(
                emptyList()
            ).apply { readDeviceContacts = { throw SecurityException("READ_CONTACTS") } }
            vm.load(contentResolver = mockk<ContentResolver>(relaxed = true))
            advanceUntilIdle()

            val state = vm.uiState.value
            org.junit.Assert.assertTrue(state.permissionRequired)
            org.junit.Assert.assertFalse(state.isLoading)
            assertEquals(emptyList<DeviceContactCandidate>(), state.contacts)
        }

    @Test
    fun `a later successful load clears permissionRequired`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = vm(emptyList())
            vm.readDeviceContacts = { throw SecurityException("READ_CONTACTS") }
            vm.load(contentResolver = mockk<ContentResolver>(relaxed = true))
            advanceUntilIdle()
            vm.readDeviceContacts = { emptyList() }
            vm.load(contentResolver = mockk<ContentResolver>(relaxed = true))
            advanceUntilIdle()

            org.junit.Assert.assertFalse(vm.uiState.value.permissionRequired)
        }

    // Issue #2464/#2210: the device-contacts review gets the same "Fix this row" flow as VCF/CSV.
    private fun errorRowPreview() = preview(
        ImportRowPreview(rowIndex = 0, suggestedAction = "add"),
        ImportRowPreview(
            rowIndex = 1,
            suggestedAction = "add",
            validationErrors = listOf("name required")
        )
    )

    private suspend fun kotlinx.coroutines.test.TestScope.toReview(vm: ImportContactsViewModel) {
        vm.load(contentResolver = mockk<ContentResolver>(relaxed = true))
        advanceUntilIdle()
        vm.toggle(1)
        vm.toggle(2)
        vm.submitSelected()
        advanceUntilIdle()
    }

    private val twoDevices = listOf(
        DeviceContact(1, "lk-1", "Jane", emptyList(), emptyList(), emptyList(), null, null),
        DeviceContact(2, "lk-2", "", emptyList(), emptyList(), emptyList(), null, null)
    )

    @Test
    fun `saving a fix keeps the errored row and sends only that edit with the confirm`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.uploadImportRecords(any()) } returns
                Result.success(errorRowPreview())
            val sent = slot<ImportConfirmRequest>()
            coEvery { apiClient.confirmVcfImport(capture(sent)) } returns
                Result.success(ImportResult())
            val vm = vm(twoDevices)
            toReview(vm)
            assertEquals("skip", vm.uiState.value.rowActions[1])

            val edit = ImportRowEdit(rowIndex = 1, firstname = "Dana")
            vm.saveRowEdit(edit)
            assertEquals("add", vm.uiState.value.rowActions[1])
            assertEquals(edit, vm.uiState.value.rowEdits[1])
            vm.confirmImport()
            advanceUntilIdle()

            assertEquals(listOf(edit), sent.captured.rowEdits)
        }

    @Test
    fun `an edit for a row that is skipped again is not sent and no edits sends null`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.uploadImportRecords(any()) } returns
                Result.success(errorRowPreview())
            val sent = slot<ImportConfirmRequest>()
            coEvery { apiClient.confirmVcfImport(capture(sent)) } returns
                Result.success(ImportResult())
            val vm = vm(twoDevices)
            toReview(vm)

            vm.saveRowEdit(ImportRowEdit(rowIndex = 1, firstname = "Dana"))
            vm.setRowAction(1, "skip")
            vm.confirmImport()
            advanceUntilIdle()

            assertNull(sent.captured.rowEdits)
        }

    @Test
    fun `saving a fix for a row without errors is ignored and an unfixed error row stays skip`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.uploadImportRecords(any()) } returns
                Result.success(errorRowPreview())
            val vm = vm(twoDevices)
            toReview(vm)

            vm.saveRowEdit(ImportRowEdit(rowIndex = 0, firstname = "X"))
            vm.setRowAction(1, "add")

            assertEquals(emptyMap<Int, ImportRowEdit>(), vm.uiState.value.rowEdits)
            assertEquals("skip", vm.uiState.value.rowActions[1])
        }

    @Test
    fun `resolveAll does not unskip a fixed row`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { apiClient.uploadImportRecords(any()) } returns Result.success(errorRowPreview())
        val vm = vm(twoDevices)
        toReview(vm)
        vm.saveRowEdit(ImportRowEdit(rowIndex = 1, firstname = "Dana"))
        vm.setRowAction(1, "skip")

        vm.resolveAll()

        assertEquals("skip", vm.uiState.value.rowActions[1])
    }

    @Test
    fun `the result state carries the per-row issues from the confirm response`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val issues = listOf(
                ImportRowIssue(row = 2, name = "Jane", status = "failed", reason = "name required")
            )
            coEvery { apiClient.uploadImportRecords(any()) } returns
                Result.success(errorRowPreview())
            coEvery { apiClient.confirmVcfImport(any()) } returns
                Result.success(ImportResult(created = 1, skipped = 1, issues = issues))
            val vm = vm(twoDevices)
            toReview(vm)

            vm.confirmImport()
            advanceUntilIdle()

            assertEquals(ImportStep.RESULT, vm.uiState.value.step)
            assertEquals(issues, vm.uiState.value.result?.issues)
        }
}
