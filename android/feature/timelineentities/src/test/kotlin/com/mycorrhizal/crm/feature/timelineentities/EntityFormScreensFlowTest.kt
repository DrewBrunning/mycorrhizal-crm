package com.mycorrhizal.crm.feature.timelineentities

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.StateRestorationTester
import androidx.compose.ui.test.performScrollToIndex
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextReplacement
import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.ActivityRepository
import com.mycorrhizal.crm.domain.repository.ContactActivitiesPage
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.GiftRepository
import com.mycorrhizal.crm.domain.repository.LifeEventRepository
import com.mycorrhizal.crm.domain.repository.PreferenceRepository
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.Gift
import com.mycorrhizal.crm.model.network.LifeEvent
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.testing.MainDispatcherRule
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import org.junit.Assert.assertEquals
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.UnconfinedTestDispatcher
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issue #1773: on the real screens, "New gift" / "New life event" open a full-screen
 * form that replaces the list, and Back returns to the list without saving; saving
 * creates the entity and returns to the list.
 */
@OptIn(ExperimentalCoroutinesApi::class)
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class EntityFormScreensFlowTest {
    @get:Rule
    val mainDispatcherRule = MainDispatcherRule(UnconfinedTestDispatcher())

    @get:Rule
    val composeTestRule = createComposeRule()

    private val uid = "11111111-1111-4111-8111-111111111111"

    private fun contacts(): ContactRepository = mockk<ContactRepository>().also {
        coEvery { it.getContact(5) } returns Result.success(
            ContactRecordResponse(id = 5, card = Card(uid = uid, name = Name(full = "Dana White"))),
        )
    }

    private fun giftsViewModel(giftRepo: GiftRepository): GiftsViewModel {
        val lifeRepo = mockk<LifeEventRepository>()
        val activityRepo = mockk<ActivityRepository>()
        val prefRepo = mockk<PreferenceRepository>()
        coEvery { lifeRepo.listForContact(uid) } returns Result.success(emptyList())
        coEvery { activityRepo.listForContact(5, any(), any(), any(), any(), any()) } returns
            Result.success(ContactActivitiesPage(activities = emptyList(), nextCursor = null))
        coEvery { prefRepo.listForContact(uid) } returns Result.success(emptyList())
        return GiftsViewModel(giftRepo, lifeRepo, activityRepo, prefRepo, contacts(), SavedStateHandle(mapOf("contactId" to 5)))
    }

    @Test
    fun `new gift opens a full-screen form, back returns to the list, save creates the gift`() {
        val giftRepo = mockk<GiftRepository>()
        coEvery { giftRepo.listForContact(uid) } returns Result.success(emptyList())
        coEvery { giftRepo.create(any()) } returns Result.success(Gift(id = "g1", entityId = uid, description = "Socks"))
        val vm = giftsViewModel(giftRepo)
        composeTestRule.setContent { MycorrhizalTheme { GiftsScreen(onBack = {}, viewModel = vm) } }

        composeTestRule.onNodeWithContentDescription("New gift").performClick()
        // The form replaced the list: the add FAB is gone, the form's status picker is up.
        composeTestRule.onNodeWithContentDescription("New gift").assertDoesNotExist()
        composeTestRule.onNodeWithTag("gift-status").assertIsDisplayed()

        // Back discards and returns to the list.
        composeTestRule.onNodeWithContentDescription("Back").performClick()
        composeTestRule.onNodeWithTag("gift-status").assertDoesNotExist()
        composeTestRule.onNodeWithContentDescription("New gift").assertIsDisplayed()
        coVerify(exactly = 0) { giftRepo.create(any()) }

        // Reopen, fill, save.
        composeTestRule.onNodeWithContentDescription("New gift").performClick()
        composeTestRule.onNodeWithText("Description").performTextReplacement("Socks")
        composeTestRule.onNodeWithText("Create").performClick()
        coVerify(exactly = 1) { giftRepo.create(any()) }
        composeTestRule.onNodeWithContentDescription("New gift").assertIsDisplayed()
    }

    @Test
    fun `new life event opens a full-screen form and saving creates it`() {
        val repo = mockk<LifeEventRepository>()
        coEvery { repo.listForContact(uid) } returns Result.success(emptyList())
        coEvery { repo.create(any()) } returns Result.success(LifeEvent(id = "e1", entityId = uid, type = "married"))
        val vm = LifeEventsViewModel(repo, contacts(), SavedStateHandle(mapOf("contactId" to 5)))
        composeTestRule.setContent { MycorrhizalTheme { LifeEventsScreen(onBack = {}, viewModel = vm) } }

        composeTestRule.onNodeWithContentDescription("New life event").performClick()
        composeTestRule.onNodeWithContentDescription("New life event").assertDoesNotExist()
        composeTestRule.onNodeWithTag("life-event-category").assertIsDisplayed()

        composeTestRule.onNodeWithTag("life-event-category").performClick()
        composeTestRule.onNodeWithText("Family & relationships").performClick()
        composeTestRule.onNodeWithTag("life-event-type").performClick()
        composeTestRule.onNodeWithText("married").performScrollTo().performClick()
        composeTestRule.onNodeWithText("Create").performClick()

        coVerify(exactly = 1) { repo.create(any()) }
        composeTestRule.onNodeWithContentDescription("New life event").assertIsDisplayed()
    }

    @Test
    fun `new life event back leaves the form without saving`() {
        val repo = mockk<LifeEventRepository>()
        coEvery { repo.listForContact(uid) } returns Result.success(emptyList())
        val vm = LifeEventsViewModel(repo, contacts(), SavedStateHandle(mapOf("contactId" to 5)))
        composeTestRule.setContent { MycorrhizalTheme { LifeEventsScreen(onBack = {}, viewModel = vm) } }

        composeTestRule.onNodeWithContentDescription("New life event").performClick()
        composeTestRule.onNodeWithContentDescription("Back").performClick()
        composeTestRule.onNodeWithContentDescription("New life event").assertIsDisplayed()
        coVerify(exactly = 0) { repo.create(any()) }
    }

    // Issue #1773 review: the form replaces the list, so the list's scroll position is
    // hoisted above both and must be where the user left it after Back.
    @Test
    fun `back from an edit form returns to the same scroll position in the list`() {
        val giftRepo = mockk<GiftRepository>()
        val gifts = (0 until 40).map { Gift(id = "g$it", entityId = uid, description = "Gift $it") }
        coEvery { giftRepo.listForContact(uid) } returns Result.success(gifts)
        val vm = giftsViewModel(giftRepo)
        composeTestRule.setContent { MycorrhizalTheme { GiftsScreen(onBack = {}, viewModel = vm) } }

        composeTestRule.onNodeWithTag("entity-list").performScrollToIndex(30)
        composeTestRule.onNodeWithText("Gift 29").performClick()
        composeTestRule.onNodeWithTag("gift-status").assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("Back").performClick()

        composeTestRule.onNodeWithText("Gift 29").assertIsDisplayed()
        composeTestRule.onNodeWithText("Gift 0").assertDoesNotExist()
    }

    // Issue #1773 review: a full-screen form survives a configuration change with what
    // was typed, like the note/activity/reminder forms (the dialog it replaced did not).
    @Test
    fun `an open gift form and its input survive a configuration change`() {
        val giftRepo = mockk<GiftRepository>()
        coEvery { giftRepo.listForContact(uid) } returns Result.success(emptyList())
        val vm = giftsViewModel(giftRepo)
        val restoration = StateRestorationTester(composeTestRule)
        restoration.setContent { MycorrhizalTheme { GiftsScreen(onBack = {}, viewModel = vm) } }

        composeTestRule.onNodeWithContentDescription("New gift").performClick()
        composeTestRule.onNodeWithText("Description").performTextReplacement("Socks")
        restoration.emulateSavedInstanceStateRestore()

        composeTestRule.onNodeWithTag("gift-status").assertIsDisplayed()
        composeTestRule.onNodeWithText("Socks").assertIsDisplayed()
        coVerify(exactly = 0) { giftRepo.create(any()) }
    }

    // Issue #1773 review: the edit form's LaunchedEffect runs again after a configuration
    // change; it must not reset the related contacts the user already changed.
    @Test
    fun `reopening the same life event form keeps its related-contact selection`() {
        val repo = mockk<LifeEventRepository>()
        val event = LifeEvent(id = "e1", entityId = uid, type = "married", relatedEntityIds = listOf("u2"))
        coEvery { repo.listForContact(uid) } returns Result.success(listOf(event))
        val contactRepo = contacts()
        val sam = ContactSummary(id = 2, uid = "u2", fn = "Sam")
        coEvery { contactRepo.resolveByUid(listOf("u2")) } returns Result.success(mapOf("u2" to sam))
        val vm = LifeEventsViewModel(repo, contactRepo, SavedStateHandle(mapOf("contactId" to 5)))

        vm.onDialogOpened(event)
        assertEquals(listOf("u2"), vm.relatedContacts.value.map { it.uid })
        vm.removeRelated("u2")
        vm.onDialogOpened(event) // the recreated composition after a rotation
        assertEquals(emptyList<String?>(), vm.relatedContacts.value.map { it.uid })
        coVerify(exactly = 1) { contactRepo.resolveByUid(listOf("u2")) }

        // Closing the form starts the next one afresh.
        vm.onFormClosed()
        vm.onDialogOpened(event)
        assertEquals(listOf("u2"), vm.relatedContacts.value.map { it.uid })
    }
}
