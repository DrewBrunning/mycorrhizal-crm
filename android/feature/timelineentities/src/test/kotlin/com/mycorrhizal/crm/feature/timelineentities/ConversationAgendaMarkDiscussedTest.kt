package com.mycorrhizal.crm.feature.timelineentities

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.StateRestorationTester
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.ActivityRepository
import com.mycorrhizal.crm.domain.repository.ContactActivitiesPage
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.ConversationAgendaRepository
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.ConversationAgenda
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.UnconfinedTestDispatcher
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

// Issue #1723: tapping "Mark discussed" did nothing -- a LaunchedEffect cleared
// pendingDiscussId on the dialog's first composition (discussingId still null).
// These drive the real ConversationAgendaScreen against a mocked repository.
@OptIn(ExperimentalCoroutinesApi::class)
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ConversationAgendaMarkDiscussedTest {

    // Unconfined, so viewModelScope work finishes before control returns to the
    // test (waitForIdle only waits on the main thread; see #1802/#1803).
    @get:Rule
    val mainDispatcherRule = MainDispatcherRule(UnconfinedTestDispatcher())

    @get:Rule
    val composeTestRule = createComposeRule()

    private val uid = "11111111-1111-4111-8111-111111111111"
    private val open = ConversationAgenda(id = "a1", entityId = uid, content = "Ask about the move")
    private val discussed = open.copy(discussedAt = "2026-10-19T10:00:00Z")

    private val repo = mockk<ConversationAgendaRepository>()

    private fun vm(
        saved: SavedStateHandle = SavedStateHandle(mapOf("contactId" to 5)),
        activities: List<Activity> = emptyList()
    ): ConversationAgendaViewModel {
        val activityRepo = mockk<ActivityRepository>()
        val contacts = mockk<ContactRepository>()
        coEvery { contacts.getContact(5) } returns Result.success(
            ContactRecordResponse(id = 5, card = Card(uid = uid, name = Name(full = "Dana White")))
        )
        coEvery { activityRepo.listForContact(5, any(), any(), any(), any(), any()) } returns
            Result.success(ContactActivitiesPage(activities = activities, nextCursor = null))
        return ConversationAgendaViewModel(repo, activityRepo, contacts, saved)
    }

    private fun show(viewModel: ConversationAgendaViewModel = vm()) {
        composeTestRule.setContent {
            MycorrhizalTheme { ConversationAgendaScreen(onBack = {}, viewModel = viewModel) }
        }
    }

    @Test
    fun `tapping mark discussed opens the full-screen form and it stays open`() {
        coEvery { repo.listForContact(uid) } returns Result.success(listOf(open))
        show()

        composeTestRule.onNodeWithContentDescription("Mark discussed").performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Mark discussed").assertIsDisplayed()
        composeTestRule.onNodeWithText("Discuss").assertIsDisplayed()
    }

    @Test
    fun `confirming PATCHes discuss, closes the form and moves the item to Discussed`() {
        coEvery { repo.listForContact(uid) } returnsMany
            listOf(Result.success(listOf(open)), Result.success(listOf(discussed)))
        coEvery { repo.discuss("a1", null) } returns Result.success(discussed)
        show()

        composeTestRule.onNodeWithContentDescription("Mark discussed").performClick()
        composeTestRule.onNodeWithText("Discuss").performClick()
        composeTestRule.waitForIdle()

        coVerify(exactly = 1) { repo.discuss("a1", null) }
        // Dialog gone (its title was the only "Mark discussed" text node); the
        // item now sits under Discussed with no mark-discussed action left.
        composeTestRule.onNodeWithText("Mark discussed").assertDoesNotExist()
        composeTestRule.onNodeWithText("Discussed").assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("Mark discussed").assertDoesNotExist()
    }

    @Test
    fun `dialog stays open with the button disabled while the request is in flight`() {
        val gate = CompletableDeferred<Result<ConversationAgenda>>()
        coEvery { repo.listForContact(uid) } returns Result.success(listOf(open))
        coEvery { repo.discuss("a1", null) } coAnswers { gate.await() }
        show()

        composeTestRule.onNodeWithContentDescription("Mark discussed").performClick()
        composeTestRule.onNodeWithText("Discuss").performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Mark discussed").assertIsDisplayed()
        composeTestRule.onNodeWithText("Discuss").assertIsNotEnabled()

        coEvery { repo.listForContact(uid) } returns Result.success(listOf(discussed))
        gate.complete(Result.success(discussed))
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithText("Mark discussed").assertDoesNotExist()
    }

    @Test
    fun `a failed request keeps the dialog open and re-enables the button`() {
        coEvery { repo.listForContact(uid) } returns Result.success(listOf(open))
        coEvery { repo.discuss("a1", null) } returns Result.failure(ApiError.Server(500, "boom"))
        show()

        composeTestRule.onNodeWithContentDescription("Mark discussed").performClick()
        composeTestRule.onNodeWithText("Discuss").performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Mark discussed").assertIsDisplayed()
        composeTestRule.onNodeWithText("Discuss").assertIsEnabled()
    }

    // #1773 review: the full-screen form replaces the list, whose snackbar used to
    // report the failure behind the old dialog. The form must say why it stayed open.
    @Test
    fun `a failed request shows its error on the open form`() {
        coEvery { repo.listForContact(uid) } returns Result.success(listOf(open))
        coEvery { repo.discuss("a1", null) } returns Result.failure(ApiError.Server(500, "boom"))
        show()

        composeTestRule.onNodeWithContentDescription("Mark discussed").performClick()
        composeTestRule.onNodeWithText("Discuss").performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Mark discussed").assertIsDisplayed()
        composeTestRule.onNodeWithText("Server error (500)").assertIsDisplayed()
    }

    // Review follow-up: the close signal must not depend on the screen
    // collecting at the moment the PATCH lands (rotation mid-request). A
    // success with no UI attached still leaves the dialog closed.
    @Test
    fun `a success while no screen is attached still closes the form`() = runTest {
        coEvery { repo.listForContact(uid) } returnsMany
            listOf(Result.success(listOf(open)), Result.success(listOf(discussed)))
        coEvery { repo.discuss("a1", null) } returns Result.success(discussed)
        val viewModel = vm()

        viewModel.openDiscuss("a1")
        viewModel.markDiscussed("a1", null)

        assertNull(viewModel.pendingDiscussId.value)
    }

    // Process death: the open dialog is restored from the SavedStateHandle.
    @Test
    fun `an open dialog is restored after process death`() {
        coEvery { repo.listForContact(uid) } returns Result.success(listOf(open))
        val saved = SavedStateHandle(mapOf("contactId" to 5))
        vm(saved).openDiscuss("a1")

        // A fresh ViewModel over the same saved state, as after process death.
        show(vm(saved))

        composeTestRule.onNodeWithText("Mark discussed").assertIsDisplayed()
        composeTestRule.onNodeWithText("Discuss").assertIsEnabled()
    }

    // Process death after the PATCH landed: nothing left to confirm.
    @Test
    fun `a restored dialog for an already discussed item is dropped`() {
        coEvery { repo.listForContact(uid) } returnsMany
            listOf(Result.success(listOf(open)), Result.success(listOf(discussed)))
        val saved = SavedStateHandle(mapOf("contactId" to 5))
        vm(saved).openDiscuss("a1")

        val restored = vm(saved)

        assertNull(restored.pendingDiscussId.value)
    }

    @Test
    fun `cancel closes the form`() {
        coEvery { repo.listForContact(uid) } returns Result.success(listOf(open))
        show()

        composeTestRule.onNodeWithContentDescription("Mark discussed").performClick()
        composeTestRule.onNodeWithContentDescription("Back").performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Mark discussed").assertDoesNotExist()
    }

    // Rotation: the picked activity survives an activity recreation.
    @Test
    fun `the picked activity survives a configuration change`() {
        val restoration = StateRestorationTester(composeTestRule)
        restoration.setContent {
            MycorrhizalTheme {
                MarkDiscussedForm(
                    item = open,
                    activities = listOf(Activity(id = 9, title = "Coffee", date = "2026-10-18")),
                    confirming = false,
                    onConfirm = {},
                    onDismiss = {}
                )
            }
        }
        composeTestRule.onNodeWithText("None").performClick()
        composeTestRule.onNodeWithText("Coffee").performClick()
        composeTestRule.onNodeWithText("Coffee").assertIsDisplayed()

        restoration.emulateSavedInstanceStateRestore()

        composeTestRule.onNodeWithText("Coffee").assertIsDisplayed()
    }
}
