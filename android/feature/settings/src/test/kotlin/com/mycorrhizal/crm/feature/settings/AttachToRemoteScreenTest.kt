package com.mycorrhizal.crm.feature.settings

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import com.mycorrhizal.crm.data.attach.AttachException
import com.mycorrhizal.crm.data.attach.AttachFinishResult
import com.mycorrhizal.crm.data.attach.AttachPreview
import com.mycorrhizal.crm.data.attach.AttachProgress
import com.mycorrhizal.crm.data.attach.AttachSignInResult
import com.mycorrhizal.crm.data.attach.AttachStage
import com.mycorrhizal.crm.data.attach.AttachToRemoteCoordinator
import com.mycorrhizal.crm.testing.FakePasskeyClient
import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.domain.profile.ServerProfile
import com.mycorrhizal.crm.domain.profile.ServerProfileKind
import com.mycorrhizal.crm.model.network.ImportRowPreview
import com.mycorrhizal.crm.model.network.MycorrhizalBundleCounts
import com.mycorrhizal.crm.model.network.SourceImportIssue
import com.mycorrhizal.crm.model.network.SourceImportResult
import com.mycorrhizal.crm.model.network.SourceImportPreviewResponse
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.CompletableDeferred
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class AttachToRemoteScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val local = ServerProfile("loc", ServerProfileKind.Local, "On this device")
    private val home = ServerProfile("r1", ServerProfileKind.Remote("https://home.example"), "Home")

    private val coordinator = mockk<AttachToRemoteCoordinator>(relaxed = true)
    private val session = mockk<SessionManager>(relaxed = true)

    private val preview = AttachPreview(
        totals = MycorrhizalBundleCounts(contacts = 3, notes = 4, activities = 5),
        preview = SourceImportPreviewResponse(
            sessionId = "s",
            rows = listOf(ImportRowPreview(rowIndex = 0, parsedContact = mapOf("firstname" to "Ada"))),
            lossReport = listOf(SourceImportIssue(record = "c1", field = "f", category = "lossy", message = "m")),
        ),
    )

    private var backCount = 0

    private fun show(beginOk: Boolean = true, remotes: List<ServerProfile> = listOf(home)) {
        coEvery { coordinator.begin() } returns
            if (beginOk) Result.success(local) else Result.failure(AttachException("x"))
        coEvery { session.profiles() } returns listOf(local) + remotes
        val vm = AttachToRemoteViewModel(coordinator, session, FakePasskeyClient())
        composeTestRule.setContent {
            MycorrhizalTheme { AttachToRemoteScreen(onBack = { backCount++ }, viewModel = vm) }
        }
        composeTestRule.waitForIdle()
    }

    @Test
    fun `sign-in lists the configured servers and signs in to the chosen one`() {
        coEvery { coordinator.signIn(any(), any(), any(), any(), any()) } returns
            Result.failure(AttachException("stop here"))
        show()

        composeTestRule.onNodeWithText("Home").assertIsDisplayed()
        composeTestRule.onNodeWithText("Add a new server").assertIsDisplayed()
        composeTestRule.onNodeWithTag("attach-identifier").performTextInput("alice")
        composeTestRule.onNodeWithTag("attach-password").performTextInput("pw")
        composeTestRule.onNodeWithTag("attach-sign-in").performScrollTo().performClick()
        composeTestRule.waitForIdle()

        coVerify { coordinator.signIn("r1", "", "", "alice", "pw") }
        composeTestRule.onNodeWithTag("attach-error").assertIsDisplayed()
    }

    @Test
    fun `choosing to add a new server reveals the url fields and sends them`() {
        coEvery { coordinator.signIn(any(), any(), any(), any(), any()) } returns
            Result.failure(AttachException("stop here"))
        show()

        composeTestRule.onNodeWithTag("attach-server-new").performClick()
        composeTestRule.onNodeWithTag("attach-label").performTextInput("Work")
        composeTestRule.onNodeWithTag("attach-url").performTextInput("https://work.example.com")
        composeTestRule.onNodeWithTag("attach-identifier").performTextInput("alice")
        composeTestRule.onNodeWithTag("attach-password").performTextInput("pw")
        composeTestRule.onNodeWithTag("attach-sign-in").performScrollTo().performClick()
        composeTestRule.waitForIdle()

        coVerify { coordinator.signIn(null, "Work", "https://work.example.com", "alice", "pw") }
    }

    @Test
    fun `with no configured servers only the new-server form is offered`() {
        show(remotes = emptyList())

        composeTestRule.onNodeWithTag("attach-url").assertIsDisplayed()
    }

    @Test
    fun `a blocked start explains that only the on-device profile can be moved`() {
        show(beginOk = false)

        composeTestRule.onNodeWithText("Only the on-device profile can be moved to a server.").assertIsDisplayed()
    }

    @Test
    fun `two-factor accounts are asked for a code`() {
        coEvery { coordinator.signIn(any(), any(), any(), any(), any()) } returns
            Result.success(AttachSignInResult.TwoFactorRequired())
        coEvery { coordinator.completeTwoFactor(any()) } returns Result.failure(AttachException("nope"))
        show()

        composeTestRule.onNodeWithTag("attach-identifier").performTextInput("alice")
        composeTestRule.onNodeWithTag("attach-password").performTextInput("pw")
        composeTestRule.onNodeWithTag("attach-sign-in").performScrollTo().performClick()
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithTag("attach-2fa-code").performTextInput("123456")
        composeTestRule.onNodeWithTag("attach-2fa-submit").performClick()
        composeTestRule.waitForIdle()

        coVerify { coordinator.completeTwoFactor("123456") }
    }

    @Test
    fun `the working step shows the stage the coordinator reports`() {
        val gate = CompletableDeferred<Unit>()
        coEvery { coordinator.signIn(any(), any(), any(), any(), any()) } returns
            Result.success(AttachSignInResult.SignedIn)
        coEvery { coordinator.prepare(any()) } coAnswers {
            firstArg<(AttachProgress) -> Unit>().invoke(AttachProgress(AttachStage.Uploading, 1, 4))
            gate.await()
            Result.success(preview)
        }
        show()

        composeTestRule.onNodeWithTag("attach-identifier").performTextInput("alice")
        composeTestRule.onNodeWithTag("attach-password").performTextInput("pw")
        composeTestRule.onNodeWithTag("attach-sign-in").performScrollTo().performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Uploading to the server…").assertIsDisplayed()
        composeTestRule.onNodeWithText("1 of 4").assertIsDisplayed()
        gate.complete(Unit)
        composeTestRule.waitForIdle()
    }

    @Test
    fun `a prepare failure offers a retry`() {
        coEvery { coordinator.signIn(any(), any(), any(), any(), any()) } returns
            Result.success(AttachSignInResult.SignedIn)
        coEvery { coordinator.prepare(any()) } returnsMany
            listOf(Result.failure(AttachException("net")), Result.success(preview))
        show()

        composeTestRule.onNodeWithTag("attach-identifier").performTextInput("alice")
        composeTestRule.onNodeWithTag("attach-password").performTextInput("pw")
        composeTestRule.onNodeWithTag("attach-sign-in").performScrollTo().performClick()
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithText("The move could not be prepared. Nothing on this device was changed.")
            .assertIsDisplayed()
        composeTestRule.onNodeWithTag("attach-retry").performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Ready to move 3 contacts, 4 notes and 5 activities.").assertIsDisplayed()
    }

    private fun reachReview() {
        coEvery { coordinator.signIn(any(), any(), any(), any(), any()) } returns
            Result.success(AttachSignInResult.SignedIn)
        coEvery { coordinator.prepare(any()) } returns Result.success(preview)
        show()
        composeTestRule.onNodeWithTag("attach-identifier").performTextInput("alice")
        composeTestRule.onNodeWithTag("attach-password").performTextInput("pw")
        composeTestRule.onNodeWithTag("attach-sign-in").performScrollTo().performClick()
        composeTestRule.waitForIdle()
    }

    @Test
    fun `review shows the totals the loss report and the shared per-row review`() {
        reachReview()

        composeTestRule.onNodeWithText("Ready to move 3 contacts, 4 notes and 5 activities.").assertIsDisplayed()
        composeTestRule.onNodeWithTag("attach-loss").assertIsDisplayed()
        composeTestRule.onNodeWithText("Ada").assertIsDisplayed()
        composeTestRule.onNodeWithText("Apply decisions (1)").assertIsDisplayed()
    }

    @Test
    fun `confirming imports then shows the done summary and the archive note`() {
        coEvery { coordinator.confirm(any(), any()) } returns
            Result.success(SourceImportResult(created = 7, updated = 2, skipped = 1))
        coEvery { coordinator.finish(false) } returns AttachFinishResult.Done
        reachReview()

        composeTestRule.onNodeWithText("Apply decisions (1)").performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Data moved").assertIsDisplayed()
        composeTestRule.onNodeWithTag("attach-result").assertIsDisplayed()
        composeTestRule.onNodeWithText("7 created, 2 merged, 1 skipped.").assertIsDisplayed()
        composeTestRule.onNodeWithTag("attach-done").performClick()
        // Done: leaving must NOT cancel a run that has already completed.
        coVerify(exactly = 0) { coordinator.cancel() }
        assert(backCount == 1)
    }

    @Test
    fun `unsent local activity prompts before finishing and can be discarded`() {
        coEvery { coordinator.confirm(any(), any()) } returns Result.success(SourceImportResult())
        coEvery { coordinator.finish(false) } returns AttachFinishResult.NeedsConfirmation(2)
        coEvery { coordinator.finish(true) } returns AttachFinishResult.Done
        reachReview()

        composeTestRule.onNodeWithText("Apply decisions (1)").performClick()
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithText("Unsent activity").assertIsDisplayed()
        composeTestRule.onNodeWithText("Discard and finish").performClick()
        composeTestRule.waitForIdle()

        coVerify { coordinator.finish(true) }
        composeTestRule.onNodeWithText("Data moved").assertIsDisplayed()
    }

    @Test
    fun `backing out before the switch cancels the run`() {
        show()

        composeTestRule.onNodeWithContentDescriptionBack().performClick()
        composeTestRule.waitForIdle()

        coVerify { coordinator.cancel() }
        assert(backCount == 1)
    }

    private fun androidx.compose.ui.test.junit4.ComposeContentTestRule.onNodeWithContentDescriptionBack() =
        onNodeWithContentDescription("Back")
}
