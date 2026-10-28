package com.mycorrhizal.crm.feature.imports

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.ViewModelStore
import com.mycorrhizal.crm.domain.backup.BundleBackupStatus
import com.mycorrhizal.crm.domain.repository.BundleBackupRepository
import com.mycorrhizal.crm.model.network.ImportConfirmRequest
import com.mycorrhizal.crm.model.network.ImportRowPreview
import com.mycorrhizal.crm.model.network.MeerkatConnectRequest
import com.mycorrhizal.crm.model.network.MeerkatConnectResponse
import com.mycorrhizal.crm.model.network.MeerkatEntityCounts
import com.mycorrhizal.crm.model.network.MeerkatVersionInfo
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
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

/** Issue #2013: the connect → fetch → poll → preview → confirm → poll Meerkat state machine. */
class MeerkatImportViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val apiClient = mockk<ApiClient>()

    private fun newVm(local: Boolean = false): MeerkatImportViewModel {
        val backup = mockk<BundleBackupRepository>()
        coEvery { backup.observeStatus() } returns
            flowOf(BundleBackupStatus(isLocalProfile = local))
        return MeerkatImportViewModel(apiClient, backup).also { it.pollIntervalMillis = 10L }
    }

    private fun connected(supported: Boolean = true, insecure: Boolean = false) =
        MeerkatConnectResponse(
            sessionId = "s1",
            version = MeerkatVersionInfo("v1.5-v1.7", supported, if (supported) "" else "newer"),
            totals = MeerkatEntityCounts(contacts = 12),
            insecureTransport = insecure
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

    private fun MeerkatImportViewModel.fillToken() {
        onBaseUrlChange("  https://meerkat.example  ")
        onApiTokenChange("tok")
    }

    @Test
    fun `connect needs an address and a credential for the chosen mode`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = newVm()
            assertFalse(vm.uiState.value.canConnect)
            vm.onBaseUrlChange("https://m")
            assertFalse(vm.uiState.value.canConnect)
            vm.onApiTokenChange("t")
            assertTrue(vm.uiState.value.canConnect)
            vm.onAuthModeChange(MeerkatAuthMode.PASSWORD)
            assertFalse(vm.uiState.value.canConnect)
            vm.onUsernameChange("me")
            vm.onPasswordChange("pw")
            assertTrue(vm.uiState.value.canConnect)
        }

    @Test
    fun `connecting with a token sends only the token, trims the address and clears the secret`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val request = slot<MeerkatConnectRequest>()
            coEvery { apiClient.connectMeerkat(capture(request)) } returns
                Result.success(connected())
            val vm = newVm()
            vm.fillToken()

            vm.connect()
            advanceUntilIdle()

            assertEquals(
                MeerkatConnectRequest("https://meerkat.example", apiToken = "tok"),
                request.captured
            )
            val state = vm.uiState.value
            assertEquals("s1", state.connected?.sessionId)
            assertEquals("", state.apiToken)
            assertFalse(state.isConnecting)
        }

    @Test
    fun `connecting with a username and password sends them and clears the password`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val request = slot<MeerkatConnectRequest>()
            coEvery { apiClient.connectMeerkat(capture(request)) } returns
                Result.success(connected())
            val vm = newVm()
            vm.onBaseUrlChange("https://m")
            vm.onAuthModeChange(MeerkatAuthMode.PASSWORD)
            vm.onUsernameChange(" me ")
            vm.onPasswordChange("pw")

            vm.connect()
            advanceUntilIdle()

            assertEquals(
                MeerkatConnectRequest("https://m", username = "me", password = "pw"),
                request.captured
            )
            assertEquals("", vm.uiState.value.password)
        }

    @Test
    fun `a rejected connect shows the server message and keeps the form editable`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.connectMeerkat(any()) } returns
                Result.failure(ApiError.Client(400, "Those credentials were rejected"))
            val vm = newVm()
            vm.fillToken()

            vm.connect()
            advanceUntilIdle()

            val state = vm.uiState.value
            assertNull(state.connected)
            assertEquals("Those credentials were rejected", state.error?.message)
            assertTrue(state.canConnect)
        }

    @Test
    fun `the form is locked once connected`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { apiClient.connectMeerkat(any()) } returns Result.success(connected())
        val vm = newVm()
        vm.fillToken()
        vm.connect()
        advanceUntilIdle()

        vm.onBaseUrlChange("https://other")

        assertEquals("https://meerkat.example", vm.uiState.value.baseUrl.trim())
        assertFalse(vm.uiState.value.canConnect)
    }

    @Test
    fun `an untested version cannot start until try anyway is accepted, then sends it`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.connectMeerkat(any()) } returns
                Result.success(connected(supported = false))
            coEvery { apiClient.startMeerkatFetch("s1", true) } returns Result.success(Unit)
            coEvery { apiClient.getMeerkatImportStatus("s1") } returns
                Result.success(status("ready"))
            coEvery { apiClient.getMeerkatImportPreview("s1") } returns Result.success(preview)
            val vm = newVm()
            vm.fillToken()
            vm.connect()
            advanceUntilIdle()

            assertFalse(vm.uiState.value.canStart)
            vm.startImport()
            advanceUntilIdle()
            coVerify(exactly = 0) { apiClient.startMeerkatFetch(any(), any()) }

            vm.onAcceptUntestedChange(true)
            vm.startImport()
            advanceUntilIdle()

            coVerify(exactly = 1) { apiClient.startMeerkatFetch("s1", true) }
            assertEquals(MeerkatImportStep.REVIEW, vm.uiState.value.step)
        }

    @Test
    fun `a supported version fetches, polls to ready and lands on review with suggested actions`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.connectMeerkat(any()) } returns Result.success(connected())
            coEvery { apiClient.startMeerkatFetch("s1", false) } returns Result.success(Unit)
            coEvery { apiClient.getMeerkatImportStatus("s1") } returnsMany listOf(
                Result.success(status("fetching")),
                Result.success(status("building_preview")),
                Result.success(status("ready"))
            )
            coEvery { apiClient.getMeerkatImportPreview("s1") } returns Result.success(preview)
            val vm = newVm()
            vm.fillToken()
            vm.connect()
            advanceUntilIdle()

            vm.startImport()
            advanceUntilIdle()

            val state = vm.uiState.value
            assertEquals(MeerkatImportStep.REVIEW, state.step)
            assertFalse(state.isImporting)
            assertEquals(mapOf(0 to "add", 1 to "update", 2 to "skip"), state.rowActions)
            coVerify(exactly = 3) { apiClient.getMeerkatImportStatus("s1") }
        }

    @Test
    fun `a failed fetch returns to the connect form and asks to connect again`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.connectMeerkat(any()) } returns Result.success(connected())
            coEvery { apiClient.startMeerkatFetch("s1", false) } returns Result.success(Unit)
            coEvery { apiClient.getMeerkatImportStatus("s1") } returns
                Result.success(status("failed", error = "Meerkat went away"))
            coEvery { apiClient.cancelMeerkatImport("s1") } returns Result.success(Unit)
            val vm = newVm()
            vm.fillToken()
            vm.connect()
            advanceUntilIdle()

            vm.startImport()
            advanceUntilIdle()

            val state = vm.uiState.value
            assertEquals(MeerkatImportStep.CONNECT, state.step)
            assertNull(state.connected)
            assertEquals("Meerkat went away", state.errorText)
        }

    @Test
    fun `a failed fetch cancels the abandoned session once, and leaving later does not cancel again`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.connectMeerkat(any()) } returns Result.success(connected())
            coEvery { apiClient.startMeerkatFetch("s1", false) } returns Result.success(Unit)
            coEvery { apiClient.getMeerkatImportStatus("s1") } returns
                Result.success(status("failed", error = "Meerkat went away"))
            coEvery { apiClient.cancelMeerkatImport("s1") } returns Result.success(Unit)
            val vm = newVm()
            vm.fillToken()
            vm.connect()
            advanceUntilIdle()

            vm.startImport()
            advanceUntilIdle()

            coVerify(exactly = 1) { apiClient.cancelMeerkatImport("s1") }
            vm.leaveScreen()
            advanceUntilIdle()
            coVerify(exactly = 1) { apiClient.cancelMeerkatImport("s1") }
        }

    @Test
    fun `a failed fetch with no message falls back to the generic string`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.connectMeerkat(any()) } returns Result.success(connected())
            coEvery { apiClient.startMeerkatFetch("s1", false) } returns Result.success(Unit)
            coEvery { apiClient.getMeerkatImportStatus("s1") } returns
                Result.failure(RuntimeException("net"))
            coEvery { apiClient.cancelMeerkatImport("s1") } returns Result.success(Unit)
            val vm = newVm()
            vm.fillToken()
            vm.connect()
            advanceUntilIdle()

            vm.startImport()
            advanceUntilIdle()

            assertEquals(R.string.meerkat_import_error_failed, vm.uiState.value.errorRes)
        }

    private fun TestScope.reviewVm(): MeerkatImportViewModel {
        coEvery { apiClient.connectMeerkat(any()) } returns Result.success(connected())
        coEvery { apiClient.startMeerkatFetch("s1", false) } returns Result.success(Unit)
        coEvery { apiClient.getMeerkatImportPreview("s1") } returns Result.success(preview)
        coEvery { apiClient.getMeerkatImportStatus("s1") } returns Result.success(status("ready"))
        val vm = newVm()
        vm.fillToken()
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
            vm.setRowAction(2, "add")
            assertEquals("skip", vm.uiState.value.rowActions[0])
            assertEquals("skip", vm.uiState.value.rowActions[2])

            vm.resolveAll()
            assertEquals(mapOf(0 to "add", 1 to "update", 2 to "skip"), vm.uiState.value.rowActions)
        }

    @Test
    fun `confirming sends the per-row actions, polls to done and shows the result`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = reviewVm()
            val request = slot<ImportConfirmRequest>()
            coEvery {
                apiClient.confirmMeerkatImport(
                    capture(request)
                )
            } returns Result.success(Unit)
            coEvery { apiClient.getMeerkatImportStatus("s1") } returnsMany listOf(
                Result.success(status("importing")),
                Result.success(
                    status("done", result = SourceImportResult(created = 2, skipped = 1))
                )
            )

            vm.confirm()
            advanceUntilIdle()

            assertEquals("s1", request.captured.sessionId)
            assertTrue(RowImportAction(1, "update") in request.captured.actions)
            val state = vm.uiState.value
            assertEquals(MeerkatImportStep.RESULT, state.step)
            assertEquals(2, state.result?.created)
        }

    @Test
    fun `a poll timeout after the import committed shows the committed result`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = reviewVm()
            vm.maxPolls = 5
            val committed = SourceImportResult(created = 2, skipped = 1)
            coEvery { apiClient.confirmMeerkatImport(any()) } returns Result.success(Unit)
            coEvery { apiClient.getMeerkatImportStatus("s1") } returnsMany listOf(
                Result.success(status("importing")),
                Result.success(status("importing_photos", result = committed)),
                Result.success(status("importing_photos", result = committed)),
                Result.success(status("importing_photos", result = committed)),
                Result.success(status("importing_photos", result = committed))
            )

            vm.confirm()
            advanceUntilIdle()

            val state = vm.uiState.value
            assertEquals(MeerkatImportStep.RESULT, state.step)
            assertEquals(committed, state.result)
            assertFalse(state.isImporting)
            assertNull(state.error)
            assertNull(state.errorRes)
        }

    @Test
    fun `a failed poll after the import committed shows the committed result`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = reviewVm()
            val committed = SourceImportResult(created = 3)
            coEvery { apiClient.confirmMeerkatImport(any()) } returns Result.success(Unit)
            coEvery { apiClient.getMeerkatImportStatus("s1") } returnsMany listOf(
                Result.success(status("importing_photos", result = committed)),
                Result.failure(RuntimeException("net"))
            )

            vm.confirm()
            advanceUntilIdle()

            assertEquals(MeerkatImportStep.RESULT, vm.uiState.value.step)
            assertEquals(committed, vm.uiState.value.result)
        }

    @Test
    fun `a poll timeout before anything committed still returns to review`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = reviewVm()
            vm.maxPolls = 3
            coEvery { apiClient.confirmMeerkatImport(any()) } returns Result.success(Unit)
            coEvery { apiClient.getMeerkatImportStatus("s1") } returns
                Result.success(status("importing"))

            vm.confirm()
            advanceUntilIdle()

            assertEquals(MeerkatImportStep.REVIEW, vm.uiState.value.step)
            assertNull(vm.uiState.value.result)
        }

    @Test
    fun `a failed confirm returns to review with the message`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = reviewVm()
            coEvery { apiClient.confirmMeerkatImport(any()) } returns
                Result.failure(ApiError.Client(409, "Already running"))

            vm.confirm()
            advanceUntilIdle()

            assertEquals(MeerkatImportStep.REVIEW, vm.uiState.value.step)
            assertEquals("Already running", vm.uiState.value.error?.message)
        }

    @Test
    fun `cancel drops the session unless the import already finished`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.cancelMeerkatImport("s1") } returns Result.success(Unit)
            newVm().cancel() // never connected: nothing to drop
            val vm = reviewVm()

            vm.cancel()
            advanceUntilIdle()

            coVerify(exactly = 1) { apiClient.cancelMeerkatImport("s1") }
        }

    /** Clears [this] through a [ViewModelStore], exactly as popping its nav destination does. */
    @Suppress("UNCHECKED_CAST")
    private fun MeerkatImportViewModel.leaveScreen() {
        val self = this
        val store = ViewModelStore()
        ViewModelProvider(
            store,
            object : ViewModelProvider.Factory {
                override fun <T : ViewModel> create(modelClass: Class<T>): T = self as T
            }
        )[MeerkatImportViewModel::class.java]
        store.clear()
    }

    @Test
    fun `leaving the screen after connect sends exactly one cancel for the session`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.connectMeerkat(any()) } returns Result.success(connected())
            coEvery { apiClient.cancelMeerkatImport("s1") } returns Result.success(Unit)
            val vm = newVm()
            vm.fillToken()
            vm.connect()
            advanceUntilIdle()

            vm.leaveScreen()
            advanceUntilIdle()

            coVerify(exactly = 1) { apiClient.cancelMeerkatImport("s1") }
        }

    @Test
    fun `leaving the screen before connecting sends no cancel`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = newVm()
            advanceUntilIdle()

            vm.leaveScreen()
            advanceUntilIdle()

            coVerify(exactly = 0) { apiClient.cancelMeerkatImport(any()) }
        }

    @Test
    fun `leaving mid-fetch and from review both cancel the session`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.cancelMeerkatImport("s1") } returns Result.success(Unit)
            val review = reviewVm()
            review.leaveScreen()
            advanceUntilIdle()
            coVerify(exactly = 1) { apiClient.cancelMeerkatImport("s1") }

            coEvery { apiClient.getMeerkatImportStatus("s1") } returns
                Result.success(status("fetching"))
            val fetching = newVm()
            fetching.fillToken()
            fetching.connect()
            advanceUntilIdle()
            fetching.startImport()
            advanceTimeBy(50)
            fetching.leaveScreen()
            advanceUntilIdle()

            coVerify(exactly = 2) { apiClient.cancelMeerkatImport("s1") }
        }

    @Test
    fun `leaving after a confirm does not cancel the import the user just confirmed`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = reviewVm()
            coEvery { apiClient.confirmMeerkatImport(any()) } returns Result.success(Unit)
            coEvery { apiClient.cancelMeerkatImport(any()) } returns Result.success(Unit)
            coEvery { apiClient.getMeerkatImportStatus("s1") } returns
                Result.success(status("importing"))

            vm.confirm()
            advanceTimeBy(50)
            assertEquals(MeerkatImportStep.WORKING, vm.uiState.value.step)
            vm.leaveScreen()
            advanceUntilIdle()

            coVerify(exactly = 0) { apiClient.cancelMeerkatImport(any()) }
        }

    @Test
    fun `a confirm the server rejected leaves the session cancellable`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = reviewVm()
            coEvery { apiClient.confirmMeerkatImport(any()) } returns
                Result.failure(ApiError.Client(409, "Already running"))
            coEvery { apiClient.cancelMeerkatImport("s1") } returns Result.success(Unit)
            vm.confirm()
            advanceUntilIdle()

            vm.leaveScreen()
            advanceUntilIdle()

            coVerify(exactly = 1) { apiClient.cancelMeerkatImport("s1") }
        }

    @Test
    fun `a Local profile marks the import unavailable`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = newVm(local = true)
            advanceUntilIdle()
            assertTrue(vm.uiState.value.unavailable)
            assertFalse(newVm(local = false).also { advanceUntilIdle() }.uiState.value.unavailable)
        }
}
