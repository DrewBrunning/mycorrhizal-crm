package com.mycorrhizal.crm.feature.timelineentities

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.ActivityRepository
import com.mycorrhizal.crm.domain.repository.ContactActivitiesPage
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.ConversationAgendaRepository
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

    private fun show() {
        val activityRepo = mockk<ActivityRepository>()
        val contacts = mockk<ContactRepository>()
        coEvery { contacts.getContact(5) } returns Result.success(
            ContactRecordResponse(id = 5, card = Card(uid = uid, name = Name(full = "Dana White"))),
        )
        coEvery { activityRepo.listForContact(5, any(), any(), any(), any(), any()) } returns
            Result.success(ContactActivitiesPage(activities = emptyList(), nextCursor = null))
        val vm = ConversationAgendaViewModel(repo, activityRepo, contacts, SavedStateHandle(mapOf("contactId" to 5)))
        composeTestRule.setContent {
            MycorrhizalTheme { ConversationAgendaScreen(onBack = {}, viewModel = vm) }
        }
    }

    @Test
    fun `tapping mark discussed opens the dialog and it stays open`() {
        coEvery { repo.listForContact(uid) } returns Result.success(listOf(open))
        show()

        composeTestRule.onNodeWithContentDescription("Mark discussed").performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Mark discussed").assertIsDisplayed()
        composeTestRule.onNodeWithText("Discuss").assertIsDisplayed()
    }

    @Test
    fun `confirming PATCHes discuss, closes the dialog and moves the item to Discussed`() {
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
}
