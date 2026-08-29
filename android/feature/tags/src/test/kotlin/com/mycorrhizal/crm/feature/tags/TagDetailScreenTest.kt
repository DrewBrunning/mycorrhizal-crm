package com.mycorrhizal.crm.feature.tags

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
import com.mycorrhizal.crm.domain.repository.TagDetail
import com.mycorrhizal.crm.domain.repository.TagRepository
import com.mycorrhizal.crm.model.network.ContactTag
import com.mycorrhizal.crm.model.network.Tag
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
 * Issue #682: the tag detail screen — member list, remove action, and the
 * add-contact dialog — had no test coverage at all. Mounts the real screen
 * against a [TagDetailViewModel] backed by a mocked [TagRepository].
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class TagDetailScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun str(@StringRes res: Int, vararg args: Any): String =
        ApplicationProvider.getApplicationContext<Context>().getString(res, *args)

    private fun viewModel(): TagDetailViewModel {
        val repository = mockk<TagRepository>()
        coEvery { repository.getWithContacts("t1") } returns Result.success(
            TagDetail(
                tag = Tag(id = "t1", name = "Work"),
                contacts = listOf(
                    ContactTag(id = 1, tagId = "t1", contactVCardUid = "uid-alice"),
                    ContactTag(id = 2, tagId = "t1", contactVCardUid = "uid-bob"),
                ),
            ),
        )
        return TagDetailViewModel(repository, SavedStateHandle(mapOf("tagId" to "t1")))
    }

    private fun setScreen() {
        val vm = viewModel()
        composeTestRule.setContent {
            MycorrhizalTheme(darkTheme = false) {
                TagDetailScreen(onBack = {}, viewModel = vm)
            }
        }
    }

    @Test
    fun `tag detail renders the tag name and its tagged contacts`() {
        setScreen()

        composeTestRule.onNodeWithText("Work").assertIsDisplayed()
        composeTestRule.onNodeWithText("uid-alice").assertIsDisplayed()
        composeTestRule.onNodeWithText("uid-bob").assertIsDisplayed()
    }

    @Test
    fun `tag detail has no accessibility violations`() {
        setScreen()

        composeTestRule.assertAccessibleSemantics()
    }

    @Test
    fun `an empty tag shows the empty state`() {
        val repository = mockk<TagRepository>()
        coEvery { repository.getWithContacts("t1") } returns Result.success(
            TagDetail(tag = Tag(id = "t1", name = "Empty"), contacts = emptyList()),
        )
        val vm = TagDetailViewModel(repository, SavedStateHandle(mapOf("tagId" to "t1")))
        composeTestRule.setContent {
            MycorrhizalTheme { TagDetailScreen(onBack = {}, viewModel = vm) }
        }

        composeTestRule.onNodeWithText(str(R.string.tags_contacts_empty)).assertIsDisplayed()
    }

    @Test
    fun `removing a tagged contact calls the repository`() {
        val repository = mockk<TagRepository>()
        coEvery { repository.getWithContacts("t1") } returns Result.success(
            TagDetail(
                tag = Tag(id = "t1", name = "Work"),
                contacts = listOf(ContactTag(id = 1, tagId = "t1", contactVCardUid = "uid-alice")),
            ),
        )
        coEvery { repository.removeContact("t1", "uid-alice") } returns Result.success(Unit)
        val vm = TagDetailViewModel(repository, SavedStateHandle(mapOf("tagId" to "t1")))
        composeTestRule.setContent {
            MycorrhizalTheme { TagDetailScreen(onBack = {}, viewModel = vm) }
        }

        composeTestRule.onNodeWithContentDescription(str(R.string.tags_remove_contact_named, "uid-alice"))
            .performClick()

        coVerify(exactly = 1) { repository.removeContact("t1", "uid-alice") }
        composeTestRule.onNodeWithText("uid-alice").assertDoesNotExist()
    }

    @Test
    fun `the add-contact dialog confirms a typed uid`() {
        val repository = mockk<TagRepository>()
        coEvery { repository.getWithContacts("t1") } returns Result.success(
            TagDetail(tag = Tag(id = "t1", name = "Work"), contacts = emptyList()),
        )
        coEvery { repository.addContact("t1", "uid-new") } returns Result.success(
            ContactTag(id = 3, tagId = "t1", contactVCardUid = "uid-new"),
        )
        val vm = TagDetailViewModel(repository, SavedStateHandle(mapOf("tagId" to "t1")))
        composeTestRule.setContent {
            MycorrhizalTheme { TagDetailScreen(onBack = {}, viewModel = vm) }
        }

        composeTestRule.onNodeWithContentDescription(str(R.string.tags_add_contact)).performClick()

        // Add is disabled while the field is blank, enabled once a uid is typed.
        composeTestRule.onNodeWithText(str(R.string.action_add)).assertIsNotEnabled()
        composeTestRule.onNodeWithText(str(R.string.tags_contact_vcard_uid)).performTextInput("uid-new")
        composeTestRule.onNodeWithText(str(R.string.action_add)).assertIsEnabled().performClick()

        coVerify(exactly = 1) { repository.addContact("t1", "uid-new") }
        composeTestRule.onNodeWithText("uid-new").assertIsDisplayed()
    }
}
