package com.mycorrhizal.crm.feature.shares

import android.content.Context
import androidx.annotation.StringRes
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.isToggleable
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.lifecycle.SavedStateHandle
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.domain.repository.ContactShareRepository
import com.mycorrhizal.crm.domain.repository.ShareGrantRepository
import com.mycorrhizal.crm.model.network.ShareFieldSections
import com.mycorrhizal.crm.model.network.UserDirectoryEntry
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.a11y.assertAccessibleSemantics
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.CompletableDeferred
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issue #682: the share-a-contact screen — recipient picker, section
 * checkboxes with the sensitivity lock, the reveal step, and the share button
 * gating — had no test coverage. Mounts the real screen against a
 * [ShareContactViewModel] backed by a mocked [ContactShareRepository].
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ShareContactScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun str(@StringRes res: Int, vararg args: Any): String =
        ApplicationProvider.getApplicationContext<Context>().getString(res, *args)

    private fun viewModel(repo: ContactShareRepository): ShareContactViewModel =
        ShareContactViewModel(
            repo,
            mockk<ShareGrantRepository>(),
            SavedStateHandle(mapOf("uid" to "u-viewed"))
        )

    private fun screen(repo: ContactShareRepository, vm: ShareContactViewModel = viewModel(repo)) {
        composeTestRule.setContent {
            MycorrhizalTheme { ShareContactScreen(onBack = {}, viewModel = vm) }
        }
    }

    @Test
    fun `renders the recipient picker and sections once the directory loads`() {
        val repo = mockk<ContactShareRepository>()
        coEvery { repo.userDirectory() } returns Result.success(
            listOf(UserDirectoryEntry(id = 1, username = "alice"))
        )
        screen(repo)

        composeTestRule.onNodeWithText(str(R.string.shares_share_dialog_title)).assertIsDisplayed()
        composeTestRule.onNodeWithText(str(R.string.shares_recipient)).assertIsDisplayed()
        composeTestRule.onNodeWithText(str(R.string.shares_fields_label)).assertIsDisplayed()
        composeTestRule.onNodeWithText(
            str(R.string.shares_share_button)
        ).performScrollTo().assertIsDisplayed()
    }

    // Issue #2106: a failed directory load is worded by localizedMessage().
    @Test
    fun `a failed directory load shows the localized error`() {
        val repo = mockk<ContactShareRepository>()
        coEvery { repo.userDirectory() } returns Result.failure(ApiError.Client(404, "gone"))
        screen(repo)

        composeTestRule.onAllNodesWithText("Not found").assertCountEquals(2)
    }

    @Test
    fun `share is disabled until a recipient is selected`() {
        val repo = mockk<ContactShareRepository>()
        coEvery { repo.userDirectory() } returns Result.success(
            listOf(UserDirectoryEntry(id = 1, username = "alice"))
        )
        screen(repo)

        composeTestRule.onNodeWithText(str(R.string.shares_share_button))
            .performScrollTo()
            .assertIsNotEnabled()
    }

    @Test
    fun `an empty directory shows the no-recipients explanation`() {
        val repo = mockk<ContactShareRepository>()
        coEvery { repo.userDirectory() } returns Result.success(emptyList())
        screen(repo)

        composeTestRule.onNodeWithText(str(R.string.shares_no_recipients)).assertIsDisplayed()
    }

    @Test
    fun `share screen has no accessibility violations`() {
        val repo = mockk<ContactShareRepository>()
        coEvery { repo.userDirectory() } returns Result.success(
            listOf(UserDirectoryEntry(id = 1, username = "alice"))
        )
        screen(repo)

        composeTestRule.assertAccessibleSemantics()
    }

    @Test
    fun `the reveal button opens the sensitive confirmation dialog`() {
        val repo = mockk<ContactShareRepository>()
        coEvery { repo.userDirectory() } returns Result.success(
            listOf(UserDirectoryEntry(id = 1, username = "alice"))
        )
        screen(repo)

        composeTestRule.onNodeWithText(
            str(R.string.shares_reveal_button)
        ).performScrollTo().performClick()

        // The confirm gate is reachable only after the deliberate reveal step.
        composeTestRule.onNodeWithText(str(R.string.shares_reveal_title)).assertIsDisplayed()
    }

    @Test
    fun `sharing with a selected recipient calls the repository`() {
        val repo = mockk<ContactShareRepository>()
        coEvery { repo.userDirectory() } returns Result.success(
            listOf(UserDirectoryEntry(id = 1, username = "alice"))
        )
        coEvery { repo.create(any()) } returns Result.success(
            com.mycorrhizal.crm.model.network.ContactShare(id = "share-1")
        )
        val vm = viewModel(repo)
        // Recipient selection is exercised by the ViewModel suite; here the
        // screen just needs one selected to enable the share button.
        vm.selectRecipient(1)
        screen(repo, vm)

        composeTestRule.onNodeWithText(str(R.string.shares_share_button))
            .performScrollTo()
            .assertIsEnabled()
            .performClick()

        coVerify(exactly = 1) { repo.create(any()) }
    }

    // --- Issue #2293: the one-time vs standing choice ---

    private fun standingScreen(
        repo: ContactShareRepository,
        grants: ShareGrantRepository,
        onStandingShared: () -> Unit = {}
    ): ShareContactViewModel {
        val vm = ShareContactViewModel(repo, grants, SavedStateHandle(mapOf("uid" to "u-viewed")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                ShareContactScreen(onBack = {}, onStandingShared = onStandingShared, viewModel = vm)
            }
        }
        return vm
    }

    @Test
    fun `one-time is the default mode and keeps the existing description`() {
        val repo = mockk<ContactShareRepository>()
        coEvery { repo.userDirectory() } returns Result.success(
            listOf(UserDirectoryEntry(id = 1, username = "alice"))
        )
        screen(repo)

        composeTestRule.onNodeWithText(str(R.string.shares_mode_one_time)).assertIsDisplayed()
        composeTestRule.onNodeWithText(str(R.string.shares_share_dialog_description))
            .assertIsDisplayed()
        composeTestRule.onNodeWithText(
            str(R.string.shares_standing_share_button)
        ).assertDoesNotExist()
    }

    @Test
    fun `choosing keep sharing creates a grant and never a one-time share`() {
        val repo = mockk<ContactShareRepository>()
        val grants = mockk<ShareGrantRepository>()
        coEvery { grants.previewContact(any(), any()) } returns Result.success(emptyMap())
        coEvery { repo.userDirectory() } returns Result.success(
            listOf(UserDirectoryEntry(id = 1, username = "alice"))
        )
        coEvery { grants.create(any()) } returns Result.success(
            com.mycorrhizal.crm.model.network.OutgoingShareGrant(id = "g1")
        )
        var landed = false
        val vm = standingScreen(repo, grants) { landed = true }
        vm.selectRecipient(1)

        composeTestRule.onNodeWithText(str(R.string.shares_mode_standing)).performClick()
        composeTestRule.onNodeWithText(
            str(R.string.shares_standing_pending_note)
        ).assertIsDisplayed()
        composeTestRule.onNodeWithText(str(R.string.shares_standing_share_button))
            .performScrollTo()
            .assertIsEnabled()
            .performClick()

        coVerify(exactly = 1) { grants.create(match { it.entityId == "u-viewed" }) }
        coVerify(exactly = 0) { repo.create(any()) }
        composeTestRule.waitForIdle()
        assertTrue(landed)
    }

    @Test
    fun `a standing share failure is worded from its reason`() {
        val repo = mockk<ContactShareRepository>()
        val grants = mockk<ShareGrantRepository>()
        coEvery { grants.previewContact(any(), any()) } returns Result.success(emptyMap())
        coEvery { repo.userDirectory() } returns Result.success(
            listOf(UserDirectoryEntry(id = 1, username = "alice"))
        )
        coEvery { grants.create(any()) } returns Result.failure(
            ApiError.Client(
                409,
                "RAW",
                reason = com.mycorrhizal.crm.model.network.ErrorReason(
                    com.mycorrhizal.crm.model.network.ErrorReasonCode.SHARE_GRANT_ALREADY_EXISTS,
                    emptyMap()
                )
            )
        )
        val vm = standingScreen(repo, grants)
        vm.selectRecipient(1)

        composeTestRule.onNodeWithText(str(R.string.shares_mode_standing)).performClick()
        composeTestRule.onNodeWithText(str(R.string.shares_standing_share_button))
            .performScrollTo()
            .performClick()

        composeTestRule.onNodeWithText(
            "That is already shared with this person"
        ).assertIsDisplayed()
        composeTestRule.onNodeWithText("RAW").assertDoesNotExist()
    }

    // --- Issue #2519: standing notices and the "What they will see" preview ---

    private val fullCard: Map<String, Any?> = mapOf(
        "name" to mapOf("full" to "Dana White"),
        "emails" to mapOf("e1" to mapOf("address" to "dana@example.com")),
        "phones" to mapOf("p1" to mapOf("number" to "+1 555 0100")),
        "personalInfo" to mapOf("i1" to mapOf("kind" to "diet", "value" to "vegan"))
    )

    /** Plays the server: the card carries only the requested sections. */
    private fun cardFor(sections: List<String>): Map<String, Any?> =
        fullCard.filterKeys { it == "name" || it == "personalInfo" || it in sections }

    private fun previewScreen(
        grants: ShareGrantRepository = mockk<ShareGrantRepository>().also {
            coEvery { it.previewContact(any(), any()) } answers {
                @Suppress("UNCHECKED_CAST")
                Result.success(cardFor(secondArg<List<String>>()))
            }
        }
    ): ShareContactViewModel {
        val repo = mockk<ContactShareRepository>()
        coEvery { repo.userDirectory() } returns Result.success(
            listOf(UserDirectoryEntry(id = 1, username = "alice"))
        )
        return standingScreen(repo, grants)
    }

    private fun chooseStanding() {
        composeTestRule.onNodeWithText(str(R.string.shares_mode_standing)).performClick()
        composeTestRule.waitForIdle()
    }

    private fun checkbox(label: String) = composeTestRule.onNode(hasText(label) and isToggleable())

    private val noticeRes = listOf(
        R.string.shares_standing_live_notice,
        R.string.shares_standing_no_recall_notice,
        R.string.shares_standing_private_notice
    )

    @Test
    fun `the notices and preview are absent in one-time mode`() {
        previewScreen()

        noticeRes.forEach { composeTestRule.onNodeWithText(str(it)).assertDoesNotExist() }
        composeTestRule.onNodeWithText(str(R.string.shares_standing_preview_title))
            .assertDoesNotExist()
    }

    @Test
    fun `choosing keep sharing shows the live, no-recall and private notices`() {
        previewScreen()
        chooseStanding()

        noticeRes.forEach {
            composeTestRule.onNodeWithText(str(it)).performScrollTo().assertIsDisplayed()
        }
    }

    @Test
    fun `going back to one-time hides the notices and the preview again`() {
        previewScreen()
        chooseStanding()
        composeTestRule.onNodeWithText(str(R.string.shares_mode_one_time)).performClick()
        composeTestRule.waitForIdle()

        noticeRes.forEach { composeTestRule.onNodeWithText(str(it)).assertDoesNotExist() }
        composeTestRule.onNodeWithText(str(R.string.shares_standing_preview_title))
            .assertDoesNotExist()
    }

    @Test
    fun `the preview shows what the selected sections carry and nothing else`() {
        previewScreen()
        chooseStanding()

        composeTestRule.onNodeWithText(str(R.string.shares_standing_preview_title))
            .performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText(str(R.string.shares_standing_preview_hint))
            .performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Dana White").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("dana@example.com").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("+1 555 0100").performScrollTo().assertIsDisplayed()
        // personal_info is sensitivity-gated and not selected: even if a payload carried it, it is
        // not rendered.
        composeTestRule.onNodeWithText("diet: vegan").assertDoesNotExist()
    }

    @Test
    fun `toggling a section updates the preview`() {
        previewScreen()
        chooseStanding()
        composeTestRule.onNodeWithText("+1 555 0100").performScrollTo().assertIsDisplayed()

        checkbox(str(R.string.shares_section_phones)).performScrollTo().performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("+1 555 0100").assertDoesNotExist()
        composeTestRule.onNodeWithText("dana@example.com").performScrollTo().assertIsDisplayed()

        checkbox(str(R.string.shares_section_phones)).performScrollTo().performClick()
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithText("+1 555 0100").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `with no section selected the empty state shows and sharing is blocked`() {
        val vm = previewScreen()
        vm.selectRecipient(1)
        chooseStanding()
        composeTestRule.runOnIdle {
            ShareFieldSections.DEFAULT_SELECTED.forEach { vm.toggleSection(it, false) }
        }
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText(str(R.string.shares_standing_preview_empty))
            .performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("dana@example.com").assertDoesNotExist()
        composeTestRule.onNodeWithText(str(R.string.shares_standing_share_button))
            .performScrollTo().assertIsNotEnabled()
    }

    @Test
    fun `a card with no visible items shows the name and the empty state`() {
        val grants = mockk<ShareGrantRepository>()
        coEvery { grants.previewContact(any(), any()) } returns
            Result.success(mapOf("name" to mapOf("full" to "Dana White")))
        previewScreen(grants)
        chooseStanding()

        composeTestRule.onNodeWithText("Dana White").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText(str(R.string.shares_standing_preview_empty))
            .performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `the preview shows a loading state until the server answers`() {
        val grants = mockk<ShareGrantRepository>()
        val pending = CompletableDeferred<Result<Map<String, Any?>>>()
        coEvery { grants.previewContact(any(), any()) } coAnswers { pending.await() }
        previewScreen(grants)
        chooseStanding()

        composeTestRule.onNodeWithText(str(R.string.shares_standing_preview_loading))
            .performScrollTo().assertIsDisplayed()

        pending.complete(Result.success(fullCard))
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithText(str(R.string.shares_standing_preview_loading))
            .assertDoesNotExist()
        composeTestRule.onNodeWithText("dana@example.com").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `a failed preview shows its error and does not block sharing`() {
        val grants = mockk<ShareGrantRepository>()
        coEvery { grants.previewContact(any(), any()) } returns
            Result.failure(ApiError.Client(404, "gone"))
        val vm = previewScreen(grants)
        vm.selectRecipient(1)
        chooseStanding()

        composeTestRule.onNodeWithText(str(R.string.shares_standing_preview_error))
            .performScrollTo().assertIsDisplayed()
        // The detail line is the localized reason, never the raw server text.
        composeTestRule.onNodeWithText("Not found").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("gone").assertDoesNotExist()
        composeTestRule.onNodeWithText(str(R.string.shares_standing_share_button))
            .performScrollTo().assertIsEnabled()
    }
}
