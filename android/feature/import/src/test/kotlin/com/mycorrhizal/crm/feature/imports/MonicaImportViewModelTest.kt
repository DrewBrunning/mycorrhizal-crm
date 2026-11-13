package com.mycorrhizal.crm.feature.imports

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.ViewModelStore
import com.mycorrhizal.crm.domain.backup.BundleBackupStatus
import com.mycorrhizal.crm.domain.repository.BundleBackupRepository
import com.mycorrhizal.crm.model.network.ImportConfirmRequest
import com.mycorrhizal.crm.model.network.ImportRowIssue
import com.mycorrhizal.crm.model.network.ImportRowPreview
import com.mycorrhizal.crm.model.network.MonicaConnectRequest
import com.mycorrhizal.crm.model.network.MonicaConnectResponse
import com.mycorrhizal.crm.model.network.MonicaEntityCounts
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
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

/** Issues #549/#2525: the connect -> fetch -> poll -> preview -> confirm -> poll Monica state machine. */
class MonicaImportViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val apiClient = mockk<ApiClient>()

    private fun newVm(local: Boolean = false): MonicaImportViewModel {
        val backup = mockk<BundleBackupRepository>()
        coEvery { backup.observeStatus() } returns
            flowOf(BundleBackupStatus(isLocalProfile = local))
        return MonicaImportViewModel(apiClient, backup).also { it.pollIntervalMillis = 10L }
    }

    private fun connected() = MonicaConnectResponse(
        sessionId = "s1",
        totals = MonicaEntityCounts(contacts = 12, activities = 4, notes = 30),
        estimatedFetchSeconds = 95
    )

    private fun status(phase: String, error: String? = null, result: SourceImportResult? = null) =
        SourceImportStatus(sessionId = "s1", phase = phase, error = error, result = result)

    private fun row(index: Int, action: String = "add", errors: List<String> = emptyList()) =
        ImportRowPreview(rowIndex = index, suggestedAction = action, validationErrors = errors)

    private val preview = SourceImportPreviewResponse(
        sessionId = "s1",
        rows = listOf(row(0), row(1, action = "update"), row(2, errors = listOf("bad"))),
        totalRows = 3
    )

    private fun MonicaImportViewModel.fillForm() {
        onBaseUrlChange("  https://monica.example  ")
        onApiTokenChange("tok")
    }

    @Test
    fun `connect needs both an address and a token`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = newVm()
        assertFalse(vm.uiState.value.canConnect)
        vm.onBaseUrlChange("   ")
        vm.onApiTokenChange("t")
        assertFalse(vm.uiState.value.canConnect)
        vm.onBaseUrlChange("https://m")
        assertTrue(vm.uiState.value.canConnect)
        vm.onApiTokenChange("")
        assertFalse(vm.uiState.value.canConnect)
    }

    @Test
    fun `both fetch options default on and can be toggled`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = newVm()
            assertTrue(vm.uiState.value.includeRelationships)
            assertTrue(vm.uiState.value.includeExtras)

            vm.onIncludeRelationshipsChange(false)
            vm.onIncludeExtrasChange(false)

            assertFalse(vm.uiState.value.includeRelationships)
            assertFalse(vm.uiState.value.includeExtras)
        }

    @Test
    fun `connecting trims the address, sends the token once and clears it from state`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val request = slot<MonicaConnectRequest>()
            coEvery { apiClient.connectMonica(capture(request)) } returns
                Result.success(connected())
            val vm = newVm()
            vm.fillForm()

            vm.connect()
            advanceUntilIdle()

            assertEquals(MonicaConnectRequest("https://monica.example", "tok"), request.captured)
            val state = vm.uiState.value
            assertEquals("s1", state.connected?.sessionId)
            assertEquals("", state.apiToken)
            assertFalse(state.isConnecting)
            assertTrue(state.canStart)
        }

    @Test
    fun `a second connect tap while connecting sends one request`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.connectMonica(any()) } returns Result.success(connected())
            val vm = newVm()
            vm.fillForm()

            vm.connect()
            vm.connect()
            advanceUntilIdle()

            coVerify(exactly = 1) { apiClient.connectMonica(any()) }
        }

    @Test
    fun `a rejected token shows the typed error and keeps the form editable`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.connectMonica(any()) } returns
                Result.failure(ApiError.Client(400, "Monica rejected the API token"))
            val vm = newVm()
            vm.fillForm()

            vm.connect()
            advanceUntilIdle()

            val state = vm.uiState.value
            assertNull(state.connected)
            assertEquals("Monica rejected the API token", state.error?.message)
            assertFalse(state.isConnecting)
            // The token was kept on failure so a typo can be corrected without retyping.
            assertEquals("tok", state.apiToken)
            assertTrue(state.canConnect)
        }

    @Test
    fun `an unreachable Monica surfaces the server error`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.connectMonica(any()) } returns
                Result.failure(ApiError.Server(502, "The Monica instance could not be reached"))
            val vm = newVm()
            vm.fillForm()

            vm.connect()
            advanceUntilIdle()

            assertTrue(vm.uiState.value.error is ApiError.Server)
            assertEquals(MonicaImportStep.CONNECT, vm.uiState.value.step)
        }

    @Test
    fun `dismissing the error clears both the typed error and the job text`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.connectMonica(any()) } returns
                Result.failure(ApiError.Client(400, "nope"))
            val vm = newVm()
            vm.fillForm()
            vm.connect()
            advanceUntilIdle()
            assertTrue(vm.uiState.value.error != null)

            vm.onErrorShown()

            assertNull(vm.uiState.value.error)
            assertNull(vm.uiState.value.errorText)
            assertNull(vm.uiState.value.errorRes)
        }

    @Test
    fun `the form is locked once connected`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { apiClient.connectMonica(any()) } returns Result.success(connected())
        val vm = newVm()
        vm.fillForm()
        vm.connect()
        advanceUntilIdle()

        vm.onBaseUrlChange("https://other")
        vm.onApiTokenChange("other")
        vm.onIncludeExtrasChange(false)

        val state = vm.uiState.value
        assertEquals("https://monica.example", state.baseUrl.trim())
        assertEquals("", state.apiToken)
        assertTrue(state.includeExtras)
        assertFalse(state.canConnect)
    }

    @Test
    fun `starting sends the chosen fetch options, polls to ready and lands on review`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.connectMonica(any()) } returns Result.success(connected())
            coEvery { apiClient.startMonicaFetch("s1", false, true) } returns Result.success(Unit)
            coEvery { apiClient.getMonicaImportStatus("s1") } returnsMany listOf(
                Result.success(status("fetching_contacts")),
                Result.success(status("building_preview")),
                Result.success(status("ready"))
            )
            coEvery { apiClient.getMonicaImportPreview("s1") } returns Result.success(preview)
            val vm = newVm()
            vm.fillForm()
            vm.onIncludeRelationshipsChange(false)
            vm.connect()
            advanceUntilIdle()

            vm.startImport()
            advanceUntilIdle()

            val state = vm.uiState.value
            assertEquals(MonicaImportStep.REVIEW, state.step)
            assertFalse(state.isImporting)
            assertEquals(mapOf(0 to "add", 1 to "update", 2 to "skip"), state.rowActions)
            coVerify(exactly = 3) { apiClient.getMonicaImportStatus("s1") }
        }

    @Test
    fun `starting is a no-op before connecting and a second tap does not fetch twice`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.connectMonica(any()) } returns Result.success(connected())
            coEvery { apiClient.startMonicaFetch(any(), any(), any()) } returns Result.success(Unit)
            coEvery { apiClient.getMonicaImportStatus("s1") } returns
                Result.success(status("ready"))
            coEvery { apiClient.getMonicaImportPreview("s1") } returns Result.success(preview)
            val vm = newVm()
            vm.startImport()
            coVerify(exactly = 0) { apiClient.startMonicaFetch(any(), any(), any()) }
            vm.fillForm()
            vm.connect()
            advanceUntilIdle()

            vm.startImport()
            vm.startImport()
            advanceUntilIdle()

            coVerify(exactly = 1) { apiClient.startMonicaFetch("s1", true, true) }
        }

    @Test
    fun `a fetch the server refused returns to connect with the error and drops the session`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.connectMonica(any()) } returns Result.success(connected())
            coEvery { apiClient.startMonicaFetch(any(), any(), any()) } returns
                Result.failure(ApiError.Client(409, "A fetch is already running"))
            coEvery { apiClient.cancelMonicaImport("s1") } returns Result.success(Unit)
            val vm = newVm()
            vm.fillForm()
            vm.connect()
            advanceUntilIdle()

            vm.startImport()
            advanceUntilIdle()

            val state = vm.uiState.value
            assertEquals(MonicaImportStep.CONNECT, state.step)
            assertNull(state.connected)
            assertEquals("A fetch is already running", state.error?.message)
            coVerify(exactly = 1) { apiClient.cancelMonicaImport("s1") }
        }

    @Test
    fun `a failed fetch returns to the connect form, shows the job error and cancels once`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.connectMonica(any()) } returns Result.success(connected())
            coEvery { apiClient.startMonicaFetch(any(), any(), any()) } returns Result.success(Unit)
            coEvery { apiClient.getMonicaImportStatus("s1") } returns
                Result.success(status("failed", error = "The Monica instance could not be reached"))
            coEvery { apiClient.cancelMonicaImport("s1") } returns Result.success(Unit)
            val vm = newVm()
            vm.fillForm()
            vm.connect()
            advanceUntilIdle()

            vm.startImport()
            advanceUntilIdle()

            val state = vm.uiState.value
            assertEquals(MonicaImportStep.CONNECT, state.step)
            assertNull(state.connected)
            assertEquals("The Monica instance could not be reached", state.errorText)
            coVerify(exactly = 1) { apiClient.cancelMonicaImport("s1") }
            vm.leaveScreen()
            advanceUntilIdle()
            coVerify(exactly = 1) { apiClient.cancelMonicaImport("s1") }
        }

    @Test
    fun `a failed fetch with no message falls back to the generic string`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.connectMonica(any()) } returns Result.success(connected())
            coEvery { apiClient.startMonicaFetch(any(), any(), any()) } returns Result.success(Unit)
            coEvery { apiClient.getMonicaImportStatus("s1") } returns
                Result.failure(RuntimeException("net"))
            coEvery { apiClient.cancelMonicaImport("s1") } returns Result.success(Unit)
            val vm = newVm()
            vm.fillForm()
            vm.connect()
            advanceUntilIdle()

            vm.startImport()
            advanceUntilIdle()

            assertEquals(R.string.monica_import_error_failed, vm.uiState.value.errorRes)
        }

    private fun TestScope.reviewVm(): MonicaImportViewModel {
        coEvery { apiClient.connectMonica(any()) } returns Result.success(connected())
        coEvery { apiClient.startMonicaFetch("s1", true, true) } returns Result.success(Unit)
        coEvery { apiClient.getMonicaImportPreview("s1") } returns Result.success(preview)
        coEvery { apiClient.getMonicaImportStatus("s1") } returns Result.success(status("ready"))
        val vm = newVm()
        vm.fillForm()
        vm.connect()
        advanceUntilIdle()
        vm.startImport()
        advanceUntilIdle()
        return vm
    }

    @Test
    fun `row actions can be changed except on an errored row, and resolve all resets them`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = reviewVm()

            vm.setRowAction(0, "skip")
            vm.setRowAction(1, "add")
            vm.setRowAction(2, "add")
            assertEquals("skip", vm.uiState.value.rowActions[0])
            assertEquals("add", vm.uiState.value.rowActions[1])
            assertEquals("skip", vm.uiState.value.rowActions[2])

            vm.resolveAll()
            assertEquals(mapOf(0 to "add", 1 to "update", 2 to "skip"), vm.uiState.value.rowActions)
        }

    @Test
    fun `confirming sends the per-row merge and skip choices, polls to done and shows the result`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = reviewVm()
            vm.setRowAction(0, "skip")
            val request = slot<ImportConfirmRequest>()
            coEvery { apiClient.confirmMonicaImport(capture(request)) } returns Result.success(Unit)
            coEvery { apiClient.getMonicaImportStatus("s1") } returnsMany listOf(
                Result.success(status("importing")),
                Result.success(
                    status("done", result = SourceImportResult(created = 2, skipped = 1))
                )
            )

            vm.confirm()
            advanceUntilIdle()

            assertEquals("s1", request.captured.sessionId)
            assertEquals(
                setOf(
                    RowImportAction(0, "skip"),
                    RowImportAction(1, "update"),
                    RowImportAction(2, "skip")
                ),
                request.captured.actions.toSet()
            )
            val state = vm.uiState.value
            assertEquals(MonicaImportStep.RESULT, state.step)
            assertEquals(2, state.result?.created)
        }

    @Test
    fun `a poll timeout after the import committed shows the committed result`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = reviewVm()
            vm.maxPolls = 4
            val committed = SourceImportResult(created = 2, skipped = 1)
            coEvery { apiClient.confirmMonicaImport(any()) } returns Result.success(Unit)
            coEvery { apiClient.getMonicaImportStatus("s1") } returnsMany listOf(
                Result.success(status("importing")),
                Result.success(status("importing_photos", result = committed)),
                Result.success(status("importing_photos", result = committed)),
                Result.success(status("importing_photos", result = committed))
            )

            vm.confirm()
            advanceUntilIdle()

            val state = vm.uiState.value
            assertEquals(MonicaImportStep.RESULT, state.step)
            assertEquals(committed, state.result)
            assertNull(state.error)
        }

    @Test
    fun `a poll timeout before anything committed returns to review`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = reviewVm()
            vm.maxPolls = 3
            coEvery { apiClient.confirmMonicaImport(any()) } returns Result.success(Unit)
            coEvery { apiClient.getMonicaImportStatus("s1") } returns
                Result.success(status("importing"))

            vm.confirm()
            advanceUntilIdle()

            assertEquals(MonicaImportStep.REVIEW, vm.uiState.value.step)
            assertNull(vm.uiState.value.result)
        }

    @Test
    fun `an import that fails or is cancelled returns to review with the job message`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = reviewVm()
            coEvery { apiClient.confirmMonicaImport(any()) } returns Result.success(Unit)
            coEvery { apiClient.getMonicaImportStatus("s1") } returns
                Result.success(status("failed", error = "The import could not be applied"))

            vm.confirm()
            advanceUntilIdle()

            assertEquals(MonicaImportStep.REVIEW, vm.uiState.value.step)
            assertEquals("The import could not be applied", vm.uiState.value.errorText)
            assertFalse(vm.uiState.value.isImporting)
        }

    @Test
    fun `a failed confirm returns to review with the message`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = reviewVm()
            coEvery { apiClient.confirmMonicaImport(any()) } returns
                Result.failure(ApiError.Client(409, "Already running"))

            vm.confirm()
            advanceUntilIdle()

            assertEquals(MonicaImportStep.REVIEW, vm.uiState.value.step)
            assertEquals("Already running", vm.uiState.value.error?.message)
        }

    /** Clears [this] through a [ViewModelStore], exactly as popping its nav destination does. */
    @Suppress("UNCHECKED_CAST")
    private fun MonicaImportViewModel.leaveScreen() {
        val self = this
        val store = ViewModelStore()
        ViewModelProvider(
            store,
            object : ViewModelProvider.Factory {
                override fun <T : ViewModel> create(modelClass: Class<T>): T = self as T
            }
        )[MonicaImportViewModel::class.java]
        store.clear()
    }

    @Test
    fun `leaving the screen after connect or from review sends one cancel and before connect none`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.cancelMonicaImport("s1") } returns Result.success(Unit)
            newVm().leaveScreen()
            advanceUntilIdle()
            coVerify(exactly = 0) { apiClient.cancelMonicaImport(any()) }

            reviewVm().leaveScreen()
            advanceUntilIdle()

            coVerify(exactly = 1) { apiClient.cancelMonicaImport("s1") }
        }

    @Test
    fun `leaving mid-fetch cancels the session`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { apiClient.connectMonica(any()) } returns Result.success(connected())
        coEvery { apiClient.startMonicaFetch(any(), any(), any()) } returns Result.success(Unit)
        coEvery { apiClient.getMonicaImportStatus("s1") } returns
            Result.success(status("fetching_notes"))
        coEvery { apiClient.cancelMonicaImport("s1") } returns Result.success(Unit)
        val vm = newVm()
        vm.fillForm()
        vm.connect()
        advanceUntilIdle()
        vm.startImport()
        advanceTimeBy(50)

        vm.leaveScreen()
        advanceUntilIdle()

        coVerify(exactly = 1) { apiClient.cancelMonicaImport("s1") }
    }

    @Test
    fun `leaving after a confirm does not cancel the import the user just confirmed`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = reviewVm()
            coEvery { apiClient.confirmMonicaImport(any()) } returns Result.success(Unit)
            coEvery { apiClient.cancelMonicaImport(any()) } returns Result.success(Unit)
            coEvery { apiClient.getMonicaImportStatus("s1") } returns
                Result.success(status("importing"))

            vm.confirm()
            advanceTimeBy(50)
            assertEquals(MonicaImportStep.WORKING, vm.uiState.value.step)
            vm.leaveScreen()
            advanceUntilIdle()

            coVerify(exactly = 0) { apiClient.cancelMonicaImport(any()) }
        }

    @Test
    fun `leaving while the confirm request is in flight does not cancel the import being confirmed`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = reviewVm()
            val gate = CompletableDeferred<Unit>()
            coEvery { apiClient.confirmMonicaImport(any()) } coAnswers {
                gate.await()
                Result.success(Unit)
            }
            coEvery { apiClient.cancelMonicaImport(any()) } returns Result.success(Unit)
            coEvery { apiClient.getMonicaImportStatus("s1") } returns
                Result.success(status("importing"))

            vm.confirm()
            runCurrent()
            vm.leaveScreen()
            gate.complete(Unit)
            advanceUntilIdle()

            coVerify(exactly = 0) { apiClient.cancelMonicaImport(any()) }
        }

    @Test
    fun `a session that connects after the screen was left is cancelled`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val gate = CompletableDeferred<Unit>()
            coEvery { apiClient.connectMonica(any()) } coAnswers {
                gate.await()
                Result.success(connected())
            }
            coEvery { apiClient.cancelMonicaImport(any()) } returns Result.success(Unit)
            val vm = newVm()
            vm.fillForm()

            vm.connect()
            runCurrent()
            vm.leaveScreen()
            gate.complete(Unit)
            advanceUntilIdle()

            coVerify(exactly = 1) { apiClient.cancelMonicaImport("s1") }
        }

    @Test
    fun `a connect that fails after the screen was left sends no cancel`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val gate = CompletableDeferred<Unit>()
            coEvery { apiClient.connectMonica(any()) } coAnswers {
                gate.await()
                Result.failure(ApiError.Client(401, "bad token"))
            }
            coEvery { apiClient.cancelMonicaImport(any()) } returns Result.success(Unit)
            val vm = newVm()
            vm.fillForm()

            vm.connect()
            runCurrent()
            vm.leaveScreen()
            gate.complete(Unit)
            advanceUntilIdle()

            coVerify(exactly = 0) { apiClient.cancelMonicaImport(any()) }
        }

    @Test
    fun `leaving the result screen does not cancel`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = reviewVm()
        coEvery { apiClient.confirmMonicaImport(any()) } returns Result.success(Unit)
        coEvery { apiClient.cancelMonicaImport(any()) } returns Result.success(Unit)
        coEvery { apiClient.getMonicaImportStatus("s1") } returns
            Result.success(status("done", result = SourceImportResult(created = 1)))
        vm.confirm()
        advanceUntilIdle()

        vm.leaveScreen()
        advanceUntilIdle()

        coVerify(exactly = 0) { apiClient.cancelMonicaImport(any()) }
    }

    @Test
    fun `a rejected confirm leaves the session cancellable`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = reviewVm()
            coEvery { apiClient.confirmMonicaImport(any()) } returns
                Result.failure(ApiError.Client(409, "Already running"))
            coEvery { apiClient.cancelMonicaImport("s1") } returns Result.success(Unit)
            vm.confirm()
            advanceUntilIdle()

            vm.leaveScreen()
            advanceUntilIdle()

            coVerify(exactly = 1) { apiClient.cancelMonicaImport("s1") }
        }

    @Test
    fun `a Local profile marks the import unavailable`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = newVm(local = true)
            advanceUntilIdle()
            assertTrue(vm.uiState.value.unavailable)
            assertFalse(newVm(local = false).also { advanceUntilIdle() }.uiState.value.unavailable)
        }

    @Test
    fun `the result state carries the per-row issues`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = reviewVm()
            val issues = listOf(
                ImportRowIssue(row = 3, name = "Ada", status = "skipped", reason = "already exists")
            )
            coEvery { apiClient.confirmMonicaImport(any()) } returns Result.success(Unit)
            coEvery { apiClient.getMonicaImportStatus("s1") } returns Result.success(
                status("done", result = SourceImportResult(skipped = 1, issues = issues))
            )

            vm.confirm()
            advanceUntilIdle()

            assertEquals(issues, vm.uiState.value.result?.issues)
        }
}
