package com.mycorrhizal.crm.feature.tags

import android.content.Context
import androidx.annotation.StringRes
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertHasClickAction
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.lifecycle.SavedStateHandle
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.TagDetail
import com.mycorrhizal.crm.domain.repository.TagRepository
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.ContactTag
import com.mycorrhizal.crm.model.network.Tag
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.a11y.assertAccessibleSemantics
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.CompletableDeferred
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
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

    private val alice = ContactSummary(id = 42, uid = "uid-alice", firstname = "Alice", lastname = "Smith")

    private fun contactRepo(resolve: Map<String, ContactSummary> = emptyMap()): ContactRepository {
        val repo = mockk<ContactRepository>()
        coEvery { repo.resolveByUid(any()) } returns Result.success(resolve)
        return repo
    }

    private fun viewModel(resolve: Map<String, ContactSummary> = mapOf("uid-alice" to alice)): TagDetailViewModel {
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
        return TagDetailViewModel(
            repository,
            contactRepo(resolve),
            SavedStateHandle(mapOf("tagId" to "t1")),
        )
    }

    private fun setScreen(
        onNavigate: (Int) -> Unit = {},
        resolve: Map<String, ContactSummary> = mapOf("uid-alice" to alice),
    ) {
        val vm = viewModel(resolve)
        composeTestRule.setContent {
            MycorrhizalTheme(darkTheme = false) {
                TagDetailScreen(onBack = {}, onNavigateToContact = onNavigate, viewModel = vm)
            }
        }
    }

    @Test
    fun `tag detail renders the tag name and its tagged contacts`() {
        setScreen()

        composeTestRule.onNodeWithText("Work").assertIsDisplayed()
        // Issue #1768: names, never the raw vCard UID.
        composeTestRule.onNodeWithText("Alice Smith").assertIsDisplayed()
        composeTestRule.onNodeWithText("uid-alice").assertDoesNotExist()
    }

    @Test
    fun `a tagged contact missing from the cache shows Unknown contact never its UID`() {
        setScreen()

        composeTestRule.onNodeWithText(str(R.string.contact_unknown)).assertIsDisplayed()
        composeTestRule.onNodeWithText("uid-bob").assertDoesNotExist()
        composeTestRule.onNodeWithContentDescription(
            str(R.string.tags_remove_contact_named, str(R.string.contact_unknown)),
        ).assertIsDisplayed()
    }

    @Test
    fun `tapping a resolved row opens the contact and an unresolved row is inert`() {
        var opened: Int? = null
        setScreen(onNavigate = { opened = it })

        composeTestRule.onNodeWithText(str(R.string.contact_unknown)).performClick()
        assertEquals(null, opened)
        composeTestRule.onNodeWithText("Alice Smith").assertHasClickAction().performClick()
        assertEquals(42, opened)
    }

    @Test
    fun `resolved rows render with a thumbnail or a full photo`() {
        var opened: Int? = null
        setScreen(
            onNavigate = { opened = it },
            resolve = mapOf(
                "uid-alice" to alice.copy(photoThumbnail = "/thumb-a.jpg"),
                "uid-bob" to ContactSummary(id = 43, uid = "uid-bob", firstname = "Bob", photo = "/photo-b.jpg"),
            ),
        )

        composeTestRule.onNodeWithText("Alice Smith").assertIsDisplayed()
        composeTestRule.onNodeWithText("Bob").performClick()
        assertEquals(43, opened)
    }

    @Test
    fun `a resolved contact without a numeric id is not tappable`() {
        var opened: Int? = null
        setScreen(onNavigate = { opened = it }, resolve = mapOf("uid-alice" to alice.copy(id = 0)))

        composeTestRule.onNodeWithText("Alice Smith").performClick()
        assertEquals(null, opened)
    }

    @Test
    fun `the remove button announces the contact name`() {
        setScreen()

        composeTestRule.onNodeWithContentDescription(str(R.string.tags_remove_contact_named, "Alice Smith"))
            .assertIsDisplayed()
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
        val vm = TagDetailViewModel(repository, contactRepo(), SavedStateHandle(mapOf("tagId" to "t1")))
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
        val vm = TagDetailViewModel(repository, contactRepo(mapOf("uid-alice" to alice)), SavedStateHandle(mapOf("tagId" to "t1")))
        composeTestRule.setContent {
            MycorrhizalTheme { TagDetailScreen(onBack = {}, viewModel = vm) }
        }

        composeTestRule.onNodeWithContentDescription(str(R.string.tags_remove_contact_named, "Alice Smith"))
            .performClick()

        coVerify(exactly = 1) { repository.removeContact("t1", "uid-alice") }
        composeTestRule.onNodeWithText("Alice Smith").assertDoesNotExist()
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
        val vm = TagDetailViewModel(repository, contactRepo(), SavedStateHandle(mapOf("tagId" to "t1")))
        composeTestRule.setContent {
            MycorrhizalTheme { TagDetailScreen(onBack = {}, viewModel = vm) }
        }

        composeTestRule.onNodeWithContentDescription(str(R.string.tags_add_contact)).performClick()

        // Add is disabled while the field is blank, enabled once a uid is typed.
        composeTestRule.onNodeWithText(str(R.string.action_add)).assertIsNotEnabled()
        composeTestRule.onNodeWithText(str(R.string.tags_contact_vcard_uid)).performTextInput("uid-new")
        composeTestRule.onNodeWithText(str(R.string.action_add)).assertIsEnabled().performClick()

        coVerify(exactly = 1) { repository.addContact("t1", "uid-new") }
        composeTestRule.onNodeWithText(str(R.string.contact_unknown)).assertIsDisplayed()
    }

    @Test
    fun `an initial load renders the loading skeleton`() {
        val gate = CompletableDeferred<Result<TagDetail>>()
        val repository = mockk<TagRepository>()
        coEvery { repository.getWithContacts("t1") } coAnswers { gate.await() }
        val vm = TagDetailViewModel(repository, contactRepo(), SavedStateHandle(mapOf("tagId" to "t1")))
        composeTestRule.setContent {
            MycorrhizalTheme { TagDetailScreen(onBack = {}, viewModel = vm) }
        }

        composeTestRule.onNodeWithContentDescription(str(R.string.a11y_state_loading)).assertIsDisplayed()
    }

    @Test
    fun `a failed load with no contacts renders the error text`() {
        val repository = mockk<TagRepository>()
        coEvery { repository.getWithContacts("t1") } returns
            Result.failure(ApiError.Client(500, "boom"))
        val vm = TagDetailViewModel(repository, contactRepo(), SavedStateHandle(mapOf("tagId" to "t1")))
        composeTestRule.setContent {
            MycorrhizalTheme { TagDetailScreen(onBack = {}, viewModel = vm) }
        }

        // The error is both the body Text and the transient snackbar, so assert
        // on the collection rather than a single node.
        assertTrue(composeTestRule.onAllNodesWithText("boom").fetchSemanticsNodes().isNotEmpty())
    }
}
