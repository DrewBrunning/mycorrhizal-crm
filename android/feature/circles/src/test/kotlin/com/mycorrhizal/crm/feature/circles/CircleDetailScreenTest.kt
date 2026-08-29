package com.mycorrhizal.crm.feature.circles

import android.content.Context
import androidx.annotation.StringRes
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.lifecycle.SavedStateHandle
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.domain.repository.CircleDetail
import com.mycorrhizal.crm.domain.repository.CircleRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.CircleMember
import com.mycorrhizal.crm.model.network.ContactSummary
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
 * Issue #682: the circle detail screen — member list, the resolved-name /
 * group-SMS actions, and the add-member dialog — had no test coverage.
 * Mounts the real screen against a [CircleDetailViewModel] backed by mocked
 * repositories.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class CircleDetailScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun str(@StringRes res: Int, vararg args: Any): String =
        ApplicationProvider.getApplicationContext<Context>().getString(res, *args)

    private fun setScreen(
        members: List<CircleMember> = listOf(
            CircleMember(id = 1, circleId = "c1", memberVCardUid = "uid-alice"),
        ),
        resolve: Map<String, ContactSummary> = emptyMap(),
    ) {
        val circleRepository = mockk<CircleRepository>()
        val contactRepository = mockk<ContactRepository>()
        coEvery { circleRepository.getWithMembers("c1") } returns Result.success(
            CircleDetail(circle = Circle(id = "c1", name = "Family"), members = members),
        )
        coEvery { contactRepository.resolveByUid(any()) } returns Result.success(resolve)
        val vm = CircleDetailViewModel(circleRepository, contactRepository, SavedStateHandle(mapOf("circleId" to "c1")))
        composeTestRule.setContent {
            MycorrhizalTheme(darkTheme = false) {
                CircleDetailScreen(onBack = {}, viewModel = vm)
            }
        }
    }

    @Test
    fun `circle detail renders the circle name and its members`() {
        setScreen()

        composeTestRule.onNodeWithText("Family").assertIsDisplayed()
        composeTestRule.onNodeWithText("uid-alice").assertIsDisplayed()
    }

    @Test
    fun `circle detail has no accessibility violations`() {
        setScreen()

        composeTestRule.assertAccessibleSemantics()
    }

    @Test
    fun `an empty circle shows the empty state`() {
        setScreen(members = emptyList())

        composeTestRule.onNodeWithText(str(R.string.circles_members_empty)).assertIsDisplayed()
    }

    @Test
    fun `removing a member calls the repository`() {
        val circleRepository = mockk<CircleRepository>()
        val contactRepository = mockk<ContactRepository>()
        coEvery { circleRepository.getWithMembers("c1") } returns Result.success(
            CircleDetail(
                circle = Circle(id = "c1", name = "Family"),
                members = listOf(CircleMember(id = 1, circleId = "c1", memberVCardUid = "uid-alice")),
            ),
        )
        coEvery { contactRepository.resolveByUid(any()) } returns Result.success(emptyMap())
        coEvery { circleRepository.removeMember("c1", "uid-alice") } returns Result.success(Unit)
        val vm = CircleDetailViewModel(circleRepository, contactRepository, SavedStateHandle(mapOf("circleId" to "c1")))
        composeTestRule.setContent {
            MycorrhizalTheme { CircleDetailScreen(onBack = {}, viewModel = vm) }
        }

        composeTestRule.onNodeWithContentDescription(str(R.string.circles_remove_member_named, "uid-alice"))
            .performClick()

        coVerify(exactly = 1) { circleRepository.removeMember("c1", "uid-alice") }
        composeTestRule.onNodeWithText("uid-alice").assertDoesNotExist()
    }

    @Test
    fun `the add-member dialog confirms a typed uid`() {
        val circleRepository = mockk<CircleRepository>()
        val contactRepository = mockk<ContactRepository>()
        coEvery { circleRepository.getWithMembers("c1") } returns Result.success(
            CircleDetail(circle = Circle(id = "c1", name = "Family"), members = emptyList()),
        )
        coEvery { contactRepository.resolveByUid(any()) } returns Result.success(emptyMap())
        coEvery { circleRepository.addMember("c1", "uid-new") } returns Result.success(
            CircleMember(id = 2, circleId = "c1", memberVCardUid = "uid-new"),
        )
        val vm = CircleDetailViewModel(circleRepository, contactRepository, SavedStateHandle(mapOf("circleId" to "c1")))
        composeTestRule.setContent {
            MycorrhizalTheme { CircleDetailScreen(onBack = {}, viewModel = vm) }
        }

        composeTestRule.onNodeWithContentDescription(str(R.string.circles_add_member)).performClick()

        composeTestRule.onNodeWithText(str(R.string.action_add)).assertIsNotEnabled()
        composeTestRule.onNodeWithText(str(R.string.circles_member_vcard_uid)).performTextInput("uid-new")
        composeTestRule.onNodeWithText(str(R.string.action_add)).assertIsEnabled().performClick()

        coVerify(exactly = 1) { circleRepository.addMember("c1", "uid-new") }
        composeTestRule.onNodeWithText("uid-new").assertIsDisplayed()
    }
}
