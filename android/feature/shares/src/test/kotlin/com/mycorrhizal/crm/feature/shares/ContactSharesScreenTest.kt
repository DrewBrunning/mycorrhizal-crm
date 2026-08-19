package com.mycorrhizal.crm.feature.shares

import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.domain.repository.ContactShareRepository
import com.mycorrhizal.crm.model.network.ContactShare
import com.mycorrhizal.crm.model.network.ContactShareStatuses
import com.mycorrhizal.crm.model.network.ContactSharesPage
import com.mycorrhizal.crm.model.network.ImportPreviewResponse
import com.mycorrhizal.crm.model.network.ImportRowPreview
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
@OptIn(ExperimentalMaterial3Api::class)
class ContactSharesScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val repository = mockk<ContactShareRepository>()

    private val incomingShare = ContactShare(
        id = "s1",
        fromUserId = 7,
        toUserId = 1,
        contactDisplayName = "Dana White",
        status = ContactShareStatuses.PENDING,
    )

    private fun screen(
        viewModel: ContactSharesViewModel = ContactSharesViewModel(repository),
        darkTheme: Boolean = false,
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme(darkTheme = darkTheme) {
                ContactSharesScreen(onMenuClick = {}, viewModel = viewModel)
            }
        }
    }

    @Test
    fun `incoming pending share renders with accept and decline actions`() {
        coEvery { repository.listIncoming(any(), any()) } returns Result.success(
            ContactSharesPage(contactShares = listOf(incomingShare), usernames = mapOf("7" to "dana")),
        )
        coEvery { repository.listOutgoing(any(), any()) } returns Result.success(ContactSharesPage())

        screen()

        composeTestRule.onNodeWithText("Dana White").assertIsDisplayed()
        composeTestRule.onNodeWithText("From dana").assertIsDisplayed()
        composeTestRule.onNodeWithText("Accept").assertIsDisplayed()
        composeTestRule.onNodeWithText("Decline").assertIsDisplayed()
    }

    @Test
    fun `tapping decline opens the confirmation dialog and does not call the repository`() {
        coEvery { repository.listIncoming(any(), any()) } returns Result.success(
            ContactSharesPage(contactShares = listOf(incomingShare)),
        )
        coEvery { repository.listOutgoing(any(), any()) } returns Result.success(ContactSharesPage())

        screen()

        composeTestRule.onNodeWithText("Decline").performClick()

        // Confirmation dialog gates the repository call (M15 test case 3).
        composeTestRule.onNodeWithText("Decline this share?").assertIsDisplayed()
        coVerify(exactly = 0) { repository.decline(any()) }
    }

    @Test
    fun `confirming the decline dialog calls the repository and removes the share`() {
        // First load returns the pending share; after the decline-triggered
        // reload the inbox is empty (the share was removed).
        coEvery { repository.listIncoming(any(), any()) } returnsMany listOf(
            Result.success(ContactSharesPage(contactShares = listOf(incomingShare))),
            Result.success(ContactSharesPage()),
        )
        coEvery { repository.listOutgoing(any(), any()) } returns Result.success(ContactSharesPage())
        coEvery { repository.decline("s1") } returns Result.success(Unit)

        screen()

        composeTestRule.onNodeWithText("Decline").performClick()
        // The dialog's confirm button — scoped by tag since the row's Decline
        // button carries the same label.
        composeTestRule.onNodeWithTag("decline-confirm").performClick()

        coVerify(exactly = 1) { repository.decline("s1") }
    }

    @Test
    fun `tapping accept opens the preview dialog`() {
        coEvery { repository.listIncoming(any(), any()) } returns Result.success(
            ContactSharesPage(contactShares = listOf(incomingShare)),
        )
        coEvery { repository.listOutgoing(any(), any()) } returns Result.success(ContactSharesPage())
        coEvery { repository.accept("s1") } returns Result.success(
            ImportPreviewResponse(
                sessionId = "import-1",
                rows = listOf(ImportRowPreview(rowIndex = 0, suggestedAction = "add")),
                totalRows = 1,
            ),
        )

        screen()

        composeTestRule.onNodeWithText("Accept").performClick()

        composeTestRule.onNodeWithText("Review shared contact").assertIsDisplayed()
        composeTestRule.onNodeWithText("Add as new contact").assertIsDisplayed()
    }

    // --- Issue #214: Compose semantics a11y sweep (the axe-core analog) -----

    private fun setUpPopulatedShares() {
        coEvery { repository.listIncoming(any(), any()) } returns Result.success(
            ContactSharesPage(contactShares = listOf(incomingShare), usernames = mapOf("7" to "dana")),
        )
        coEvery { repository.listOutgoing(any(), any()) } returns Result.success(ContactSharesPage())
    }

    @Test
    fun `contact shares has no accessibility violations (light)`() {
        setUpPopulatedShares()

        screen(darkTheme = false)

        composeTestRule.assertAccessibleSemantics()
    }

    @Test
    fun `contact shares has no accessibility violations (dark)`() {
        setUpPopulatedShares()

        screen(darkTheme = true)

        composeTestRule.assertAccessibleSemantics()
    }
}
