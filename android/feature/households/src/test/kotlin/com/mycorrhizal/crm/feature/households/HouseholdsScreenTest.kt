package com.mycorrhizal.crm.feature.households

import androidx.compose.ui.test.junit4.createComposeRule
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.HouseholdRepository
import com.mycorrhizal.crm.model.network.Household
import com.mycorrhizal.crm.testing.a11y.assertAccessibleSemantics
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
    fun `households screen has no accessibility violations (dark)`() {
        setScreen(darkTheme = true)

        composeTestRule.assertAccessibleSemantics()
    }
}
