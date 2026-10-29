package com.mycorrhizal.crm.feature.trash

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.TrashedContact
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.a11y.assertAccessibleSemantics
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.every
import io.mockk.mockk
import io.mockk.verify
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.receiveAsFlow
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], qualifiers = "w480dp-h1600dp")
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class TrashScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val ada = TrashedContact(
        contact = ContactSummary(id = 1, firstname = "Ada", lastname = "Test"),
        deletedAt = "2026-10-01T00:00:00Z",
        purgeAt = "2026-10-31T00:00:00Z"
    )
    private val bob = TrashedContact(
        contact = ContactSummary(id = 2, firstname = "Bob", lastname = "Test"),
        deletedAt = "2026-10-02T00:00:00Z",
        purgeAt = null
    )

    @Test
    fun `rows show the name and fire restore and delete-forever for that row`() {
        var restored: Int? = null
        var purged: Int? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                TrashList(
                    items = listOf(ada, bob),
                    retentionDays = 30,
                    enabled = true,
                    onRestore = { restored = it.contact.id },
                    onPurge = { purged = it.contact.id }
                )
            }
        }

        composeTestRule.onNodeWithText("Ada Test").assertIsDisplayed()
        composeTestRule.onNodeWithText("Bob Test").assertIsDisplayed()
        composeTestRule.onNodeWithTag("trash-restore-2").performClick()
        composeTestRule.onNodeWithTag("trash-delete-1").performClick()
        assertEquals(2, restored)
        assertEquals(1, purged)
    }

    @Test
    fun `retention zero says the contacts are kept until the trash is emptied`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                TrashList(
                    items = listOf(bob),
                    retentionDays = 0,
                    enabled = true,
                    onRestore = {},
                    onPurge = {}
                )
            }
        }

        composeTestRule.onNodeWithTag("trash-retention").assertIsDisplayed()
        composeTestRule.onNodeWithText("Deleted contacts are kept until you empty the trash.")
            .assertIsDisplayed()
    }

    @Test
    fun `list passes the accessibility sweep`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                TrashList(
                    items = listOf(ada, bob),
                    retentionDays = 30,
                    enabled = true,
                    onRestore = {},
                    onPurge = {}
                )
            }
        }

        composeTestRule.assertAccessibleSemantics()
    }

    private fun screenWith(
        state: TrashUiState,
        events: Channel<TrashUiEvent> = Channel(Channel.BUFFERED),
        onMenuClick: (() -> Unit)? = {}
    ): TrashViewModel {
        val vm = mockk<TrashViewModel>(relaxed = true)
        every { vm.uiState } returns MutableStateFlow(state)
        every { vm.events } returns events.receiveAsFlow()
        composeTestRule.setContent {
            MycorrhizalTheme { TrashScreen(onMenuClick = onMenuClick, viewModel = vm) }
        }
        return vm
    }

    @Test
    fun `loading with no items shows the skeleton`() {
        screenWith(TrashUiState(isLoading = true))
        composeTestRule.onNodeWithTag("trash-loading").assertIsDisplayed()
        composeTestRule.onNodeWithTag("trash-empty").assertDoesNotExist()
    }

    @Test
    fun `a failed first load shows the error text`() {
        screenWith(TrashUiState(error = ApiError.Client(400, "boom")))
        composeTestRule.onNodeWithText("boom").assertIsDisplayed()
    }

    @Test
    fun `an empty trash shows the empty state with the retention window`() {
        screenWith(TrashUiState(retentionDays = 30))
        composeTestRule.onNodeWithText("Trash is empty").assertIsDisplayed()
        composeTestRule.onNodeWithText(
            "Deleted contacts are kept for 30 days, then deleted forever."
        )
            .assertIsDisplayed()
    }

    @Test
    fun `menu button is shown and fires when a drawer handler is given`() {
        var clicked = false
        screenWith(TrashUiState(), onMenuClick = { clicked = true })
        composeTestRule.onNodeWithContentDescription("Menu").performClick()
        assertTrue(clicked)
    }

    @Test
    fun `menu button is hidden without a drawer handler`() {
        screenWith(TrashUiState(), onMenuClick = null)
        composeTestRule.onNodeWithContentDescription("Menu").assertDoesNotExist()
    }

    @Test
    fun `restore delegates to the view model`() {
        val vm = screenWith(TrashUiState(items = listOf(ada), retentionDays = 30))
        composeTestRule.onNodeWithTag("trash-restore-1").performClick()
        verify { vm.restore(ada) }
    }

    @Test
    fun `delete forever asks first and purges only after confirming`() {
        val vm = screenWith(TrashUiState(items = listOf(ada), retentionDays = 30))
        composeTestRule.onNodeWithTag("trash-delete-1").performClick()
        composeTestRule.onNodeWithText("Permanently delete Ada Test? This cannot be undone.")
            .assertIsDisplayed()
        verify(exactly = 0) { vm.purge(any()) }
        composeTestRule.onNodeWithTag("trash-purge-confirm").performClick()
        verify { vm.purge(ada) }
        composeTestRule.onNodeWithTag("trash-purge-confirm").assertDoesNotExist()
    }

    @Test
    fun `cancelling the purge dialog purges nothing`() {
        val vm = screenWith(TrashUiState(items = listOf(ada), retentionDays = 30))
        composeTestRule.onNodeWithTag("trash-delete-1").performClick()
        composeTestRule.onNodeWithText("Cancel").performClick()
        verify(exactly = 0) { vm.purge(any()) }
        composeTestRule.onNodeWithTag("trash-purge-confirm").assertDoesNotExist()
    }

    @Test
    fun `empty trash asks first and empties only after confirming`() {
        val vm = screenWith(TrashUiState(items = listOf(ada, bob), retentionDays = 30))
        composeTestRule.onNodeWithTag("trash-empty").performClick()
        composeTestRule.onNodeWithText(
            "Permanently delete all 2 contacts in the Trash? This cannot be undone."
        )
            .assertIsDisplayed()
        verify(exactly = 0) { vm.emptyTrash() }
        composeTestRule.onNodeWithTag("trash-empty-confirm").performClick()
        verify { vm.emptyTrash() }
        composeTestRule.onNodeWithTag("trash-empty-confirm").assertDoesNotExist()
    }

    @Test
    fun `cancelling the empty dialog empties nothing`() {
        val vm = screenWith(TrashUiState(items = listOf(ada), retentionDays = 30))
        composeTestRule.onNodeWithTag("trash-empty").performClick()
        composeTestRule.onNodeWithText("Cancel").performClick()
        verify(exactly = 0) { vm.emptyTrash() }
        composeTestRule.onNodeWithTag("trash-empty-confirm").assertDoesNotExist()
    }

    @Test
    fun `actions are disabled while a mutation is in flight`() {
        screenWith(TrashUiState(items = listOf(ada), retentionDays = 30, isWorking = true))
        composeTestRule.onNodeWithTag("trash-empty").assertIsNotEnabled()
        composeTestRule.onNodeWithTag("trash-restore-1").assertIsNotEnabled()
    }

    @Test
    fun `a refresh failure with items still listed surfaces as a snackbar`() {
        screenWith(
            TrashUiState(
                items = listOf(ada),
                retentionDays = 30,
                error = ApiError.Client(400, "refresh failed")
            )
        )
        composeTestRule.onNodeWithText("refresh failed").assertIsDisplayed()
    }

    @Test
    fun `events are announced as snackbars`() {
        val cases = listOf(
            TrashUiEvent.Restored("Ada", 0) to "Restored Ada.",
            TrashUiEvent.Restored("Ada", 1) to
                "Restored Ada, but 1 item could not be restored because what it linked to no longer exists.",
            TrashUiEvent.RestoreConflict("Ada") to
                "Can't restore Ada: another contact already uses this contact's ID. Merge or delete that contact first.",
            TrashUiEvent.Purged("Ada") to "Permanently deleted Ada.",
            TrashUiEvent.Emptied(2) to "Permanently deleted 2 contacts.",
            TrashUiEvent.Failed(ApiError.Client(400, "nope")) to "nope"
        )
        val events = Channel<TrashUiEvent>(Channel.BUFFERED)
        screenWith(TrashUiState(), events)
        for ((event, text) in cases) {
            events.trySend(event)
            composeTestRule.waitUntil(5_000) {
                composeTestRule.onAllNodesWithText(text).fetchSemanticsNodes().isNotEmpty()
            }
            composeTestRule.mainClock.advanceTimeBy(11_000)
        }
    }
}
