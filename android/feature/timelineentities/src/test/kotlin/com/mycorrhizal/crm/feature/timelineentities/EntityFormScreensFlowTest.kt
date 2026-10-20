package com.mycorrhizal.crm.feature.timelineentities

import androidx.compose.ui.test.assertIsDisplayed
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
import com.mycorrhizal.crm.model.network.Gift
import com.mycorrhizal.crm.model.network.LifeEvent
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.testing.MainDispatcherRule
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
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
}

