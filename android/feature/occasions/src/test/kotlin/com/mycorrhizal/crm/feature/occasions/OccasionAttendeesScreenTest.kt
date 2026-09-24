package com.mycorrhizal.crm.feature.occasions

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.CircleRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.OccasionEventRepository
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.InviteeSuggestion
import com.mycorrhizal.crm.model.network.OccasionEvent
import com.mycorrhizal.crm.model.network.OccasionEventAttendeeView
import com.mycorrhizal.crm.model.network.OccasionEventDetail
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.mockk
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class OccasionAttendeesScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val occasionRepository = mockk<OccasionEventRepository>()
    private val circleRepository = mockk<CircleRepository>()
    private val contactRepository = mockk<ContactRepository>()

    private fun render(attendees: List<OccasionEventAttendeeView> = listOf(attendee())) {
        coEvery { occasionRepository.get("e1") } returns Result.success(
            OccasionEventDetail(
                occasionEvent = OccasionEvent(id = "e1", title = "BBQ", startsAt = "2026-07-04T15:00:00Z"),
                attendees = attendees,
            ),
        )
        coEvery { circleRepository.list(any(), any()) } returns Result.success(
            listOf(Circle(id = "c1", name = "Friends")),
        )
        coEvery { occasionRepository.suggestInvitees(listOf("c1"), "e1") } returns Result.success(
            listOf(InviteeSuggestion(contactId = 5, contactName = "Bob", entityId = "uid-5")),
        )
        val viewModel = OccasionAttendeesViewModel(
            occasionRepository,
            circleRepository,
            contactRepository,
            SavedStateHandle(mapOf("eventId" to "e1")),
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                OccasionAttendeesScreen(onBack = {}, viewModel = viewModel)
            }
        }
        composeTestRule.waitForIdle()
    }

    private fun attendee() = OccasionEventAttendeeView(
        id = "a1",
        eventId = "e1",
        entityId = "uid-1",
        contactId = 3,
        contactName = "Alice",
        rsvp = "pending",
    )

    @Test
    fun `renders the attendee and the RSVP hint`() {
        render()
        composeTestRule.onNodeWithText("Attendees — BBQ").assertIsDisplayed()
        composeTestRule.onNodeWithText("Alice").assertIsDisplayed()
        composeTestRule
            .onNodeWithText("Record what each guest told you — no invitation is sent from here.")
            .assertIsDisplayed()
    }

    @Test
    fun `shows the empty state when nobody is invited`() {
        render(attendees = emptyList())
        composeTestRule.onNodeWithText("No one invited yet.").assertIsDisplayed()
    }

    @Test
    fun `the remove action opens a confirmation`() {
        render()
        composeTestRule.onNodeWithContentDescription("Remove attendee").performClick()
        composeTestRule.onNodeWithText("Remove Alice from this event?").assertIsDisplayed()
    }

    @Test
    fun `suggesting from a selected circle lists candidates`() {
        render()
        composeTestRule.onNodeWithText("Friends").performClick()
        composeTestRule.onNodeWithText("Suggest").performClick()
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithText("Bob").assertIsDisplayed()
    }
}
