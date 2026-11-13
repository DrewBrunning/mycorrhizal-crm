package com.mycorrhizal.crm.feature.shares

import androidx.annotation.StringRes
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.ContactShareRepository
import com.mycorrhizal.crm.domain.repository.ShareGrantRepository
import com.mycorrhizal.crm.model.AppVersion
import com.mycorrhizal.crm.model.network.ContactSharesPage
import com.mycorrhizal.crm.model.network.UserDirectoryEntry
import com.mycorrhizal.crm.ui.LocalServerVersion
import com.mycorrhizal.crm.ui.R
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
 * Issue #2527: the standing-share entry points inside the shares feature are hidden when the
 * connected server predates `/share-grants*` (floor v1.7.0, ServerFeature.SHARE_GRANTS). A null
 * version fails open; the one-time contact-share flow (a v1.0.0 baseline capability) is never
 * touched.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ShareGrantsFloorTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val v160 = AppVersion(1, 6, 0)
    private val v170 = AppVersion(1, 7, 0)

    private fun str(@StringRes res: Int): String =
        androidx.test.core.app.ApplicationProvider.getApplicationContext<android.content.Context>()
            .getString(res)

    private fun contactShares(version: AppVersion?) {
        val repository = mockk<ContactShareRepository>()
        val empty = Result.success(ContactSharesPage())
        coEvery { repository.listIncoming(any(), any()) } returns empty
        coEvery { repository.listOutgoing(any(), any()) } returns empty
        composeTestRule.setContent {
            CompositionLocalProvider(LocalServerVersion provides version) {
                MycorrhizalTheme {
                    ContactSharesScreen(
                        onMenuClick = {},
                        onOpenSharedWithMe = {},
                        onOpenSharedByMe = {},
                        viewModel = ContactSharesViewModel(repository)
                    )
                }
            }
        }
    }

    private fun shareContact(version: AppVersion?) {
        val repo = mockk<ContactShareRepository>()
        coEvery { repo.userDirectory() } returns Result.success(
            listOf(UserDirectoryEntry(id = 1, username = "alice"))
        )
        val vm = ShareContactViewModel(
            repo,
            mockk<ShareGrantRepository>(),
            SavedStateHandle(mapOf("uid" to "u-viewed"))
        )
        composeTestRule.setContent {
            CompositionLocalProvider(LocalServerVersion provides version) {
                MycorrhizalTheme { ShareContactScreen(onBack = {}, viewModel = vm) }
            }
        }
    }

    @Test
    fun `contact shares hides the Shared with me and by me links on a v1_6_0 server`() {
        contactShares(v160)
        composeTestRule.onNodeWithText(str(R.string.shares_open_shared_with_me))
            .assertDoesNotExist()
        composeTestRule.onNodeWithText(str(R.string.shares_open_shared_by_me)).assertDoesNotExist()
        // The one-time shares tabs are a baseline capability and stay.
        composeTestRule.onNodeWithText(str(R.string.shares_tab_incoming)).assertExists()
    }

    @Test
    fun `contact shares shows the standing-share links on a v1_7_0 server`() {
        contactShares(v170)
        composeTestRule.onNodeWithText(str(R.string.shares_open_shared_with_me)).assertExists()
        composeTestRule.onNodeWithText(str(R.string.shares_open_shared_by_me)).assertExists()
    }

    @Test
    fun `contact shares shows the standing-share links when the server version is unknown`() {
        contactShares(null)
        composeTestRule.onNodeWithText(str(R.string.shares_open_shared_with_me)).assertExists()
    }

    @Test
    fun `share contact offers no Keep sharing mode on a v1_6_0 server`() {
        shareContact(v160)
        composeTestRule.onNodeWithText(str(R.string.shares_mode_standing)).assertDoesNotExist()
        composeTestRule.onNodeWithText(str(R.string.shares_mode_label)).assertDoesNotExist()
        // The one-time flow is untouched.
        composeTestRule.onNodeWithText(str(R.string.shares_share_dialog_description)).assertExists()
    }

    @Test
    fun `share contact offers Keep sharing on a v1_7_0 server`() {
        shareContact(v170)
        composeTestRule.onNodeWithText(str(R.string.shares_mode_standing)).assertExists()
    }

    @Test
    fun `share contact offers Keep sharing when the server version is unknown`() {
        shareContact(null)
        composeTestRule.onNodeWithText(str(R.string.shares_mode_standing)).assertExists()
    }
}
