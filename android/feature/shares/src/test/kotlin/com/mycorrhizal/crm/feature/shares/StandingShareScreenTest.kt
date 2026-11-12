package com.mycorrhizal.crm.feature.shares

import android.content.Context
import androidx.annotation.StringRes
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.lifecycle.SavedStateHandle
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.domain.repository.ContactShareRepository
import com.mycorrhizal.crm.domain.repository.ShareGrantRepository
import com.mycorrhizal.crm.model.network.OutgoingShareGrant
import com.mycorrhizal.crm.model.network.ShareGrantEntityTypes
import com.mycorrhizal.crm.model.network.ShareGrantInput
import com.mycorrhizal.crm.model.network.UserDirectoryEntry
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.a11y.assertAccessibleSemantics
import com.mycorrhizal.crm.ui.R
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

/** Issue #2518: the standing-share screen for a note, an activity or a circle. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class StandingShareScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val directory = mockk<ContactShareRepository>()
    private val grants = mockk<ShareGrantRepository>()

    private fun str(@StringRes res: Int): String =
        ApplicationProvider.getApplicationContext<Context>().getString(res)

    private fun mount(
        type: String,
        recipients: List<UserDirectoryEntry> = listOf(
            UserDirectoryEntry(id = 1, username = "alice")
        ),
        onShared: () -> Unit = {}
    ): StandingShareViewModel {
        coEvery { directory.userDirectory() } returns Result.success(recipients)
        val vm = StandingShareViewModel(
            directory,
            grants,
            SavedStateHandle(mapOf("entityType" to type, "entityId" to "e-1"))
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                StandingShareScreen(onBack = {}, onShared = onShared, viewModel = vm)
            }
        }
        return vm
    }

    private val perType = listOf(
        Triple(
            ShareGrantEntityTypes.NOTE,
            R.string.shares_standing_title_note,
            R.string.shares_standing_notice_note
        ),
        Triple(
            ShareGrantEntityTypes.ACTIVITY,
            R.string.shares_standing_title_activity,
            R.string.shares_standing_notice_activity
        ),
        Triple(
            ShareGrantEntityTypes.CIRCLE,
            R.string.shares_standing_title_circle,
            R.string.shares_standing_notice_circle
        )
    )

    private fun assertTypeShown(index: Int) {
        val (type, title, notice) = perType[index]
        val vm = mount(type)
        composeTestRule.onNodeWithText(str(title)).assertIsDisplayed()
        composeTestRule.onNodeWithText(str(notice)).assertIsDisplayed()
        composeTestRule.onNodeWithText(str(R.string.shares_standing_pending_note))
            .assertIsDisplayed()
        composeTestRule.onNodeWithText(str(R.string.shares_recipient)).assertIsDisplayed()
        composeTestRule.onNodeWithText(str(R.string.shares_standing_expiry_label))
            .assertIsDisplayed()
        // Sections only exist for a contact grant.
        composeTestRule.onNodeWithText(str(R.string.shares_fields_label)).assertDoesNotExist()
        assertEquals(type, vm.uiState.value.entityType)
    }

    @Test
    fun `a note shows its title, notice and the pending note`() = assertTypeShown(0)

    @Test
    fun `an activity shows its title, notice and the pending note`() = assertTypeShown(1)

    @Test
    fun `a circle shows its title, notice and the pending note`() = assertTypeShown(2)

    @Test
    fun `the share button stays disabled until a recipient is picked`() {
        mount(ShareGrantEntityTypes.NOTE)
        composeTestRule.onNodeWithText(str(R.string.shares_standing_share_button))
            .performScrollTo()
            .assertIsNotEnabled()
    }

    @Test
    fun `sharing creates the grant and reports success`() {
        coEvery { grants.create(any()) } returns Result.success(OutgoingShareGrant("g1"))
        var shared = 0
        val vm = mount(ShareGrantEntityTypes.CIRCLE, onShared = { shared++ })
        vm.selectRecipient(1)
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText(str(R.string.shares_standing_share_button))
            .performScrollTo()
            .assertIsEnabled()
            .performClick()
        composeTestRule.waitForIdle()

        coVerify(exactly = 1) {
            grants.create(
                ShareGrantInput(1, ShareGrantEntityTypes.CIRCLE, "e-1", emptyList(), null)
            )
        }
        assertEquals(1, shared)
    }

    @Test
    fun `a failed create shows the localized error and does not navigate`() {
        coEvery { grants.create(any()) } returns Result.failure(ApiError.Client(404, "gone"))
        var shared = 0
        val vm = mount(ShareGrantEntityTypes.ACTIVITY, onShared = { shared++ })
        vm.selectRecipient(1)
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText(str(R.string.shares_standing_share_button))
            .performScrollTo()
            .performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Not found").assertIsDisplayed()
        assertEquals(0, shared)
    }

    @Test
    fun `an empty directory explains there is nobody to share with`() {
        mount(ShareGrantEntityTypes.NOTE, recipients = emptyList())
        composeTestRule.onNodeWithText(str(R.string.shares_no_recipients)).assertIsDisplayed()
    }

    @Test
    fun `the screen has no accessibility violations`() {
        mount(ShareGrantEntityTypes.NOTE)
        composeTestRule.assertAccessibleSemantics()
    }
}
