package com.mycorrhizal.crm.feature.settings

import com.mycorrhizal.crm.data.attach.AttachException
import com.mycorrhizal.crm.data.attach.AttachFinishResult
import com.mycorrhizal.crm.data.attach.AttachPreview
import com.mycorrhizal.crm.data.attach.AttachProgress
import com.mycorrhizal.crm.data.attach.AttachSignInResult
import com.mycorrhizal.crm.data.attach.AttachStage
import com.mycorrhizal.crm.data.attach.AttachToRemoteCoordinator
import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.domain.profile.ServerProfile
import com.mycorrhizal.crm.domain.profile.ServerProfileKind
import com.mycorrhizal.crm.model.network.ImportRowPreview
import com.mycorrhizal.crm.model.network.MycorrhizalBundleCounts
import com.mycorrhizal.crm.model.network.SourceImportResult
import com.mycorrhizal.crm.model.network.SourceImportPreviewResponse
import com.mycorrhizal.crm.model.network.RowImportAction
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import com.mycorrhizal.crm.ui.R
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import io.mockk.verify
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class AttachToRemoteViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val local = ServerProfile("loc", ServerProfileKind.Local, "On this device")
    private val remoteProfile = ServerProfile("r1", ServerProfileKind.Remote("https://home.example"), "Home")

    // Lets a mocked coordinator answer inspect the view model mid-flight.
    private lateinit var vm0: AttachToRemoteViewModel

    private val coordinator = mockk<AttachToRemoteCoordinator>(relaxed = true)
    private val session = mockk<SessionManager>(relaxed = true)

    private val rows = listOf(
        ImportRowPreview(rowIndex = 0, suggestedAction = "update"),
        ImportRowPreview(rowIndex = 1, suggestedAction = "add", validationErrors = listOf("bad")),
        ImportRowPreview(rowIndex = 2, suggestedAction = "add"),
    )
    private val preview = AttachPreview(
        totals = MycorrhizalBundleCounts(contacts = 3),
        preview = SourceImportPreviewResponse(sessionId = "s", rows = rows),
    )

    private fun viewModel(beginOk: Boolean = true): AttachToRemoteViewModel {
        coEvery { coordinator.begin() } returns
            if (beginOk) Result.success(local) else Result.failure(AttachException("nope"))
        coEvery { session.profiles() } returns listOf(local, remoteProfile)
        return AttachToRemoteViewModel(coordinator, session)
    }

    private fun stubPrepareOk() {
        coEvery { coordinator.signIn(any(), any(), any(), any(), any()) } returns
            Result.success(AttachSignInResult.SignedIn)
        coEvery { coordinator.prepare(any()) } returns Result.success(preview)
    }

    @Test
    fun `starts at sign-in with the configured remote profiles offered`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = viewModel()
            advanceUntilIdle()

            assertEquals(AttachStep.SignIn, vm.uiState.value.step)
            assertEquals(listOf(remoteProfile), vm.uiState.value.remoteProfiles)
        }

    @Test
    fun `a non-Local or archived start is blocked with a message`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = viewModel(beginOk = false)
        advanceUntilIdle()

        assertEquals(AttachStep.Blocked, vm.uiState.value.step)
        assertEquals(R.string.attach_error_not_local, vm.uiState.value.errorRes)
    }

    @Test
    fun `an invalid new server url is rejected before any network call`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = viewModel()
            advanceUntilIdle()

            vm.signIn(null, "x", "not a url", "alice", "pw")
            advanceUntilIdle()

            assertEquals(R.string.servers_add_invalid_url, vm.uiState.value.errorRes)
            assertEquals(AttachStep.SignIn, vm.uiState.value.step)
            coVerify(exactly = 0) { coordinator.signIn(any(), any(), any(), any(), any()) }
        }

    @Test
    fun `missing credentials are rejected before any network call`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = viewModel()
        advanceUntilIdle()

        vm.signIn("r1", "", "", " ", "")
        advanceUntilIdle()

        assertEquals(R.string.attach_error_credentials, vm.uiState.value.errorRes)
        coVerify(exactly = 0) { coordinator.signIn(any(), any(), any(), any(), any()) }
    }

    @Test
    fun `sign-in then prepare lands on review with rows seeded from their suggested actions`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubPrepareOk()
            val vm = viewModel()
            advanceUntilIdle()

            vm.signIn("r1", "", "", "alice", "pw")
            advanceUntilIdle()

            val state = vm.uiState.value
            assertEquals(AttachStep.Review, state.step)
            assertEquals(preview, state.preview)
            assertEquals("update", state.rowActions[0])
            assertEquals("a row with validation errors is forced to skip", "skip", state.rowActions[1])
            assertEquals("add", state.rowActions[2])
        }

    @Test
    fun `progress reported by the coordinator is surfaced while working`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { coordinator.signIn(any(), any(), any(), any(), any()) } returns
                Result.success(AttachSignInResult.SignedIn)
            coEvery { coordinator.prepare(any()) } coAnswers {
                firstArg<(AttachProgress) -> Unit>().invoke(AttachProgress(AttachStage.Uploading, 1, 2))
                assertEquals(AttachProgress(AttachStage.Uploading, 1, 2), vm0.uiState.value.progress)
                Result.success(preview)
            }
            val vm = viewModel()
            vm0 = vm
            advanceUntilIdle()

            vm.signIn("r1", "", "", "alice", "pw")
            advanceUntilIdle()

            assertNull("progress is cleared once the review is shown", vm.uiState.value.progress)
        }

    @Test
    fun `a wrong password shows the credentials message rather than the generic session-expired text`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { coordinator.signIn(any(), any(), any(), any(), any()) } returns
                Result.failure(ApiError.Client(401, "unauthorized"))
            val vm = viewModel()
            advanceUntilIdle()

            vm.signIn("r1", "", "", "alice", "bad")
            advanceUntilIdle()

            assertEquals(AttachStep.SignIn, vm.uiState.value.step)
            assertEquals(R.string.attach_error_bad_credentials, vm.uiState.value.errorRes)
            assertNull(vm.uiState.value.error)
        }

    @Test
    fun `a network failure at sign-in returns to sign-in with the error text`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { coordinator.signIn(any(), any(), any(), any(), any()) } returns
                Result.failure(ApiError.Network(java.net.ConnectException("down")))
            val vm = viewModel()
            advanceUntilIdle()

            vm.signIn("r1", "", "", "alice", "pw")
            advanceUntilIdle()

            assertEquals(AttachStep.SignIn, vm.uiState.value.step)
            assertEquals("No connection", vm.uiState.value.error)
        }

    @Test
    fun `an unknown failure falls back to its own message`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { coordinator.signIn(any(), any(), any(), any(), any()) } returns
            Result.failure(AttachException("Server returned no session"))
        val vm = viewModel()
        advanceUntilIdle()

        vm.signIn("r1", "", "", "alice", "pw")
        advanceUntilIdle()

        assertEquals("Server returned no session", vm.uiState.value.error)
    }

    @Test
    fun `a two-factor account is asked for its code then continues to review`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { coordinator.signIn(any(), any(), any(), any(), any()) } returns
                Result.success(AttachSignInResult.TwoFactorRequired)
            coEvery { coordinator.completeTwoFactor("123456") } returns Result.success(AttachSignInResult.SignedIn)
            coEvery { coordinator.prepare(any()) } returns Result.success(preview)
            val vm = viewModel()
            advanceUntilIdle()

            vm.signIn("r1", "", "", "alice", "pw")
            advanceUntilIdle()
            assertEquals(AttachStep.TwoFactor, vm.uiState.value.step)

            vm.submitTwoFactor(" 123456 ")
            advanceUntilIdle()

            assertEquals(AttachStep.Review, vm.uiState.value.step)
        }

    @Test
    fun `a wrong two-factor code stays on the code step`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { coordinator.signIn(any(), any(), any(), any(), any()) } returns
            Result.success(AttachSignInResult.TwoFactorRequired)
        coEvery { coordinator.completeTwoFactor(any()) } returns Result.failure(ApiError.Client(401, "bad"))
        val vm = viewModel()
        advanceUntilIdle()
        vm.signIn("r1", "", "", "alice", "pw")
        advanceUntilIdle()

        vm.submitTwoFactor("000000")
        advanceUntilIdle()

        assertEquals(AttachStep.TwoFactor, vm.uiState.value.step)
        assertEquals(R.string.attach_error_bad_credentials, vm.uiState.value.errorRes)
    }

    @Test
    fun `a blank two-factor code or one submitted on the wrong step is ignored`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = viewModel()
            advanceUntilIdle()

            vm.submitTwoFactor("123456") // still on SignIn
            advanceUntilIdle()

            coVerify(exactly = 0) { coordinator.completeTwoFactor(any()) }
        }

    @Test
    fun `a prepare failure offers a retry that resumes and reaches review`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { coordinator.signIn(any(), any(), any(), any(), any()) } returns
                Result.success(AttachSignInResult.SignedIn)
            coEvery { coordinator.prepare(any()) } returnsMany
                listOf(Result.failure(ApiError.Server(500, "x")), Result.success(preview))
            val vm = viewModel()
            advanceUntilIdle()

            vm.signIn("r1", "", "", "alice", "pw")
            advanceUntilIdle()
            assertEquals(AttachStep.PrepareFailed, vm.uiState.value.step)
            assertNotNull(vm.uiState.value.error)

            vm.retryPrepare()
            advanceUntilIdle()

            assertEquals(AttachStep.Review, vm.uiState.value.step)
            assertNull(vm.uiState.value.error)
        }

    @Test
    fun `an over-limit export shows the server's actionable message on the prepare-failed step`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val msg = "The account bundle is 70.0 MiB, which is over the 64 MiB limit the import accepts."
            coEvery { coordinator.signIn(any(), any(), any(), any(), any()) } returns
                Result.success(AttachSignInResult.SignedIn)
            coEvery { coordinator.prepare(any()) } returns Result.failure(ApiError.Server(507, msg))
            val vm = viewModel()
            advanceUntilIdle()

            vm.signIn("r1", "", "", "alice", "pw")
            advanceUntilIdle()

            assertEquals(AttachStep.PrepareFailed, vm.uiState.value.step)
            assertEquals(msg, vm.uiState.value.error)
        }

    @Test
    fun `setRowAction changes a valid row and ignores a row with validation errors or an unknown row`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubPrepareOk()
            val vm = viewModel()
            advanceUntilIdle()
            vm.signIn("r1", "", "", "alice", "pw")
            advanceUntilIdle()

            vm.setRowAction(2, "skip")
            vm.setRowAction(1, "add")
            vm.setRowAction(99, "add")

            assertEquals("skip", vm.uiState.value.rowActions[2])
            assertEquals("skip", vm.uiState.value.rowActions[1])
            assertNull(vm.uiState.value.rowActions[99])
        }

    @Test
    fun `resolveAll resets valid rows to their suggested action and keeps errored rows skipped`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubPrepareOk()
            val vm = viewModel()
            advanceUntilIdle()
            vm.signIn("r1", "", "", "alice", "pw")
            advanceUntilIdle()
            vm.setRowAction(0, "skip")
            vm.setRowAction(2, "skip")

            vm.resolveAll()

            assertEquals("update", vm.uiState.value.rowActions[0])
            assertEquals("add", vm.uiState.value.rowActions[2])
            assertEquals("skip", vm.uiState.value.rowActions[1])
        }

    @Test
    fun `setRowAction and resolveAll before a preview exists do nothing`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = viewModel()
            advanceUntilIdle()

            vm.setRowAction(0, "add")
            vm.resolveAll()

            assertTrue(vm.uiState.value.rowActions.isEmpty())
        }

    private suspend fun kotlinx.coroutines.test.TestScope.toReview(vm: AttachToRemoteViewModel) {
        advanceUntilIdle()
        vm.signIn("r1", "", "", "alice", "pw")
        advanceUntilIdle()
    }

    @Test
    fun `confirm sends the reviewed actions, finishes, and lands on done`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubPrepareOk()
            val result = SourceImportResult(created = 2)
            coEvery { coordinator.confirm(any(), any()) } returns Result.success(result)
            coEvery { coordinator.finish(false) } returns AttachFinishResult.Done
            val vm = viewModel()
            toReview(vm)
            vm.setRowAction(2, "skip")

            vm.confirm()
            advanceUntilIdle()

            assertEquals(AttachStep.Done, vm.uiState.value.step)
            assertEquals(result, vm.uiState.value.result)
            coVerify {
                coordinator.confirm(
                    match { actions ->
                        actions.toSet() == setOf(
                            RowImportAction(0, "update"),
                            RowImportAction(1, "skip"),
                            RowImportAction(2, "skip"),
                        )
                    },
                    any(),
                )
            }
        }

    @Test
    fun `confirm is ignored outside the review step`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = viewModel()
        advanceUntilIdle()

        vm.confirm()
        advanceUntilIdle()

        coVerify(exactly = 0) { coordinator.confirm(any(), any()) }
    }

    @Test
    fun `a failed confirm returns to review with the error and Local is never finished`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubPrepareOk()
            coEvery { coordinator.confirm(any(), any()) } returns Result.failure(AttachException("boom"))
            val vm = viewModel()
            toReview(vm)

            vm.confirm()
            advanceUntilIdle()

            assertEquals(AttachStep.Review, vm.uiState.value.step)
            assertEquals("boom", vm.uiState.value.error)
            coVerify(exactly = 0) { coordinator.finish(any()) }
        }

    @Test
    fun `unsent local interactions ask for confirmation and discarding finishes the move`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubPrepareOk()
            coEvery { coordinator.confirm(any(), any()) } returns Result.success(SourceImportResult())
            coEvery { coordinator.finish(false) } returns AttachFinishResult.NeedsConfirmation(3)
            coEvery { coordinator.finish(true) } returns AttachFinishResult.Done
            val vm = viewModel()
            toReview(vm)

            vm.confirm()
            advanceUntilIdle()
            assertEquals(3, vm.uiState.value.pendingDiscardCount)
            assertEquals(AttachStep.Review, vm.uiState.value.step)

            vm.confirmDiscardAndFinish()
            advanceUntilIdle()

            assertEquals(AttachStep.Done, vm.uiState.value.step)
            assertNull(vm.uiState.value.pendingDiscardCount)
        }

    @Test
    fun `declining the discard keeps the import applied so confirm only finishes and never re-imports`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubPrepareOk()
            coEvery { coordinator.confirm(any(), any()) } returns Result.success(SourceImportResult())
            coEvery { coordinator.finish(false) } returnsMany
                listOf(AttachFinishResult.NeedsConfirmation(1), AttachFinishResult.Done)
            val vm = viewModel()
            toReview(vm)
            vm.confirm()
            advanceUntilIdle()

            vm.dismissDiscard()
            assertNull(vm.uiState.value.pendingDiscardCount)
            assertTrue(vm.uiState.value.importApplied)
            vm.confirm()
            advanceUntilIdle()

            assertEquals(AttachStep.Done, vm.uiState.value.step)
            coVerify(exactly = 1) { coordinator.confirm(any(), any()) }
        }

    @Test
    fun `a failure while finishing returns to review with the error`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubPrepareOk()
            coEvery { coordinator.confirm(any(), any()) } returns Result.success(SourceImportResult())
            coEvery { coordinator.finish(any()) } throws AttachException("switch failed")
            val vm = viewModel()
            toReview(vm)

            vm.confirm()
            advanceUntilIdle()

            assertEquals(AttachStep.Review, vm.uiState.value.step)
            assertEquals("switch failed", vm.uiState.value.error)
        }

    @Test
    fun `cancel drops the run and leaving the wizard always closes the coordinator`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = viewModel()
            advanceUntilIdle()

            vm.cancel()
            advanceUntilIdle()
            coVerify { coordinator.cancel() }

            AttachToRemoteViewModel::class.java.getDeclaredMethod("onCleared")
                .apply { isAccessible = true }
                .invoke(vm)
            verify { coordinator.close() }
        }

    @Test
    fun `onErrorShown clears the error`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = viewModel(beginOk = false)
        advanceUntilIdle()

        vm.onErrorShown()

        assertNull(vm.uiState.value.errorRes)
        assertNull(vm.uiState.value.error)
    }

    @Test
    fun `sign-in is ignored once past the sign-in step`() = runTest(mainDispatcherRule.testDispatcher) {
        stubPrepareOk()
        val vm = viewModel()
        toReview(vm)

        vm.signIn("r1", "", "", "alice", "pw")
        advanceUntilIdle()

        coVerify(exactly = 1) { coordinator.signIn(any(), any(), any(), any(), any()) }
    }
}
