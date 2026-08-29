package com.mycorrhizal.crm.feature.shares

import android.content.Context
import androidx.annotation.StringRes
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.lifecycle.SavedStateHandle
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.domain.repository.ContactShareRepository
import com.mycorrhizal.crm.model.network.UserDirectoryEntry
import com.mycorrhizal.crm.testing.a11y.assertAccessibleSemantics
import com.mycorrhizal.crm.ui.R
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
        ShareContactViewModel(repo, SavedStateHandle(mapOf("uid" to "u-viewed")))

    private fun screen(repo: ContactShareRepository, vm: ShareContactViewModel = viewModel(repo)) {
        composeTestRule.setContent {
            MycorrhizalTheme { ShareContactScreen(onBack = {}, viewModel = vm) }
        }
    }

    @Test
    fun `renders the recipient picker and sections once the directory loads`() {
        val repo = mockk<ContactShareRepository>()
        coEvery { repo.userDirectory() } returns Result.success(
            listOf(UserDirectoryEntry(id = 1, username = "alice")),
        )
        screen(repo)

        composeTestRule.onNodeWithText(str(R.string.shares_share_dialog_title)).assertIsDisplayed()
        composeTestRule.onNodeWithText(str(R.string.shares_recipient)).assertIsDisplayed()
        composeTestRule.onNodeWithText(str(R.string.shares_fields_label)).assertIsDisplayed()
        composeTestRule.onNodeWithText(str(R.string.shares_share_button)).performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `share is disabled until a recipient is selected`() {
        val repo = mockk<ContactShareRepository>()
        coEvery { repo.userDirectory() } returns Result.success(
            listOf(UserDirectoryEntry(id = 1, username = "alice")),
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
            listOf(UserDirectoryEntry(id = 1, username = "alice")),
        )
        screen(repo)

        composeTestRule.assertAccessibleSemantics()
    }

    @Test
    fun `the reveal button opens the sensitive confirmation dialog`() {
        val repo = mockk<ContactShareRepository>()
        coEvery { repo.userDirectory() } returns Result.success(
            listOf(UserDirectoryEntry(id = 1, username = "alice")),
        )
        screen(repo)

        composeTestRule.onNodeWithText(str(R.string.shares_reveal_button)).performScrollTo().performClick()

        // The confirm gate is reachable only after the deliberate reveal step.
        composeTestRule.onNodeWithText(str(R.string.shares_reveal_title)).assertIsDisplayed()
    }

    @Test
    fun `sharing with a selected recipient calls the repository`() {
        val repo = mockk<ContactShareRepository>()
        coEvery { repo.userDirectory() } returns Result.success(
            listOf(UserDirectoryEntry(id = 1, username = "alice")),
        )
        coEvery { repo.create(any()) } returns Result.success(
            com.mycorrhizal.crm.model.network.ContactShare(id = "share-1"),
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
}
