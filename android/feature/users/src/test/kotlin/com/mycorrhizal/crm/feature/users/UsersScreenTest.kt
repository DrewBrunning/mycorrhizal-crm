package com.mycorrhizal.crm.feature.users

import androidx.compose.ui.test.junit4.createComposeRule
import com.mycorrhizal.crm.domain.repository.UserManagementRepository
import com.mycorrhizal.crm.model.network.AdminUser
import com.mycorrhizal.crm.model.network.AdminUsersListResponse
import com.mycorrhizal.crm.testing.a11y.assertAccessibleSemantics
import com.mycorrhizal.crm.testing.a11y.assertNoDuplicateContentDescriptions
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.mockk
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
}
