package com.mycorrhizal.crm.feature.users

import android.content.Context
import androidx.annotation.StringRes
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.domain.repository.UserManagementRepository
import com.mycorrhizal.crm.model.network.AdminUser
import com.mycorrhizal.crm.model.network.AdminUsersListResponse
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.a11y.assertAccessibleSemantics
import com.mycorrhizal.crm.testing.a11y.assertNoDuplicateContentDescriptions
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.awaitCancellation
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issue #214/#348: mounts the real top-level [UsersScreen] (Scaffold +
 * TopAppBar + FAB included) against a [UsersViewModel] backed by a mocked
 * [UserManagementRepository] and sweeps it for static a11y invariants — the
 * same construction the other feature modules' screen tests use.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class UsersScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun str(@StringRes res: Int, vararg args: Any): String =
        ApplicationProvider.getApplicationContext<Context>().getString(res, *args)

    private fun setScreen(darkTheme: Boolean) {
        val repository = mockk<UserManagementRepository>()
        coEvery { repository.list(any(), any()) } returns Result.success(
            AdminUsersListResponse(
                users = listOf(
                    AdminUser(id = 1, username = "alice", email = "alice@example.com", isAdmin = true),
                    AdminUser(id = 2, username = "bob", email = "bob@example.com"),
                ),
                total = 2,
            ),
        )
        val viewModel = UsersViewModel(repository)

        composeTestRule.setContent {
            MycorrhizalTheme(darkTheme = darkTheme) {
                UsersScreen(onBack = {}, viewModel = viewModel)
            }
        }
    }

    @Test
    fun `users screen has no accessibility violations (light)`() {
        setScreen(darkTheme = false)

        composeTestRule.assertAccessibleSemantics()
    }

    @Test
    fun `users screen has no accessibility violations (dark)`() {
        setScreen(darkTheme = true)

        composeTestRule.assertAccessibleSemantics()
    }

    @Test
    fun `row action labels are unique per row`() {
        // #205: two seeded users must not both announce a bare
        // "Edit"/"Delete" — each row's actions carry the user's name.
        setScreen(darkTheme = false)

        composeTestRule.assertNoDuplicateContentDescriptions()
    }

    // --- Top-level screen loading/empty/edit branches ----------------------

    private fun setScreen(viewModel: UsersViewModel) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                UsersScreen(onBack = {}, viewModel = viewModel)
            }
        }
        composeTestRule.waitForIdle()
    }

    @Test
    fun `a spinner renders while the initial load is in flight`() {
        val repository = mockk<UserManagementRepository>()
        coEvery { repository.list(any(), any()) } coAnswers { awaitCancellation() }

        setScreen(UsersViewModel(repository))

        composeTestRule
            .onNode(SemanticsMatcher.keyIsDefined(SemanticsProperties.ProgressBarRangeInfo))
            .assertExists()
    }

    @Test
    fun `an empty list renders the empty state`() {
        val repository = mockk<UserManagementRepository>()
        coEvery { repository.list(any(), any()) } returns Result.success(
            AdminUsersListResponse(users = emptyList(), total = 0),
        )

        setScreen(UsersViewModel(repository))

        composeTestRule.onNodeWithText(str(R.string.users_empty)).assertIsDisplayed()
    }

    @Test
    fun `tapping a row's edit action opens the editor for that user`() {
        val repository = mockk<UserManagementRepository>()
        coEvery { repository.list(any(), any()) } returns Result.success(
            AdminUsersListResponse(
                users = listOf(AdminUser(id = 1, username = "alice", email = "alice@example.com", isAdmin = true)),
                total = 1,
            ),
        )

        setScreen(UsersViewModel(repository))

        composeTestRule.onNodeWithContentDescription(str(R.string.users_edit_named, "alice")).performClick()

        composeTestRule.onNodeWithText(str(R.string.users_edit)).assertIsDisplayed()
        composeTestRule.onNodeWithText(str(R.string.users_password_hint)).assertIsDisplayed()
    }

    // --- Admin 2FA reset (issue #596) ---------------------------------------

    private val alice = AdminUser(id = 1, username = "alice", email = "alice@example.com", isAdmin = true)
    private val bob = AdminUser(id = 2, username = "bob", email = "bob@example.com")

    private fun repoWithTwoUsers(): UserManagementRepository {
        val repository = mockk<UserManagementRepository>()
        coEvery { repository.list(any(), any()) } returns Result.success(
            AdminUsersListResponse(users = listOf(alice, bob), total = 2),
        )
        return repository
    }

    @Test
    fun `every row has its own reset 2FA button`() {
        setScreen(UsersViewModel(repoWithTwoUsers()))

        composeTestRule.onNodeWithContentDescription(str(R.string.users_reset_2fa_named, "alice")).assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription(str(R.string.users_reset_2fa_named, "bob")).assertIsDisplayed()
    }

    @Test
    fun `tapping reset 2FA opens the confirmation dialog without calling the API`() {
        val repository = repoWithTwoUsers()
        setScreen(UsersViewModel(repository))

        composeTestRule.onNodeWithContentDescription(str(R.string.users_reset_2fa_named, "bob")).performClick()

        composeTestRule.onNodeWithText(str(R.string.users_reset_2fa_title)).assertIsDisplayed()
        composeTestRule.onNodeWithText(str(R.string.users_reset_2fa_body, "bob")).assertIsDisplayed()
        composeTestRule.onNodeWithText(str(R.string.users_reset_2fa_warning)).assertIsDisplayed()
        coVerify(exactly = 0) { repository.resetTwoFactor(any()) }
    }

    @Test
    fun `cancelling the reset dialog closes it and makes no call`() {
        val repository = repoWithTwoUsers()
        setScreen(UsersViewModel(repository))

        composeTestRule.onNodeWithContentDescription(str(R.string.users_reset_2fa_named, "bob")).performClick()
        composeTestRule.onNodeWithText(str(R.string.action_cancel)).performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText(str(R.string.users_reset_2fa_title)).assertDoesNotExist()
        coVerify(exactly = 0) { repository.resetTwoFactor(any()) }
    }

    @Test
    fun `confirming resets the tapped row's user, closes the dialog and shows feedback`() {
        val repository = repoWithTwoUsers()
        coEvery { repository.resetTwoFactor(2) } returns Result.success(bob)
        setScreen(UsersViewModel(repository))

        composeTestRule.onNodeWithContentDescription(str(R.string.users_reset_2fa_named, "bob")).performClick()
        composeTestRule.onNodeWithText(str(R.string.users_reset_2fa_confirm)).performClick()
        composeTestRule.waitForIdle()

        coVerify(exactly = 1) { repository.resetTwoFactor(2) }
        coVerify(exactly = 0) { repository.resetTwoFactor(1) }
        composeTestRule.onNodeWithText(str(R.string.users_reset_2fa_title)).assertDoesNotExist()
        composeTestRule.onNodeWithText(str(R.string.users_reset_2fa_done)).assertIsDisplayed()
    }

    @Test
    fun `a failed reset keeps the dialog open with the server error and can be retried`() {
        val repository = repoWithTwoUsers()
        coEvery { repository.resetTwoFactor(1) } returns Result.failure(ApiError.Client(404, "User not found"))
        setScreen(UsersViewModel(repository))

        composeTestRule.onNodeWithContentDescription(str(R.string.users_reset_2fa_named, "alice")).performClick()
        composeTestRule.onNodeWithText(str(R.string.users_reset_2fa_confirm)).performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText(str(R.string.users_reset_2fa_title)).assertIsDisplayed()
        composeTestRule.onNodeWithText("User not found").assertIsDisplayed()

        coEvery { repository.resetTwoFactor(1) } returns Result.success(alice)
        composeTestRule.onNodeWithText(str(R.string.users_reset_2fa_confirm)).performClick()
        composeTestRule.waitForIdle()

        coVerify(exactly = 2) { repository.resetTwoFactor(1) }
        composeTestRule.onNodeWithText(str(R.string.users_reset_2fa_title)).assertDoesNotExist()
        composeTestRule.onNodeWithText(str(R.string.users_reset_2fa_done)).assertIsDisplayed()
    }

    @Test
    fun `the confirm button is disabled and the dialog cannot be cancelled while a reset is in flight`() {
        val repository = repoWithTwoUsers()
        coEvery { repository.resetTwoFactor(2) } coAnswers { awaitCancellation() }
        setScreen(UsersViewModel(repository))

        composeTestRule.onNodeWithContentDescription(str(R.string.users_reset_2fa_named, "bob")).performClick()
        composeTestRule.onNodeWithText(str(R.string.users_reset_2fa_confirm)).performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText(str(R.string.users_reset_2fa_confirm)).assertIsNotEnabled()
        composeTestRule.onNodeWithText(str(R.string.action_cancel)).assertIsNotEnabled()
        coVerify(exactly = 1) { repository.resetTwoFactor(2) }
    }

    @Test
    fun `cancelling after a failed reset clears the stale error and makes no further call`() {
        val repository = repoWithTwoUsers()
        coEvery { repository.resetTwoFactor(2) } returns Result.failure(ApiError.Client(404, "User not found"))
        setScreen(UsersViewModel(repository))

        composeTestRule.onNodeWithContentDescription(str(R.string.users_reset_2fa_named, "bob")).performClick()
        composeTestRule.onNodeWithText(str(R.string.users_reset_2fa_confirm)).performClick()
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithText("User not found").assertIsDisplayed()

        composeTestRule.onNodeWithText(str(R.string.action_cancel)).performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText(str(R.string.users_reset_2fa_title)).assertDoesNotExist()
        coVerify(exactly = 1) { repository.resetTwoFactor(2) }

        // Re-opening starts clean: no stale error, and still no extra call until confirmed.
        composeTestRule.onNodeWithContentDescription(str(R.string.users_reset_2fa_named, "bob")).performClick()
        composeTestRule.onNodeWithText("User not found").assertDoesNotExist()
        coVerify(exactly = 1) { repository.resetTwoFactor(2) }
    }
}
