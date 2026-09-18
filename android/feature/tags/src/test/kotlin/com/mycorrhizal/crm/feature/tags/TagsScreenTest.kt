package com.mycorrhizal.crm.feature.tags

import android.content.Context
import androidx.annotation.StringRes
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.domain.repository.TagRepository
import com.mycorrhizal.crm.model.network.Tag
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
 * Issue #214: mounts the real top-level [TagsScreen] (Scaffold + TopAppBar +
 * FAB included) against a [TagsViewModel] backed by a mocked [TagRepository]
 * — the same construction [TagsViewModelTest] uses, no Hilt container
 * required — and sweeps it for static a11y invariants.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class TagsScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun str(@StringRes res: Int, vararg args: Any): String =
        ApplicationProvider.getApplicationContext<Context>().getString(res, *args)

    private fun setScreen(result: Result<List<Tag>>) {
        val repository = mockk<TagRepository>()
        coEvery { repository.list(any(), any()) } returns result
        val viewModel = TagsViewModel(repository)

        composeTestRule.setContent {
            MycorrhizalTheme {
                TagsScreen(onOpenTag = {}, viewModel = viewModel)
            }
        }
    }

    private fun setScreen(darkTheme: Boolean) {
        val repository = mockk<TagRepository>()
        coEvery { repository.list(any(), any()) } returns Result.success(
            listOf(Tag(id = "t1", name = "vip"), Tag(id = "t2", name = "colleague")),
        )
        val viewModel = TagsViewModel(repository)

        composeTestRule.setContent {
            MycorrhizalTheme(darkTheme = darkTheme) {
                TagsScreen(onOpenTag = {}, viewModel = viewModel)
            }
        }
    }

    @Test
    fun `tags screen has no accessibility violations (light)`() {
        setScreen(darkTheme = false)

        composeTestRule.assertAccessibleSemantics()
    }

    @Test
    fun `row action labels are unique per row`() {
        // #205: two seeded tags must not both announce a bare "Rename"/"Delete" —
        // each row's actions carry the tag's name.
        setScreen(darkTheme = false)

        composeTestRule.assertNoDuplicateContentDescriptions()
    }

    @Test
    fun `tags screen has no accessibility violations (dark)`() {
        setScreen(darkTheme = true)

        composeTestRule.assertAccessibleSemantics()
    }

    @Test
    fun `the rename dialog title is a heading`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                TagNameDialog(
                    title = "Rename tag",
                    initial = "vip",
                    confirmLabel = "Save",
                    onConfirm = {},
                    onDismiss = {},
                )
            }
        }

        // #208: AlertDialog title slots aren't marked as headings by default,
        // so TalkBack's heading navigation skipped this dialog entirely.
        composeTestRule.onNodeWithText("Rename tag")
            .assert(SemanticsMatcher.keyIsDefined(SemanticsProperties.Heading))
    }

    @Test
    fun `an initial load renders the loading skeleton`() {
        val gate = CompletableDeferred<Result<List<Tag>>>()
        val repository = mockk<TagRepository>()
        coEvery { repository.list(any(), any()) } coAnswers { gate.await() }
        val viewModel = TagsViewModel(repository)

        composeTestRule.setContent {
            MycorrhizalTheme {
                TagsScreen(onOpenTag = {}, viewModel = viewModel)
            }
        }

        composeTestRule.onNodeWithContentDescription(str(R.string.a11y_state_loading)).assertIsDisplayed()
    }

    @Test
    fun `an empty list renders the empty state`() {
        setScreen(Result.success(emptyList()))

        composeTestRule.onNodeWithText(str(R.string.tags_empty)).assertIsDisplayed()
    }

    @Test
    fun `a failed load with an empty list renders the error text`() {
        setScreen(Result.failure(ApiError.Client(500, "boom")))

        // The error is both the body Text and the transient snackbar, so assert
        // on the collection rather than a single node.
        assertTrue(composeTestRule.onAllNodesWithText("boom").fetchSemanticsNodes().isNotEmpty())
    }
}
