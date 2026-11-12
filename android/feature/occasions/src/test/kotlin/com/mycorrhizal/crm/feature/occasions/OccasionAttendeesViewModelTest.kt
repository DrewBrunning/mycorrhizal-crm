package com.mycorrhizal.crm.feature.occasions

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.CircleRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.ContactsPage
import com.mycorrhizal.crm.domain.repository.OccasionEventRepository
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.InviteeSuggestion
import com.mycorrhizal.crm.model.network.OccasionEvent
import com.mycorrhizal.crm.model.network.OccasionEventAttendee
import com.mycorrhizal.crm.model.network.OccasionEventAttendeeInput
import com.mycorrhizal.crm.model.network.OccasionEventAttendeeView
import com.mycorrhizal.crm.model.network.OccasionEventDetail
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.coVerifyOrder
import io.mockk.mockk
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class OccasionAttendeesViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val occasionRepository = mockk<OccasionEventRepository>()
    private val circleRepository = mockk<CircleRepository>()
    private val contactRepository = mockk<ContactRepository>()

    private fun viewModel(eventId: String = "e1") = OccasionAttendeesViewModel(
        occasionRepository,
        circleRepository,
        contactRepository,
        SavedStateHandle(mapOf("eventId" to eventId))
    )

    private val attendeeView = OccasionEventAttendeeView(
        id = "a1",
        eventId = "e1",
        entityId = "uid-1",
        contactId = 3,
        contactName = "Alice",
        rsvp = "pending"
    )

    private fun stubLoad(attendees: List<OccasionEventAttendeeView> = listOf(attendeeView)) {
        coEvery { occasionRepository.get("e1") } returns Result.success(
            OccasionEventDetail(
                occasionEvent = OccasionEvent(
                    id = "e1",
                    title = "BBQ",
                    startsAt = "2026-07-04T15:00:00Z"
                ),
                attendees = attendees
            )
        )
        coEvery { circleRepository.list(any(), any()) } returns Result.success(
            listOf(Circle(id = "c1", name = "Friends"))
        )
    }

    @Test
    fun `load populates attendees and the event title`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad()
        val vm = viewModel()
        advanceUntilIdle()

        assertEquals(1, vm.uiState.value.attendees.size)
        assertEquals("BBQ", vm.uiState.value.eventTitle)
        assertEquals(1, vm.uiState.value.circles.size)
    }

    @Test
    fun `a blank event id surfaces the missing-id error`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { circleRepository.list(any(), any()) } returns Result.success(emptyList())
        val vm = viewModel(eventId = "")
        advanceUntilIdle()

        assertTrue(vm.uiState.value.errorRes != null)
    }

    @Test
    fun `toggleCircle adds and removes selection`() = runTest(mainDispatcherRule.testDispatcher) {
        stubLoad()
        val vm = viewModel()
        advanceUntilIdle()

        vm.toggleCircle("c1")
        assertEquals(setOf("c1"), vm.uiState.value.selectedCircleIds)
        vm.toggleCircle("c1")
        assertEquals(emptySet<String>(), vm.uiState.value.selectedCircleIds)
    }

    @Test
    fun `suggest fetches candidates for the selected circles`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad()
        coEvery { occasionRepository.suggestInvitees(listOf("c1"), "e1") } returns Result.success(
            listOf(InviteeSuggestion(contactId = 5, contactName = "Bob", entityId = "uid-5"))
        )
        val vm = viewModel()
        advanceUntilIdle()

        vm.toggleCircle("c1")
        vm.suggest()
        advanceUntilIdle()

        assertEquals(1, vm.uiState.value.suggestions.size)
        coVerify { occasionRepository.suggestInvitees(listOf("c1"), "e1") }
    }

    private val threeSuggestions = listOf(
        InviteeSuggestion(contactId = 5, contactName = "Bob", entityId = "uid-5"),
        InviteeSuggestion(contactId = 6, contactName = "Cy", entityId = "uid-6"),
        InviteeSuggestion(contactId = 7, contactName = "Di", entityId = "uid-7")
    )

    private fun attendeeResult(uid: String) = Result.success(
        OccasionEventAttendee(id = "a-$uid", eventId = "e1", entityId = uid)
    )

    private suspend fun kotlinx.coroutines.test.TestScope.suggested(
        vm: OccasionAttendeesViewModel,
        suggestions: List<InviteeSuggestion>
    ) {
        coEvery { occasionRepository.suggestInvitees(listOf("c1"), "e1") } returns
            Result.success(suggestions)
        vm.toggleCircle("c1")
        vm.suggest()
        advanceUntilIdle()
    }

    @Test
    fun `addAllSuggested invites every suggestion in order, clears them and reloads`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad(attendees = emptyList())
        threeSuggestions.forEach {
            coEvery {
                occasionRepository.addAttendee("e1", OccasionEventAttendeeInput(it.entityId))
            } returns attendeeResult(it.entityId)
        }
        val vm = viewModel()
        advanceUntilIdle()
        suggested(vm, threeSuggestions)

        vm.addAllSuggested()
        advanceUntilIdle()

        coVerifyOrder {
            occasionRepository.addAttendee("e1", OccasionEventAttendeeInput("uid-5"))
            occasionRepository.addAttendee("e1", OccasionEventAttendeeInput("uid-6"))
            occasionRepository.addAttendee("e1", OccasionEventAttendeeInput("uid-7"))
        }
        coVerify(atLeast = 2) { occasionRepository.get("e1") }
        assertTrue(vm.uiState.value.suggestions.isEmpty())
        assertEquals(null, vm.uiState.value.error)
        assertFalse(vm.uiState.value.isMutating)
    }

    @Test
    fun `addAllSuggested stops at the first failure and keeps the unadded suggestions`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad(attendees = emptyList())
        coEvery {
            occasionRepository.addAttendee("e1", OccasionEventAttendeeInput("uid-5"))
        } returns attendeeResult("uid-5")
        coEvery {
            occasionRepository.addAttendee("e1", OccasionEventAttendeeInput("uid-6"))
        } returns Result.failure(ApiError.Client(409, "already invited"))
        val vm = viewModel()
        advanceUntilIdle()
        suggested(vm, threeSuggestions)

        vm.addAllSuggested()
        advanceUntilIdle()

        coVerify(exactly = 1) {
            occasionRepository.addAttendee("e1", OccasionEventAttendeeInput("uid-5"))
        }
        coVerify(exactly = 1) {
            occasionRepository.addAttendee("e1", OccasionEventAttendeeInput("uid-6"))
        }
        coVerify(exactly = 0) {
            occasionRepository.addAttendee("e1", OccasionEventAttendeeInput("uid-7"))
        }
        // The one that was added is gone; the failed one and the untried one stay for a retry.
        assertEquals(listOf("uid-6", "uid-7"), vm.uiState.value.suggestions.map { it.entityId })
        assertTrue(vm.uiState.value.error is ApiError.Client)
        assertFalse(vm.uiState.value.isMutating)
        // The roster is refreshed even though the batch failed part-way.
        coVerify(atLeast = 2) { occasionRepository.get("e1") }
    }

    @Test
    fun `addAllSuggested with no suggestions does nothing`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad()
        val vm = viewModel()
        advanceUntilIdle()

        vm.addAllSuggested()
        advanceUntilIdle()

        coVerify(exactly = 0) { occasionRepository.addAttendee(any(), any()) }
        coVerify(exactly = 1) { occasionRepository.get("e1") }
    }

    @Test
    fun `addAttendee invites and reloads`() = runTest(mainDispatcherRule.testDispatcher) {
        stubLoad(attendees = emptyList())
        coEvery {
            occasionRepository.addAttendee(
                "e1",
                OccasionEventAttendeeInput("uid-5")
            )
        } returns Result.success(
            OccasionEventAttendee(id = "a1", eventId = "e1", entityId = "uid-5")
        )
        val vm = viewModel()
        advanceUntilIdle()

        vm.addAttendee("uid-5")
        advanceUntilIdle()

        coVerify { occasionRepository.addAttendee("e1", OccasionEventAttendeeInput("uid-5")) }
        coVerify(atLeast = 2) { occasionRepository.get("e1") }
    }

    @Test
    fun `updateRsvp and removeAttendee call the repository`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad()
        coEvery {
            occasionRepository.updateAttendee(
                "e1",
                "uid-1",
                "accepted"
            )
        } returns Result.success(
            OccasionEventAttendee(id = "a1", eventId = "e1", entityId = "uid-1", rsvp = "accepted")
        )
        coEvery { occasionRepository.removeAttendee("e1", "uid-1") } returns Result.success(Unit)
        val vm = viewModel()
        advanceUntilIdle()

        vm.updateRsvp("uid-1", "accepted")
        advanceUntilIdle()
        vm.removeAttendee("uid-1")
        advanceUntilIdle()

        coVerify { occasionRepository.updateAttendee("e1", "uid-1", "accepted") }
        coVerify { occasionRepository.removeAttendee("e1", "uid-1") }
    }

    @Test
    fun `contact search debounces, excludes attendees, and clears on blank`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubLoad()
            coEvery {
                contactRepository.listContacts(
                    search = "bo",
                    limit = 25
                )
            } returns Result.success(
                ContactsPage(
                    contacts = listOf(
                        ContactSummary(id = 5, uid = "uid-5", firstname = "Bob"),
                        ContactSummary(id = 3, uid = "uid-1", firstname = "Alice")
                    ),
                    nextCursor = null,
                    limit = 25,
                    sync = null
                )
            )
            val vm = viewModel()
            advanceUntilIdle()

            vm.onContactQueryChange("bo")
            advanceTimeBy(350)
            advanceUntilIdle()

            // Alice (already attending uid-1) is filtered out.
            assertEquals(1, vm.uiState.value.contactResults.size)
            assertEquals("Bob", vm.uiState.value.contactResults[0].firstname)

            vm.onContactQueryChange("")
            assertEquals(0, vm.uiState.value.contactResults.size)
        }
}
