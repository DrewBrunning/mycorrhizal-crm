package com.mycorrhizal.crm.feature.tags

import androidx.compose.ui.test.junit4.createComposeRule
import com.mycorrhizal.crm.domain.repository.TagRepository
import com.mycorrhizal.crm.model.network.Tag
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
    fun `tags screen has no accessibility violations (dark)`() {
        setScreen(darkTheme = true)

        composeTestRule.assertAccessibleSemantics()
    }
}
