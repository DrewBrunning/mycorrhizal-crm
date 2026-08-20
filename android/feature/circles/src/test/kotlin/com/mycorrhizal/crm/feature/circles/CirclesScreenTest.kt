package com.mycorrhizal.crm.feature.circles

import androidx.compose.ui.test.junit4.createComposeRule
import com.mycorrhizal.crm.domain.repository.CircleRepository
import com.mycorrhizal.crm.model.network.Circle
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
 * Issue #214: mounts the real top-level [CirclesScreen] (Scaffold + TopAppBar
 * + FAB included) against a [CirclesViewModel] backed by a mocked
 * [CircleRepository] — the same construction [CirclesViewModelTest] uses, no
 * Hilt container required — and sweeps it for static a11y invariants.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class CirclesScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setScreen(darkTheme: Boolean) {
        val repository = mockk<CircleRepository>()
        coEvery { repository.list(any(), any()) } returns Result.success(
            listOf(Circle(id = "c1", name = "Friends"), Circle(id = "c2", name = "Family")),
        )
        val viewModel = CirclesViewModel(repository)

        composeTestRule.setContent {
            MycorrhizalTheme(darkTheme = darkTheme) {
                CirclesScreen(onOpenCircle = {}, viewModel = viewModel)
            }
        }
    }

    @Test
    fun `circles screen has no accessibility violations (light)`() {
        setScreen(darkTheme = false)

        composeTestRule.assertAccessibleSemantics()
    }

    @Test
    fun `row action labels are unique per row`() {
        // #205: two seeded circles must not both announce a bare
        // "Rename"/"Delete" — each row's actions carry the circle's name.
        setScreen(darkTheme = false)

        composeTestRule.assertNoDuplicateContentDescriptions()
    }

    @Test
    fun `circles screen has no accessibility violations (dark)`() {
        setScreen(darkTheme = true)

        composeTestRule.assertAccessibleSemantics()
    }
}
