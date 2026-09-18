package com.mycorrhizal.crm.feature.circles

import android.content.Context
import androidx.annotation.StringRes
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.domain.repository.CircleRepository
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.a11y.assertAccessibleSemantics
import com.mycorrhizal.crm.testing.a11y.assertNoDuplicateContentDescriptions
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.CompletableDeferred
import org.junit.Assert.assertTrue
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

    private fun str(@StringRes res: Int, vararg args: Any): String =
        ApplicationProvider.getApplicationContext<Context>().getString(res, *args)

    private fun setScreen(result: Result<List<Circle>>, darkTheme: Boolean = false) {
        val repository = mockk<CircleRepository>()
        coEvery { repository.list(any(), any()) } returns result
        val viewModel = CirclesViewModel(repository)

        composeTestRule.setContent {
            MycorrhizalTheme(darkTheme = darkTheme) {
                CirclesScreen(onOpenCircle = {}, viewModel = viewModel)
            }
        }
    }

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

    @Test
    fun `an initial load renders the loading skeleton`() {
        val gate = CompletableDeferred<Result<List<Circle>>>()
        val repository = mockk<CircleRepository>()
        coEvery { repository.list(any(), any()) } coAnswers { gate.await() }
        val viewModel = CirclesViewModel(repository)

        composeTestRule.setContent {
            MycorrhizalTheme {
                CirclesScreen(onOpenCircle = {}, viewModel = viewModel)
            }
        }

        composeTestRule.onNodeWithContentDescription(str(R.string.a11y_state_loading)).assertIsDisplayed()
    }

    @Test
    fun `an empty list renders the empty state`() {
        setScreen(Result.success(emptyList()))

        composeTestRule.onNodeWithText(str(R.string.circles_empty)).assertIsDisplayed()
    }

    @Test
    fun `a failed load with an empty list renders the error text`() {
        setScreen(Result.failure(ApiError.Client(500, "boom")))

        // The error is both the body Text and the transient snackbar, so assert
        // on the collection rather than a single node.
        assertTrue(composeTestRule.onAllNodesWithText("boom").fetchSemanticsNodes().isNotEmpty())
    }
}
