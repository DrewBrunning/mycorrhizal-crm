package com.mycorrhizal.crm.feature.shares

import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertTextEquals
import androidx.compose.ui.test.isSelectable
import androidx.compose.ui.test.isToggleable
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.ShareGrantRepository
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.ErrorReason
import com.mycorrhizal.crm.model.network.ErrorReasonCode
import com.mycorrhizal.crm.model.network.OutgoingShareGrant
import com.mycorrhizal.crm.model.network.OutgoingShareGrantsPage
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

// Issue #2642: decline, leave and block are one `declined` to the sender (ADR 0042).
private const val DECLINED_LABEL = "Declined or ended by the recipient"

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class SharedByMeScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val repository = mockk<ShareGrantRepository>()
    private val contacts = mockk<ContactRepository>()

    private val contactGrant = OutgoingShareGrant(
        id = "g1",
        toUsername = "casey",
        entityType = "contact",
        label = "Dana White",
        entityId = "uid-1",
        sections = listOf("emails", "phones"),
        reconfirmSections = listOf("emails", "addresses"),
        status = ShareGrantStatuses.ACTIVE,
        expiresAt = "2027-01-15T00:00:00Z",
        revision = 2
    )
    private val circleGrant = OutgoingShareGrant(
        id = "g2",
        toUsername = "casey",
        entityType = "circle",
        label = "Family",
        status = ShareGrantStatuses.PENDING,
        memberCount = 3,
        revision = 1
    )
    private val revoked = OutgoingShareGrant(
        id = "g3",
        toUsername = "casey",
        entityType = "note",
        label = "Note",
        status = ShareGrantStatuses.REVOKED,
        revokedReason = "entity_deleted"
    )

    private fun stub(vararg grants: OutgoingShareGrant) {
        coEvery { repository.listOutgoing(any(), any()) } returns
            Result.success(OutgoingShareGrantsPage(shareGrants = grants.toList()))
    }

    private fun screen(onOpenContact: (Int) -> Unit = {}) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                SharedByMeScreen(
                    onBack = {},
                    onOpenContact = onOpenContact,
                    viewModel = SharedByMeViewModel(repository, contacts)
                )
            }
        }
    }

    @Test
    fun `Load more appends the next page and clears the affordance`() {
        val older = contactGrant.copy(id = "g4", label = "Older Share")
        coEvery { repository.listOutgoing(any(), any()) } returnsMany listOf(
            Result.success(
                OutgoingShareGrantsPage(shareGrants = listOf(contactGrant), nextCursor = "c1")
            ),
            Result.success(OutgoingShareGrantsPage(shareGrants = listOf(older), nextCursor = ""))
        )
        screen()

        composeTestRule.onNodeWithText("Load more").performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Older Share").assertIsDisplayed()
        composeTestRule.onNodeWithText("Load more").assertDoesNotExist()
    }

    @Test
    fun `rows show a status chip, one recipient-and-expiry line and the live circle member count`() {
        stub(contactGrant, circleGrant, revoked)
        screen()

        composeTestRule.onNodeWithText("Dana White").assertIsDisplayed()
        // Issue #2627: one line per row instead of separate recipient and expiry captions.
        composeTestRule.onNodeWithText("To casey · Expires 15.01.2027").assertIsDisplayed()
        composeTestRule.onNodeWithText("To casey · No expiry").assertIsDisplayed()
        // A finished grant has no expiry to report.
        composeTestRule.onNodeWithText("To casey").assertIsDisplayed()
        composeTestRule.onNodeWithText("3 members").assertIsDisplayed()
        composeTestRule.onNodeWithText("The shared item was deleted").assertIsDisplayed()
        // The status is a chip per row, not a plain caption.
        composeTestRule.onNodeWithTag("grant-status-g1").assertTextEquals("Active")
        composeTestRule.onNodeWithTag("grant-status-g2").assertTextEquals("Pending")
        composeTestRule.onNodeWithTag("grant-status-g3").assertTextEquals("Revoked")
    }

    @Test
    fun `declined and expired grants get their own status chip`() {
        stub(
            contactGrant.copy(id = "d", status = ShareGrantStatuses.DECLINED),
            contactGrant.copy(id = "x", status = ShareGrantStatuses.EXPIRED)
        )
        screen()

        composeTestRule.onNodeWithTag("grant-status-d").assertTextEquals(DECLINED_LABEL)
        composeTestRule.onNodeWithTag("grant-status-x").assertTextEquals("Expired")
    }

    // Issue #2643: what was shared, for 0, 1, 3 and 4+ sections.
    @Test
    fun `a live contact grant lists the shared sections, shortened after three`() {
        val base = contactGrant.copy(reconfirmSections = emptyList())
        stub(
            base.copy(id = "a", label = "A", sections = emptyList()),
            base.copy(id = "b", label = "B", sections = listOf("emails")),
            base.copy(id = "c", label = "C", sections = listOf("emails", "phones", "addresses")),
            base.copy(
                id = "d",
                label = "D",
                sections = listOf("emails", "phones", "addresses", "notes", "links")
            )
        )
        screen()

        composeTestRule.onNodeWithText("Shared: Emails").assertExists()
        composeTestRule.onNodeWithText("Shared: Emails, Phones, Addresses").assertExists()
        composeTestRule.onNodeWithText(
            "Shared: Emails, Phones, Addresses and 2 more"
        ).assertExists()
        // Zero sections: no sections line at all.
        composeTestRule.onAllNodesWithText("Shared:", substring = true).assertCountEquals(3)
    }

    @Test
    fun `a finished grant does not list sections`() {
        stub(contactGrant.copy(status = ShareGrantStatuses.DECLINED))
        screen()
        composeTestRule.onAllNodesWithText("Shared:", substring = true).assertCountEquals(0)
    }

    // Issue #2609: every toggle/option row of the edit dialog is a 48dp touch target.
    @Test
    fun `edit dialog section and expiry rows are at least 48dp tall`() {
        stub(contactGrant)
        screen()

        composeTestRule.onNodeWithContentDescription("More actions for Dana White").performClick()
        composeTestRule.onNodeWithText("Edit").performClick()

        // emails + phones sections; Keep, No expiry and the four presets.
        composeTestRule.assertRowsAtLeast48dp(isToggleable(), expectedCount = 2)
        composeTestRule.assertRowsAtLeast48dp(isSelectable(), expectedCount = 6)
    }

    @Test
    fun `a finished grant offers only Remove, never Edit or Revoke`() {
        stub(revoked)
        screen()
        composeTestRule.onNodeWithContentDescription("More actions for Note").performClick()
        composeTestRule.onNodeWithText("Remove").assertIsDisplayed()
        composeTestRule.onNodeWithText("Edit").assertDoesNotExist()
        composeTestRule.onNodeWithText("Revoke").assertDoesNotExist()
    }

    @Test
    fun `a live grant does not offer Remove`() {
        stub(circleGrant)
        screen()
        composeTestRule.onNodeWithContentDescription("More actions for Family").performClick()
        composeTestRule.onNodeWithText("Revoke").assertIsDisplayed()
        composeTestRule.onNodeWithText("Remove").assertDoesNotExist()
    }

    @Test
    fun `remove is explained, deletes only after confirmation and the row disappears`() {
        val declined = contactGrant.copy(id = "g4", status = ShareGrantStatuses.DECLINED)
        coEvery { repository.listOutgoing(any(), any()) } returnsMany listOf(
            Result.success(OutgoingShareGrantsPage(shareGrants = listOf(declined, circleGrant))),
            Result.success(OutgoingShareGrantsPage(shareGrants = listOf(circleGrant)))
        )
        coEvery { repository.delete("g4") } returns Result.success(Unit)
        screen()

        composeTestRule.onNodeWithTag("grant-status-g4").assertTextEquals(DECLINED_LABEL)
        composeTestRule.onNodeWithContentDescription("More actions for Dana White").performClick()
        composeTestRule.onNodeWithText("Remove").performClick()
        composeTestRule.onNodeWithText("Remove this share?").assertIsDisplayed()
        composeTestRule.onNodeWithText(
            "This share with casey has ended. Removing it clears it from this list."
        ).assertIsDisplayed()
        coVerify(exactly = 0) { repository.delete(any()) }
        composeTestRule.onNodeWithTag("grant-prompt-confirm").performClick()
        composeTestRule.waitForIdle()

        coVerify(exactly = 1) { repository.delete("g4") }
        composeTestRule.onNodeWithTag("grant-status-g4").assertDoesNotExist()
        composeTestRule.onNodeWithTag("grant-status-g2").assertIsDisplayed()
    }

    @Test
    fun `cancelling the remove dialog deletes nothing`() {
        stub(revoked)
        screen()
        composeTestRule.onNodeWithContentDescription("More actions for Note").performClick()
        composeTestRule.onNodeWithText("Remove").performClick()
        composeTestRule.onNodeWithText("Cancel").performClick()
        composeTestRule.waitForIdle()
        coVerify(exactly = 0) { repository.delete(any()) }
        composeTestRule.onNodeWithText("Remove this share?").assertDoesNotExist()
    }

    @Test
    fun `a 404 after remove is treated as done with no error`() {
        val declined = contactGrant.copy(id = "g4", status = ShareGrantStatuses.DECLINED)
        coEvery { repository.listOutgoing(any(), any()) } returnsMany listOf(
            Result.success(OutgoingShareGrantsPage(shareGrants = listOf(declined))),
            Result.success(OutgoingShareGrantsPage(shareGrants = emptyList()))
        )
        coEvery { repository.delete("g4") } returns Result.failure(
            ApiError.Client(
                404,
                "RAW SERVER TEXT",
                reason = ErrorReason(ErrorReasonCode.SHARE_GRANT_NOT_FOUND, emptyMap())
            )
        )
        screen()

        composeTestRule.onNodeWithContentDescription("More actions for Dana White").performClick()
        composeTestRule.onNodeWithText("Remove").performClick()
        composeTestRule.onNodeWithTag("grant-prompt-confirm").performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Share grant not found").assertDoesNotExist()
        composeTestRule.onNodeWithText("RAW SERVER TEXT").assertDoesNotExist()
        composeTestRule.onNodeWithTag("grant-status-g4").assertDoesNotExist()
    }

    @Test
    fun `a remove refused as not ended is worded from its reason code, never the server text`() {
        val declined = contactGrant.copy(id = "g4", status = ShareGrantStatuses.DECLINED)
        stub(declined)
        coEvery { repository.delete("g4") } returns Result.failure(
            ApiError.Client(
                409,
                "RAW SERVER TEXT",
                reason = ErrorReason(ErrorReasonCode.SHARE_GRANT_NOT_ENDED, emptyMap())
            )
        )
        screen()

        composeTestRule.onNodeWithContentDescription("More actions for Dana White").performClick()
        composeTestRule.onNodeWithText("Remove").performClick()
        composeTestRule.onNodeWithTag("grant-prompt-confirm").performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText(
            "Only a share that has ended (declined, expired or revoked) can be removed; " +
                "revoke it first"
        ).assertIsDisplayed()
        composeTestRule.onNodeWithText("RAW SERVER TEXT").assertDoesNotExist()
        // The row is still there: nothing was removed.
        composeTestRule.onNodeWithTag("grant-status-g4").assertIsDisplayed()
    }

    // Issue #2626: the server labels a note grant with the note's first line; the row shows it
    // and only an absent label falls back to the generic type name.
    @Test
    fun `a note row shows its first-line label, and an absent label falls back to Note`() {
        stub(
            revoked.copy(id = "n1", label = "Call plan for Dana's birthday"),
            revoked.copy(id = "n2", label = "")
        )
        screen()

        composeTestRule.onNodeWithText("Call plan for Dana's birthday").assertIsDisplayed()
        // The blank-label row: title and type caption are both "Note"; the first row only has
        // the caption.
        composeTestRule.onAllNodesWithText("Note").assertCountEquals(3)
        composeTestRule.onNodeWithContentDescription(
            "More actions for Call plan for Dana's birthday"
        ).assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("More actions for Note").assertIsDisplayed()
    }

    @Test
    fun `revoke is explained and calls revoke only after confirmation`() {
        stub(circleGrant)
        coEvery { repository.revoke("g2") } returns
            Result.success(circleGrant.copy(status = ShareGrantStatuses.REVOKED))
        screen()

        composeTestRule.onNodeWithContentDescription("More actions for Family").performClick()
        composeTestRule.onNodeWithText("Revoke").performClick()
        composeTestRule.onNodeWithText("Revoke this share?").assertIsDisplayed()
        composeTestRule.onNodeWithText(
            "casey will lose access the next time they load it. " +
                "Anything they have already seen cannot be taken back."
        ).assertIsDisplayed()
        coVerify(exactly = 0) { repository.revoke(any()) }
        composeTestRule.onNodeWithTag("grant-prompt-confirm").performClick()

        coVerify(exactly = 1) { repository.revoke("g2") }
    }

    @Test
    fun `a withheld section is flagged and reconfirm names it before calling reconfirm`() {
        stub(contactGrant)
        coEvery { repository.reconfirm("g1") } returns
            Result.success(contactGrant.copy(reconfirmSections = emptyList()))
        coEvery { repository.previewContact(any(), any()) } returns Result.success(emptyMap())
        screen()

        composeTestRule.onNodeWithText(
            "Some sections are withheld until you review them"
        ).assertIsDisplayed()
        composeTestRule.onNodeWithText("Review and re-confirm").performClick()
        composeTestRule.onNodeWithText("Share these sections again?").assertIsDisplayed()
        coVerify(exactly = 0) { repository.reconfirm(any()) }
        composeTestRule.onNodeWithTag("grant-prompt-confirm").performClick()

        coVerify(exactly = 1) { repository.reconfirm("g1") }
    }

    private fun card(emails: Int, addresses: Int) = mapOf<String, Any?>(
        "emails" to (1..emails).associate { "e$it" to mapOf("address" to "a$it@x.org") },
        "addresses" to (1..addresses).associate { "d$it" to mapOf("full" to "$it Main St") }
    )

    // Issue #2659: the re-confirm dialog names each withheld section with what it holds today.
    @Test
    fun `the reconfirm dialog lists each withheld section with its item count`() {
        stub(contactGrant)
        coEvery { repository.previewContact("uid-1", listOf("addresses", "emails")) } returns
            Result.success(card(emails = 2, addresses = 1))
        screen()

        composeTestRule.onNodeWithText("Review and re-confirm").performClick()

        composeTestRule.onNodeWithText("Emails · 2 items").assertIsDisplayed()
        composeTestRule.onNodeWithText("Addresses · 1 item").assertIsDisplayed()
    }

    @Test
    fun `the reconfirm dialog still names the sections when the preview fails`() {
        stub(contactGrant)
        coEvery { repository.previewContact(any(), any()) } returns
            Result.failure(ApiError.Server(503, "down"))
        screen()

        composeTestRule.onNodeWithText("Review and re-confirm").performClick()

        composeTestRule.onNodeWithText("Emails").assertIsDisplayed()
        composeTestRule.onNodeWithText("Addresses").assertIsDisplayed()
        composeTestRule.onNodeWithText("Share these sections again?").assertIsDisplayed()
    }

    @Test
    fun `open contact from the reconfirm dialog resolves the uid and navigates`() {
        stub(contactGrant)
        coEvery { repository.previewContact(any(), any()) } returns Result.success(card(0, 0))
        coEvery { contacts.resolveByUid(listOf("uid-1")) } returns
            Result.success(mapOf("uid-1" to ContactSummary(id = 42, uid = "uid-1")))
        var opened = -1
        screen(onOpenContact = { opened = it })

        composeTestRule.onNodeWithText("Review and re-confirm").performClick()
        composeTestRule.onNodeWithTag("grant-prompt-open-contact").performClick()
        composeTestRule.waitForIdle()

        assertEquals(42, opened)
    }

    @Test
    fun `open contact for a contact that no longer resolves is worded from its reason code`() {
        stub(contactGrant)
        coEvery { repository.previewContact(any(), any()) } returns Result.success(card(0, 0))
        coEvery { contacts.resolveByUid(any()) } returns Result.failure(
            ApiError.Client(
                404,
                "RAW SERVER TEXT",
                reason = ErrorReason(ErrorReasonCode.ENTITY_NOT_FOUND, emptyMap())
            )
        )
        var opened = -1
        screen(onOpenContact = { opened = it })

        composeTestRule.onNodeWithText("Review and re-confirm").performClick()
        composeTestRule.onNodeWithTag("grant-prompt-open-contact").performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Organisation, place or pet not found").assertIsDisplayed()
        composeTestRule.onNodeWithText("RAW SERVER TEXT").assertDoesNotExist()
        assertEquals(-1, opened)
    }

    @Test
    fun `open contact for a uid with no match shows the not-found message`() {
        stub(contactGrant)
        coEvery { repository.previewContact(any(), any()) } returns Result.success(card(0, 0))
        coEvery { contacts.resolveByUid(any()) } returns Result.success(emptyMap())
        screen()

        composeTestRule.onNodeWithText("Review and re-confirm").performClick()
        composeTestRule.onNodeWithTag("grant-prompt-open-contact").performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Not found").assertIsDisplayed()
    }

    @Test
    fun `edit offers only the granted sections and saves a narrowed list`() {
        stub(contactGrant)
        coEvery { repository.update("g1", any(), 2) } returns Result.success(contactGrant)
        screen()

        composeTestRule.onNodeWithContentDescription("More actions for Dana White").performClick()
        composeTestRule.onNodeWithText("Edit").performClick()
        composeTestRule.onNodeWithText("Edit share").assertIsDisplayed()
        composeTestRule.onNodeWithText("Emails").assertIsDisplayed()
        composeTestRule.onNodeWithText("Phones").performClick()
        // Never offered: a section the grant does not have.
        composeTestRule.onNodeWithText("Notes").assertDoesNotExist()
        composeTestRule.onNodeWithTag("grant-edit-save").performClick()

        coVerify(exactly = 1) {
            repository.update("g1", match { it.sections == listOf("emails") }, 2)
        }
    }

    @Test
    fun `a failed load shows a retryable error`() {
        coEvery { repository.listOutgoing(any(), any()) } returns
            Result.failure(ApiError.Server(503, "down"))
        screen()
        composeTestRule.onNodeWithText("Retry").assertIsDisplayed()
    }

    @Test
    fun `an empty list says there are no ongoing shares`() {
        stub()
        screen()
        composeTestRule.onNodeWithText("No ongoing shares yet").assertIsDisplayed()
    }

    @Test
    fun `the screen has no accessibility violations`() {
        stub(contactGrant, circleGrant, revoked)
        screen()
        composeTestRule.assertAccessibleSemantics()
    }

    @Test
    fun `a failed load is worded from its reason code, never the server text`() {
        coEvery { repository.listOutgoing(any(), any()) } returns Result.failure(
            ApiError.Client(
                409,
                "RAW SERVER TEXT",
                reason = ErrorReason(ErrorReasonCode.ENTITY_NOT_FOUND, emptyMap())
            )
        )
        screen()
        composeTestRule.onNodeWithText("Organisation, place or pet not found").assertIsDisplayed()
        composeTestRule.onNodeWithText("RAW SERVER TEXT").assertDoesNotExist()
    }

    @Test
    fun `a failed revoke is worded from its reason code, never the server text`() {
        stub(contactGrant)
        coEvery { repository.revoke("g1") } returns Result.failure(
            ApiError.Client(
                409,
                "RAW SERVER TEXT",
                reason = ErrorReason(ErrorReasonCode.ENTITY_NOT_FOUND, emptyMap())
            )
        )
        screen()

        composeTestRule.onNodeWithContentDescription("More actions for Dana White").performClick()
        composeTestRule.onNodeWithText("Revoke").performClick()
        composeTestRule.onNodeWithTag("grant-prompt-confirm").performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Organisation, place or pet not found").assertIsDisplayed()
        composeTestRule.onNodeWithText("RAW SERVER TEXT").assertDoesNotExist()
    }
}
