package com.mycorrhizal.crm.feature.shares

import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasScrollAction
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollToNode
import com.mycorrhizal.crm.domain.repository.ShareGrantRepository
import com.mycorrhizal.crm.model.network.ErrorReason
import com.mycorrhizal.crm.model.network.ErrorReasonCode
import com.mycorrhizal.crm.model.network.IncomingShareGrant
import com.mycorrhizal.crm.model.network.IncomingShareGrantsPage
import com.mycorrhizal.crm.model.network.ShareBlock
import com.mycorrhizal.crm.model.network.ShareGrantStatuses
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.a11y.assertAccessibleSemantics
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class SharedWithMeScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val repository = mockk<ShareGrantRepository>(relaxed = false)

    private val pending = IncomingShareGrant(
        id = "g1",
        fromUsername = "dana",
        entityType = "contact",
        label = "Dana White",
        sections = listOf("emails"),
        status = ShareGrantStatuses.PENDING
    )
    private val active = IncomingShareGrant(
        id = "g2",
        fromUsername = "dana",
        entityType = "note",
        label = "Note",
        status = ShareGrantStatuses.ACTIVE,
        expiresAt = "2027-01-15T00:00:00Z"
    )

    private fun stub(grants: List<IncomingShareGrant>, blocks: List<ShareBlock> = emptyList()) {
        coEvery { repository.listIncoming(any(), any()) } returns
            Result.success(IncomingShareGrantsPage(shareGrants = grants))
        coEvery { repository.blocks() } returns Result.success(blocks)
    }

    private fun screen(onOpen: (IncomingShareGrant) -> Unit = {}) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                SharedWithMeScreen(
                    onBack = {},
                    onOpenGrant = onOpen,
                    viewModel = SharedWithMeViewModel(repository)
                )
            }
        }
    }

    @Test
    fun `a pending row shows the label and sender and offers accept and decline, no content`() {
        stub(listOf(pending))
        screen()

        composeTestRule.onNodeWithText("Dana White").assertIsDisplayed()
        composeTestRule.onNodeWithText("From dana").assertIsDisplayed()
        composeTestRule.onNodeWithText("Contact").assertIsDisplayed()
        composeTestRule.onNodeWithText("Accept").assertIsDisplayed()
        composeTestRule.onNodeWithText("Decline").assertIsDisplayed()
        composeTestRule.onNodeWithText(
            "Waiting for you. Nothing is visible until you accept."
        ).assertIsDisplayed()
        // The list screen never reads grant content: a pending row must expose nothing.
        coVerify(exactly = 0) { repository.content(any()) }
    }

    @Test
    fun `accept calls the repository`() {
        stub(listOf(pending))
        coEvery { repository.accept("g1") } returns Result.success(pending.copy(status = "active"))
        screen()

        composeTestRule.onNodeWithText("Accept").performClick()

        coVerify(exactly = 1) { repository.accept("g1") }
    }

    @Test
    fun `decline asks first and then calls decline without block`() {
        stub(listOf(pending))
        coEvery { repository.decline("g1", false) } returns
            Result.success(pending.copy(status = "declined"))
        screen()

        composeTestRule.onNodeWithText("Decline").performClick()
        composeTestRule.onNodeWithText("Decline this share?").assertIsDisplayed()
        coVerify(exactly = 0) { repository.decline(any(), any()) }
        composeTestRule.onNodeWithTag("grant-prompt-confirm").performClick()

        coVerify(exactly = 1) { repository.decline("g1", false) }
    }

    @Test
    fun `block is reached from the overflow menu, explained, and declines with block`() {
        stub(listOf(pending))
        coEvery { repository.decline("g1", true) } returns
            Result.success(pending.copy(status = "declined"))
        screen()

        composeTestRule.onNodeWithContentDescription("More actions for Dana White").performClick()
        composeTestRule.onNodeWithText("Block sender").performClick()
        composeTestRule.onNodeWithText("Block dana?").assertIsDisplayed()
        coVerify(exactly = 0) { repository.decline(any(), any()) }
        composeTestRule.onNodeWithTag("grant-prompt-confirm").performClick()

        coVerify(exactly = 1) { repository.decline("g1", true) }
    }

    @Test
    fun `an active row can be opened and left after confirmation`() {
        stub(listOf(active))
        coEvery { repository.leave("g2") } returns Result.success(active.copy(status = "declined"))
        var opened: IncomingShareGrant? = null
        screen { opened = it }

        composeTestRule.onNodeWithText("View").performClick()
        assertEquals("g2", opened?.id)

        composeTestRule.onNodeWithContentDescription("More actions for Note").performClick()
        composeTestRule.onNodeWithText("Stop receiving").performClick()
        composeTestRule.onNodeWithText("Stop receiving this share?").assertIsDisplayed()
        coVerify(exactly = 0) { repository.leave(any()) }
        composeTestRule.onNodeWithTag("grant-prompt-confirm").performClick()
        coVerify(exactly = 1) { repository.leave("g2") }
    }

    // Issue #2626: the server labels an active note grant with the note's first line; the row
    // shows it, and only an absent label falls back to the generic "Note".
    @Test
    fun `an active note row shows the first-line label the server sends`() {
        stub(listOf(active.copy(label = "Gift ideas for Dana")))
        screen()

        composeTestRule.onNodeWithText("Gift ideas for Dana").assertIsDisplayed()
        // The type stays as the caption under the label.
        composeTestRule.onNodeWithText("Note").assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("More actions for Gift ideas for Dana")
            .assertIsDisplayed()
    }

    @Test
    fun `an absent label falls back to the type name in the row, its menu and the dialogs`() {
        stub(listOf(active.copy(label = "")))
        coEvery { repository.leave("g2") } returns Result.success(active.copy(status = "declined"))
        screen()

        // Row title and type caption.
        composeTestRule.onAllNodesWithText("Note").assertCountEquals(2)
        composeTestRule.onNodeWithContentDescription("More actions for Note").performClick()
        composeTestRule.onNodeWithText("Stop receiving").performClick()
        composeTestRule.onNodeWithText(
            "Note will no longer be visible to you. To see it again, dana has to share it " +
                "with you again."
        ).assertIsDisplayed()
    }

    @Test
    fun `blocked senders are listed and can be unblocked`() {
        stub(listOf(pending), blocks = listOf(ShareBlock(7, "mallory")))
        coEvery { repository.unblock(7) } returns Result.success(Unit)
        screen()

        composeTestRule.onNodeWithText("Blocked senders").assertIsDisplayed()
        composeTestRule.onNodeWithText("mallory").assertIsDisplayed()
        composeTestRule.onNodeWithText("Unblock").performClick()

        coVerify(exactly = 1) { repository.unblock(7) }
    }

    @Test
    fun `an action error is worded from its reason code, never the raw server text`() {
        stub(listOf(pending))
        coEvery { repository.accept("g1") } returns Result.failure(
            ApiError.Client(
                409,
                "RAW SERVER TEXT",
                reason = ErrorReason(ErrorReasonCode.SHARE_GRANT_STATE_CONFLICT, emptyMap())
            )
        )
        screen()

        composeTestRule.onNodeWithText("Accept").performClick()

        composeTestRule.onNodeWithText("This share can no longer be changed that way")
            .assertIsDisplayed()
        composeTestRule.onNodeWithText("RAW SERVER TEXT").assertDoesNotExist()
    }

    @Test
    fun `a failed load shows a retryable error and no rows`() {
        coEvery { repository.listIncoming(any(), any()) } returns
            Result.failure(ApiError.Server(503, "down"))
        coEvery { repository.blocks() } returns Result.success(emptyList())
        screen()

        composeTestRule.onNodeWithText("Dana White").assertDoesNotExist()
        composeTestRule.onNodeWithText("Retry").assertIsDisplayed()
    }

    @Test
    fun `an empty list says nothing is shared`() {
        stub(emptyList())
        screen()
        composeTestRule.onNodeWithText("Nothing shared with you yet").assertIsDisplayed()
    }

    @Test
    fun `the screen has no accessibility violations`() {
        stub(listOf(pending, active), blocks = listOf(ShareBlock(7, "mallory")))
        screen()
        composeTestRule.assertAccessibleSemantics()
    }

    // Issue #2643: pending and active rows say which sections are offered.
    @Test
    fun `rows list the offered sections, shortened after three`() {
        stub(
            listOf(
                pending.copy(id = "a", label = "A", sections = emptyList()),
                pending.copy(id = "b", label = "B", sections = listOf("emails")),
                pending.copy(
                    id = "c",
                    label = "C",
                    sections = listOf("emails", "phones", "addresses")
                ),
                active.copy(
                    id = "d",
                    label = "D",
                    entityType = "contact",
                    sections = listOf("emails", "phones", "addresses", "notes")
                )
            )
        )
        screen()

        composeTestRule.onNodeWithText("Offers: Emails").assertExists()
        composeTestRule.onNodeWithText("Offers: Emails, Phones, Addresses").assertExists()
        // The fourth row is below the fold of the lazy list.
        composeTestRule.onNode(hasScrollAction())
            .performScrollToNode(hasText("Offers: Emails, Phones, Addresses and 1 more"))
        composeTestRule.onNodeWithText(
            "Offers: Emails, Phones, Addresses and 1 more"
        ).assertIsDisplayed()
        // Zero sections: no line.
        composeTestRule.onAllNodesWithText("Offers:", substring = true).assertCountEquals(3)
    }

    // Issue #2656: blocking the only sender must not leave a bare "Blocked senders" header.
    @Test
    fun `with no grants but a blocked sender the empty state still shows`() {
        stub(emptyList(), blocks = listOf(ShareBlock(7, "mallory")))
        screen()

        composeTestRule.onNodeWithText("Nothing shared with you yet").assertIsDisplayed()
        composeTestRule.onNodeWithText("Blocked senders").assertIsDisplayed()
        composeTestRule.onNodeWithText("mallory").assertIsDisplayed()
    }

    @Test
    fun `with grants there is no empty-state line`() {
        stub(listOf(pending), blocks = listOf(ShareBlock(7, "mallory")))
        screen()
        composeTestRule.onNodeWithText("Nothing shared with you yet").assertDoesNotExist()
    }
}
