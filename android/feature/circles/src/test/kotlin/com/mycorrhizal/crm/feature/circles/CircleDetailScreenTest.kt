package com.mycorrhizal.crm.feature.circles

import android.content.Context
import androidx.annotation.StringRes
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
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

    private val alice = ContactSummary(
        id = 42,
        uid = "uid-alice",
        firstname = "Alice",
        lastname = "Smith"
    )

    private fun str(@StringRes res: Int, vararg args: Any): String =
        ApplicationProvider.getApplicationContext<Context>().getString(res, *args)

    private fun setScreen(
        members: List<CircleMember> = listOf(
            CircleMember(id = 1, circleId = "c1", memberVCardUid = "uid-alice")
        ),
        resolve: Map<String, ContactSummary> = emptyMap(),
        onNavigate: (Int) -> Unit = {}
    ) {
        val circleRepository = mockk<CircleRepository>()
        val contactRepository = mockk<ContactRepository>()
        coEvery { circleRepository.getWithMembers("c1") } returns Result.success(
            CircleDetail(circle = Circle(id = "c1", name = "Family"), members = members)
        )
        coEvery { contactRepository.resolveByUid(any()) } returns Result.success(resolve)
        val vm = CircleDetailViewModel(
            circleRepository,
            contactRepository,
            SavedStateHandle(mapOf("circleId" to "c1"))
        )
        composeTestRule.setContent {
            MycorrhizalTheme(darkTheme = false) {
                CircleDetailScreen(onBack = {}, onNavigateToContact = onNavigate, viewModel = vm)
            }
        }
    }

    @Test
    fun `circle detail renders the circle name and its members`() {
        setScreen(resolve = mapOf("uid-alice" to alice))

        composeTestRule.onNodeWithText("Family").assertIsDisplayed()
        // Issue #1768: the contact's name, never the raw vCard UID.
        composeTestRule.onNodeWithText("Alice Smith").assertIsDisplayed()
        composeTestRule.onNodeWithText("uid-alice").assertDoesNotExist()
    }

    @Test
    fun `a member missing from the cache shows Unknown contact never its UID`() {
        setScreen()

        composeTestRule.onNodeWithText(str(R.string.contact_unknown)).assertIsDisplayed()
        composeTestRule.onNodeWithText("uid-alice").assertDoesNotExist()
        composeTestRule.onNodeWithContentDescription(
            str(R.string.circles_remove_member_named, str(R.string.contact_unknown))
        ).assertIsDisplayed()
    }

    @Test
    fun `tapping a resolved row opens the contact and an unresolved row is inert`() {
        var opened: Int? = null
        setScreen(
            members = listOf(
                CircleMember(id = 1, circleId = "c1", memberVCardUid = "uid-alice"),
                CircleMember(id = 2, circleId = "c1", memberVCardUid = "uid-ghost")
            ),
            resolve = mapOf("uid-alice" to alice),
            onNavigate = { opened = it }
        )

        composeTestRule.onNodeWithText(str(R.string.contact_unknown)).performClick()
        assertEquals(null, opened)
        composeTestRule.onNodeWithText("Alice Smith").performClick()
        assertEquals(42, opened)
    }

    @Test
    fun `resolved rows render with a thumbnail or a full photo`() {
        var opened: Int? = null
        setScreen(
            members = listOf(
                CircleMember(id = 1, circleId = "c1", memberVCardUid = "uid-alice"),
                CircleMember(id = 2, circleId = "c1", memberVCardUid = "uid-bob")
            ),
            resolve = mapOf(
                "uid-alice" to alice.copy(photoThumbnail = "/thumb-a.jpg"),
                "uid-bob" to ContactSummary(
                    id = 43,
                    uid = "uid-bob",
                    firstname = "Bob",
                    photo = "/photo-b.jpg"
                )
            ),
            onNavigate = { opened = it }
        )

        composeTestRule.onNodeWithText("Alice Smith").assertIsDisplayed()
        composeTestRule.onNodeWithText("Bob").performClick()
        assertEquals(43, opened)
    }

    @Test
    fun `a resolved contact without a numeric id is not tappable`() {
        var opened: Int? = null
        setScreen(resolve = mapOf("uid-alice" to alice.copy(id = 0)), onNavigate = { opened = it })

        composeTestRule.onNodeWithText("Alice Smith").performClick()
        assertEquals(null, opened)
    }

    @Test
    fun `the remove button announces the member name`() {
        setScreen(resolve = mapOf("uid-alice" to alice))

        composeTestRule.onNodeWithContentDescription(
            str(R.string.circles_remove_member_named, "Alice Smith")
        )
            .assertIsDisplayed()
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
                members = listOf(
                    CircleMember(id = 1, circleId = "c1", memberVCardUid = "uid-alice")
                )
            )
        )
        coEvery {
            contactRepository.resolveByUid(
                any()
            )
        } returns Result.success(mapOf("uid-alice" to alice))
        coEvery { circleRepository.removeMember("c1", "uid-alice") } returns Result.success(Unit)
        val vm = CircleDetailViewModel(
            circleRepository,
            contactRepository,
            SavedStateHandle(mapOf("circleId" to "c1"))
        )
        composeTestRule.setContent {
            MycorrhizalTheme { CircleDetailScreen(onBack = {}, viewModel = vm) }
        }

        composeTestRule.onNodeWithContentDescription(
            str(R.string.circles_remove_member_named, "Alice Smith")
        )
            .performClick()

        coVerify(exactly = 1) { circleRepository.removeMember("c1", "uid-alice") }
        composeTestRule.onNodeWithText("Alice Smith").assertDoesNotExist()
    }

    @Test
    fun `the add-member dialog confirms a typed uid`() {
        val circleRepository = mockk<CircleRepository>()
        val contactRepository = mockk<ContactRepository>()
        coEvery { circleRepository.getWithMembers("c1") } returns Result.success(
            CircleDetail(circle = Circle(id = "c1", name = "Family"), members = emptyList())
        )
        coEvery { contactRepository.resolveByUid(any()) } returns Result.success(emptyMap())
        coEvery { circleRepository.addMember("c1", "uid-new") } returns Result.success(
            CircleMember(id = 2, circleId = "c1", memberVCardUid = "uid-new")
        )
        val vm = CircleDetailViewModel(
            circleRepository,
            contactRepository,
            SavedStateHandle(mapOf("circleId" to "c1"))
        )
        composeTestRule.setContent {
            MycorrhizalTheme { CircleDetailScreen(onBack = {}, viewModel = vm) }
        }

        composeTestRule.onNodeWithContentDescription(
            str(R.string.circles_add_member)
        ).performClick()

        composeTestRule.onNodeWithText(str(R.string.action_add)).assertIsNotEnabled()
        composeTestRule.onNodeWithText(
            str(R.string.circles_member_vcard_uid)
        ).performTextInput("uid-new")
        composeTestRule.onNodeWithText(str(R.string.action_add)).assertIsEnabled().performClick()

        coVerify(exactly = 1) { circleRepository.addMember("c1", "uid-new") }
        composeTestRule.onNodeWithText(str(R.string.contact_unknown)).assertIsDisplayed()
    }

    @Test
    fun `an initial load renders the loading skeleton`() {
        val circleRepository = mockk<CircleRepository>()
        val contactRepository = mockk<ContactRepository>()
        val gate = CompletableDeferred<Result<CircleDetail>>()
        coEvery { circleRepository.getWithMembers("c1") } coAnswers { gate.await() }
        val vm = CircleDetailViewModel(
            circleRepository,
            contactRepository,
            SavedStateHandle(mapOf("circleId" to "c1"))
        )
        composeTestRule.setContent {
            MycorrhizalTheme { CircleDetailScreen(onBack = {}, viewModel = vm) }
        }

        composeTestRule.onNodeWithContentDescription(
            str(R.string.a11y_state_loading)
        ).assertIsDisplayed()
    }

    @Test
    fun `a failed load with no members renders the error text`() {
        val circleRepository = mockk<CircleRepository>()
        val contactRepository = mockk<ContactRepository>()
        coEvery { circleRepository.getWithMembers("c1") } returns
            Result.failure(ApiError.Client(500, "boom"))
        coEvery { contactRepository.resolveByUid(any()) } returns Result.success(emptyMap())
        val vm = CircleDetailViewModel(
            circleRepository,
            contactRepository,
            SavedStateHandle(mapOf("circleId" to "c1"))
        )
        composeTestRule.setContent {
            MycorrhizalTheme { CircleDetailScreen(onBack = {}, viewModel = vm) }
        }

        // The error is both the body Text and the transient snackbar, so assert
        // on the collection rather than a single node.
        assertTrue(composeTestRule.onAllNodesWithText("boom").fetchSemanticsNodes().isNotEmpty())
    }
}
