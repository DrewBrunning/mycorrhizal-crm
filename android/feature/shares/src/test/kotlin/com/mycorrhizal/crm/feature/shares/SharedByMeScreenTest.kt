package com.mycorrhizal.crm.feature.shares

import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.domain.repository.ShareGrantRepository
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
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class SharedByMeScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val repository = mockk<ShareGrantRepository>()

    private val contactGrant = OutgoingShareGrant(
        id = "g1",
        toUsername = "casey",
        entityType = "contact",
        label = "Dana White",
        sections = listOf("emails", "phones"),
        reconfirmSections = listOf("addresses"),
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

    private fun screen() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                SharedByMeScreen(onBack = {}, viewModel = SharedByMeViewModel(repository))
            }
        }
    }

    @Test
    fun `rows show recipient status expiry and the live circle member count`() {
        stub(contactGrant, circleGrant, revoked)
        screen()

        composeTestRule.onNodeWithText("Dana White").assertIsDisplayed()
        composeTestRule.onAllNodesWithText("To casey").assertCountEquals(3)
        composeTestRule.onNodeWithText("Active").assertIsDisplayed()
        composeTestRule.onNodeWithText("Expires 15.01.2027").assertIsDisplayed()
        composeTestRule.onNodeWithText("Pending").assertIsDisplayed()
        composeTestRule.onNodeWithText("3 members").assertIsDisplayed()
        composeTestRule.onNodeWithText("No expiry").assertIsDisplayed()
        composeTestRule.onNodeWithText("Revoked").assertIsDisplayed()
        composeTestRule.onNodeWithText("The shared item was deleted").assertIsDisplayed()
    }

    @Test
    fun `a finished grant has no actions menu`() {
        stub(revoked)
        screen()
        composeTestRule.onNodeWithContentDescription("More actions for Note").assertDoesNotExist()
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
        composeTestRule.onNodeWithText("Entity not found").assertIsDisplayed()
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

        composeTestRule.onNodeWithText("Entity not found").assertIsDisplayed()
        composeTestRule.onNodeWithText("RAW SERVER TEXT").assertDoesNotExist()
    }
}
