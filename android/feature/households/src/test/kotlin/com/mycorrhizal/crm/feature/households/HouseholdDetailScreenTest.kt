package com.mycorrhizal.crm.feature.households

import android.content.Context
import androidx.annotation.StringRes
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.lifecycle.SavedStateHandle
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.HouseholdDetail
import com.mycorrhizal.crm.domain.repository.HouseholdRepository
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.Household
import com.mycorrhizal.crm.model.network.HouseholdMember
import com.mycorrhizal.crm.model.network.HouseholdTypes
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
 * Issue #682: the household detail screen — member rows with resolved names,
 * the role picker, the relationship-suggestion action, and the add-member
 * dialog — had no test coverage. Mounts the real screen against a
 * [HouseholdDetailViewModel] backed by mocked repositories.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class HouseholdDetailScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun str(@StringRes res: Int, vararg args: Any): String =
        ApplicationProvider.getApplicationContext<Context>().getString(res, *args)

    @Test
    fun `household detail renders the household name and resolved member names`() {
        val householdRepository = mockk<HouseholdRepository>()
        val contactRepository = mockk<ContactRepository>()
        coEvery { householdRepository.getWithMembers("h1") } returns Result.success(
            HouseholdDetail(
                household = Household(id = "h1", name = "Home", type = HouseholdTypes.FAMILY_UNIT),
                members = listOf(
                    HouseholdMember(id = 1, householdId = "h1", memberVCardUid = "uid-1", role = "adult"),
                ),
            ),
        )
        coEvery { contactRepository.resolveByUid(listOf("uid-1")) } returns Result.success(
            mapOf("uid-1" to ContactSummary(id = 7, uid = "uid-1", firstname = "Alice")),
        )
        val vm = HouseholdDetailViewModel(
            householdRepository,
            contactRepository,
            SavedStateHandle(mapOf("householdId" to "h1")),
        )
        composeTestRule.setContent {
            MycorrhizalTheme { HouseholdDetailScreen(onBack = {}, onNavigateToContact = {}, viewModel = vm) }
        }

        composeTestRule.onNodeWithText("Home").assertIsDisplayed()
        composeTestRule.onNodeWithText("Alice").assertIsDisplayed()
    }

    @Test
    fun `an unresolvable member falls back to the unknown label`() {
        val householdRepository = mockk<HouseholdRepository>()
        val contactRepository = mockk<ContactRepository>()
        coEvery { householdRepository.getWithMembers("h1") } returns Result.success(
            HouseholdDetail(
                household = Household(id = "h1", name = "Home", type = HouseholdTypes.FAMILY_UNIT),
                members = listOf(HouseholdMember(id = 1, householdId = "h1", memberVCardUid = "uid-1")),
            ),
        )
        coEvery { contactRepository.resolveByUid(any()) } returns Result.success(emptyMap())
        val vm = HouseholdDetailViewModel(
            householdRepository,
            contactRepository,
            SavedStateHandle(mapOf("householdId" to "h1")),
        )
        composeTestRule.setContent {
            MycorrhizalTheme { HouseholdDetailScreen(onBack = {}, onNavigateToContact = {}, viewModel = vm) }
        }

        composeTestRule.onNodeWithText(str(R.string.households_member_unknown))
            .assertIsDisplayed()
    }

    @Test
    fun `household detail has no accessibility violations`() {
        val householdRepository = mockk<HouseholdRepository>()
        val contactRepository = mockk<ContactRepository>()
        coEvery { householdRepository.getWithMembers("h1") } returns Result.success(
            HouseholdDetail(
                household = Household(id = "h1", name = "Home", type = HouseholdTypes.FAMILY_UNIT),
                members = listOf(HouseholdMember(id = 1, householdId = "h1", memberVCardUid = "uid-1")),
            ),
        )
        coEvery { contactRepository.resolveByUid(any()) } returns Result.success(emptyMap())
        val vm = HouseholdDetailViewModel(
            householdRepository,
            contactRepository,
            SavedStateHandle(mapOf("householdId" to "h1")),
        )
        composeTestRule.setContent {
            MycorrhizalTheme { HouseholdDetailScreen(onBack = {}, onNavigateToContact = {}, viewModel = vm) }
        }

        composeTestRule.assertAccessibleSemantics()
    }

    @Test
    fun `suggest relationships is enabled only with two members`() {
        val householdRepository = mockk<HouseholdRepository>()
        val contactRepository = mockk<ContactRepository>()
        coEvery { householdRepository.getWithMembers("h1") } returns Result.success(
            HouseholdDetail(
                household = Household(id = "h1", name = "Home", type = HouseholdTypes.FAMILY_UNIT),
                members = emptyList(),
            ),
        )
        coEvery { contactRepository.resolveByUid(any()) } returns Result.success(emptyMap())
        val vm = HouseholdDetailViewModel(
            householdRepository,
            contactRepository,
            SavedStateHandle(mapOf("householdId" to "h1")),
        )
        composeTestRule.setContent {
            MycorrhizalTheme { HouseholdDetailScreen(onBack = {}, onNavigateToContact = {}, viewModel = vm) }
        }

        // With fewer than two members the suggestion action is disabled and the
        // "text everyone" action (needs >= 2 resolved phones) is disabled too.
        composeTestRule.onNodeWithContentDescription(
            str(R.string.households_suggest_relationships),
        ).assertIsNotEnabled()
        composeTestRule.onNodeWithContentDescription(
            str(R.string.households_text_everyone),
        ).assertIsNotEnabled()
    }

    @Test
    fun `suggest relationships calls the repository when enabled`() {
        val householdRepository = mockk<HouseholdRepository>()
        val contactRepository = mockk<ContactRepository>()
        coEvery { householdRepository.getWithMembers("h1") } returns Result.success(
            HouseholdDetail(
                household = Household(id = "h1", name = "Home", type = HouseholdTypes.FAMILY_UNIT),
                members = listOf(
                    HouseholdMember(id = 1, householdId = "h1", memberVCardUid = "uid-1"),
                    HouseholdMember(id = 2, householdId = "h1", memberVCardUid = "uid-2"),
                ),
            ),
        )
        coEvery { contactRepository.resolveByUid(any()) } returns Result.success(emptyMap())
        coEvery { householdRepository.suggestRelationships("h1") } returns Result.success(emptyList())
        val vm = HouseholdDetailViewModel(
            householdRepository,
            contactRepository,
            SavedStateHandle(mapOf("householdId" to "h1")),
        )
        composeTestRule.setContent {
            MycorrhizalTheme { HouseholdDetailScreen(onBack = {}, onNavigateToContact = {}, viewModel = vm) }
        }

        composeTestRule.onNodeWithContentDescription(
            str(R.string.households_suggest_relationships),
        ).performClick()

        coVerify(exactly = 1) { householdRepository.suggestRelationships("h1") }
    }

    @Test
    fun `the add-member dialog opens with search and a disabled add button`() {
        val householdRepository = mockk<HouseholdRepository>()
        val contactRepository = mockk<ContactRepository>()
        coEvery { householdRepository.getWithMembers("h1") } returns Result.success(
            HouseholdDetail(
                household = Household(id = "h1", name = "Home", type = HouseholdTypes.FAMILY_UNIT),
                members = emptyList(),
            ),
        )
        coEvery { contactRepository.resolveByUid(any()) } returns Result.success(emptyMap())
        val vm = HouseholdDetailViewModel(
            householdRepository,
            contactRepository,
            SavedStateHandle(mapOf("householdId" to "h1")),
        )
        composeTestRule.setContent {
            MycorrhizalTheme { HouseholdDetailScreen(onBack = {}, onNavigateToContact = {}, viewModel = vm) }
        }

        composeTestRule.onNodeWithContentDescription(
            str(R.string.households_add_member),
        ).performClick()

        // The dialog is a search-and-select surface: a search field plus a
        // role picker, with Add gated on a selected contact.
        composeTestRule.onNodeWithText(
            str(R.string.households_member_search),
        ).assertIsDisplayed()
        composeTestRule.onNodeWithText(str(R.string.households_role))
            .assertIsDisplayed()
        composeTestRule.onNodeWithText(str(R.string.action_add))
            .assertIsNotEnabled()
    }
}
