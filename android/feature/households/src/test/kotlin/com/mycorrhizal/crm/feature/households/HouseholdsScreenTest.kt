package com.mycorrhizal.crm.feature.households

import android.content.Context
import androidx.annotation.StringRes
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.HouseholdRepository
import com.mycorrhizal.crm.model.network.AddressHouseholdSuggestion
import com.mycorrhizal.crm.model.network.Household
import com.mycorrhizal.crm.model.network.HouseholdTypes
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.a11y.assertAccessibleSemantics
import com.mycorrhizal.crm.testing.a11y.assertNoDuplicateContentDescriptions
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.mockk
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issue #214: mounts the real top-level [HouseholdsScreen] (Scaffold +
 * TopAppBar + FAB included) against a [HouseholdsViewModel] backed by mocked
 * repositories — the same construction [HouseholdsViewModelTest] uses, no
 * Hilt container required — and sweeps it for static a11y invariants.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class HouseholdsScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun str(@StringRes res: Int, vararg args: Any): String =
        ApplicationProvider.getApplicationContext<Context>().getString(res, *args)

    private fun setScreen(darkTheme: Boolean) {
        val householdRepository = mockk<HouseholdRepository>()
        val contactRepository = mockk<ContactRepository>()
        coEvery { householdRepository.list(any(), any()) } returns Result.success(
            listOf(Household(id = "h1", name = "The Smiths"), Household(id = "h2", name = "The Joneses")),
        )
        val viewModel = HouseholdsViewModel(householdRepository, contactRepository)

        composeTestRule.setContent {
            MycorrhizalTheme(darkTheme = darkTheme) {
                HouseholdsScreen(onOpenHousehold = {}, viewModel = viewModel)
            }
        }
    }

    @Test
    fun `households screen has no accessibility violations (light)`() {
        setScreen(darkTheme = false)

        composeTestRule.assertAccessibleSemantics()
    }

    @Test
    fun `row action labels are unique per row`() {
        // #205: two seeded households must not both announce a bare
        // "Rename"/"Delete" — each row's actions carry the household's name.
        setScreen(darkTheme = false)

        composeTestRule.assertNoDuplicateContentDescriptions()
    }

    @Test
    fun `households screen has no accessibility violations (dark)`() {
        setScreen(darkTheme = true)

        composeTestRule.assertAccessibleSemantics()
    }

    // --- Empty/error/suggestion branches of the top-level screen ------------

    private fun setScreenWithList(result: Result<List<Household>>) {
        val householdRepository = mockk<HouseholdRepository>()
        val contactRepository = mockk<ContactRepository>()
        coEvery { householdRepository.list(any(), any()) } returns result
        val viewModel = HouseholdsViewModel(householdRepository, contactRepository)

        composeTestRule.setContent {
            MycorrhizalTheme {
                HouseholdsScreen(onOpenHousehold = {}, viewModel = viewModel)
            }
        }
        composeTestRule.waitForIdle()
    }

    @Test
    fun `an empty list renders the empty state`() {
        setScreenWithList(Result.success(emptyList()))

        composeTestRule.onNodeWithText(str(R.string.households_empty)).assertIsDisplayed()
    }

    @Test
    fun `a failed load renders the inline error`() {
        setScreenWithList(Result.failure(ApiError.Client(500, "boom")))

        assertTrue(
            composeTestRule.onAllNodesWithText("boom").fetchSemanticsNodes().isNotEmpty(),
        )
    }

    private fun setScreenWithSuggestions(
        suggestions: List<AddressHouseholdSuggestion>,
        onAccept: (() -> Unit)? = null,
        onDismiss: (() -> Unit)? = null,
    ) {
        val householdRepository = mockk<HouseholdRepository>()
        val contactRepository = mockk<ContactRepository>()
        coEvery { householdRepository.list(any(), any()) } returns Result.success(emptyList())
        coEvery { householdRepository.suggestAddressHouseholds() } returns Result.success(suggestions)
        coEvery { contactRepository.resolveByUid(any()) } returns Result.success(emptyMap())
        coEvery { householdRepository.acceptAddressSuggestion(any()) } returns Result.success(
            Household(id = "h9", name = "Alice & Bob", type = HouseholdTypes.FAMILY_UNIT),
        )
        coEvery { householdRepository.dismissAddressSuggestion(any()) } returns Result.success(Unit)
        val viewModel = HouseholdsViewModel(householdRepository, contactRepository)

        composeTestRule.setContent {
            MycorrhizalTheme {
                HouseholdsScreen(onOpenHousehold = {}, viewModel = viewModel)
            }
        }
        composeTestRule.runOnIdle { viewModel.scanAddressSuggestions() }
        composeTestRule.waitForIdle()
        onAccept?.invoke()
        onDismiss?.invoke()
    }

    @Test
    fun `loaded address suggestions render the header and their cards`() {
        setScreenWithSuggestions(
            listOf(
                AddressHouseholdSuggestion(
                    addressHash = "ah1",
                    memberHash = "mh1",
                    memberVCardUids = listOf("u1", "u2"),
                ),
            ),
        )

        composeTestRule.onNodeWithText(str(R.string.households_address_suggestions)).assertIsDisplayed()
        composeTestRule.onNodeWithText(str(R.string.households_accept_suggestion)).assertIsDisplayed()
    }

    @Test
    fun `a scan with nothing to suggest renders the suggestion empty state`() {
        setScreenWithSuggestions(emptyList())

        composeTestRule.onNodeWithText(str(R.string.households_no_address_suggestions)).assertIsDisplayed()
    }

    @Test
    fun `accepting a suggestion invokes the accept action`() {
        val suggestion = AddressHouseholdSuggestion(
            addressHash = "ah1",
            memberHash = "mh1",
            memberVCardUids = listOf("u1", "u2"),
        )
        setScreenWithSuggestions(
            suggestions = listOf(suggestion),
            onAccept = {
                composeTestRule.onNodeWithText(str(R.string.households_accept_suggestion)).performClick()
                composeTestRule.waitForIdle()
            },
        )

        assertTrue(
            composeTestRule.onAllNodesWithText(str(R.string.households_accept_suggestion))
                .fetchSemanticsNodes().isEmpty(),
        )
    }

    @Test
    fun `dismissing a suggestion invokes the dismiss action`() {
        val suggestion = AddressHouseholdSuggestion(
            addressHash = "ah1",
            memberHash = "mh1",
            memberVCardUids = listOf("u1", "u2"),
        )
        setScreenWithSuggestions(
            suggestions = listOf(suggestion),
            onDismiss = {
                composeTestRule.onNodeWithText(str(R.string.households_dismiss_suggestion)).performClick()
                composeTestRule.waitForIdle()
            },
        )

        assertTrue(
            composeTestRule.onAllNodesWithText(str(R.string.households_dismiss_suggestion))
                .fetchSemanticsNodes().isEmpty(),
        )
    }
}
