package com.mycorrhizal.crm.feature.timelineentities

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.ActivityRepository
import com.mycorrhizal.crm.domain.repository.ContactActivitiesPage
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.ConversationAgendaRepository
import com.mycorrhizal.crm.domain.repository.GiftRepository
import com.mycorrhizal.crm.domain.repository.LifeEventRepository
import com.mycorrhizal.crm.domain.repository.PreferenceRepository
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.ConversationAgenda
import com.mycorrhizal.crm.model.network.Gift
import com.mycorrhizal.crm.model.network.LifeEvent
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.model.network.Preference
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.mockk
import java.io.IOException
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

/**
 * #2938: with the contact cached (so the contact fetch succeeds) and the server then
 * unreachable, the LIST fetch fails. That must land in `loadError` (rendered as ErrorState +
 * Retry) with an empty list, not in the 4-second snackbar `error` field.
 */
class ListLoadErrorViewModelTest {
    @get:Rule val mainDispatcherRule = MainDispatcherRule()

    private val uid = "11111111-1111-1111-1111-111111111111"
    private val contacts = mockk<ContactRepository>().also {
        coEvery { it.getContact(5) } returns Result.success(
            ContactRecordResponse(id = 5, card = Card(uid = uid, name = Name(full = "Dana")))
        )
    }
    private val handle get() = SavedStateHandle(mapOf("contactId" to 5))

    private fun <T> offlineList(): Result<List<T>> = Result.failure(
        ApiError.Network(IOException("offline"))
    )

    @Test fun `life events list failure sets loadError and no snackbar error`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val repo = mockk<LifeEventRepository>()
            coEvery { repo.listForContact(uid) } returns offlineList()
            val vm = LifeEventsViewModel(repo, contacts, handle)
            advanceUntilIdle()
            val s = vm.uiState.value
            assertNotNull(s.loadError)
            assertNull(s.error)
            assertTrue(s.items.isEmpty())
        }

    @Test fun `preferences list failure sets loadError and no snackbar error`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val repo = mockk<PreferenceRepository>()
            coEvery { repo.listForContact(uid) } returns offlineList()
            val vm = PreferencesViewModel(repo, contacts, handle)
            advanceUntilIdle()
            val s = vm.uiState.value
            assertNotNull(s.loadError)
            assertNull(s.error)
            assertTrue(s.items.isEmpty())
        }

    @Test fun `gifts list failure sets loadError and no snackbar error`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val repo = mockk<GiftRepository>()
            val lifeEvents = mockk<LifeEventRepository>()
            val activities = mockk<ActivityRepository>()
            val prefs = mockk<PreferenceRepository>()
            coEvery { repo.listForContact(uid) } returns offlineList()
            coEvery { lifeEvents.listForContact(uid) } returns offlineList()
            coEvery { prefs.listForContact(uid) } returns offlineList()
            coEvery { activities.listForContact(5, any(), any(), any(), any(), any()) } returns
                Result.success(ContactActivitiesPage(activities = emptyList(), nextCursor = null))
            val vm = GiftsViewModel(repo, lifeEvents, activities, prefs, contacts, handle)
            advanceUntilIdle()
            val s = vm.uiState.value
            assertNotNull(s.loadError)
            assertNull(s.error)
            assertTrue(s.items.isEmpty())
        }

    @Test fun `agenda list failure sets loadError and no snackbar error`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val repo = mockk<ConversationAgendaRepository>()
            val activities = mockk<ActivityRepository>()
            coEvery { repo.listForContact(uid) } returns offlineList()
            coEvery { activities.listForContact(5, any(), any(), any(), any(), any()) } returns
                Result.success(ContactActivitiesPage(activities = emptyList(), nextCursor = null))
            val vm = ConversationAgendaViewModel(repo, activities, contacts, handle)
            advanceUntilIdle()
            val s = vm.uiState.value
            assertNotNull(s.loadError)
            assertNull(s.error)
            assertTrue(s.items.isEmpty())
        }

    // #2938: a FAILED REFRESH with a list already showing keeps the list and reports through the
    // snackbar `error` field; only a failed first load is the Retry screen.
    @Test fun `life events refresh failure keeps the list and uses the snackbar error`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val repo = mockk<LifeEventRepository>()
            coEvery { repo.listForContact(uid) } returnsMany listOf(
                Result.success(listOf(LifeEvent(id = "e1", entityId = uid, type = "moved"))),
                offlineList()
            )
            val vm = LifeEventsViewModel(repo, contacts, handle)
            advanceUntilIdle()
            vm.load()
            advanceUntilIdle()
            assertEquals(1, vm.uiState.value.items.size)
            assertNotNull(vm.uiState.value.error)
            assertNull(vm.uiState.value.loadError)
        }

    @Test fun `preferences refresh failure keeps the list and uses the snackbar error`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val repo = mockk<PreferenceRepository>()
            coEvery { repo.listForContact(uid) } returnsMany listOf(
                Result.success(
                    listOf(
                        Preference(id = "p1", entityId = uid, category = "hobby", value = "pottery")
                    )
                ),
                offlineList()
            )
            val vm = PreferencesViewModel(repo, contacts, handle)
            advanceUntilIdle()
            vm.load()
            advanceUntilIdle()
            assertEquals(1, vm.uiState.value.items.size)
            assertNotNull(vm.uiState.value.error)
            assertNull(vm.uiState.value.loadError)
        }

    @Test fun `gifts refresh failure keeps the list and uses the snackbar error`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val repo = mockk<GiftRepository>()
            val lifeEvents = mockk<LifeEventRepository>()
            val activities = mockk<ActivityRepository>()
            val prefs = mockk<PreferenceRepository>()
            coEvery { repo.listForContact(uid) } returnsMany listOf(
                Result.success(listOf(Gift(id = "g1", entityId = uid, description = "Socks"))),
                offlineList()
            )
            coEvery { lifeEvents.listForContact(uid) } returns Result.success(emptyList())
            coEvery { prefs.listForContact(uid) } returns Result.success(emptyList())
            coEvery { activities.listForContact(5, any(), any(), any(), any(), any()) } returns
                Result.success(ContactActivitiesPage(activities = emptyList(), nextCursor = null))
            val vm = GiftsViewModel(repo, lifeEvents, activities, prefs, contacts, handle)
            advanceUntilIdle()
            vm.load()
            advanceUntilIdle()
            assertEquals(1, vm.uiState.value.items.size)
            assertNotNull(vm.uiState.value.error)
            assertNull(vm.uiState.value.loadError)
        }

    @Test fun `agenda refresh failure keeps the list and uses the snackbar error`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val repo = mockk<ConversationAgendaRepository>()
            val activities = mockk<ActivityRepository>()
            coEvery { repo.listForContact(uid) } returnsMany listOf(
                Result.success(
                    listOf(ConversationAgenda(id = "a1", entityId = uid, content = "Ask"))
                ),
                offlineList()
            )
            coEvery { activities.listForContact(5, any(), any(), any(), any(), any()) } returns
                Result.success(ContactActivitiesPage(activities = emptyList(), nextCursor = null))
            val vm = ConversationAgendaViewModel(repo, activities, contacts, handle)
            advanceUntilIdle()
            vm.load()
            advanceUntilIdle()
            assertEquals(1, vm.uiState.value.items.size)
            assertNotNull(vm.uiState.value.error)
            assertNull(vm.uiState.value.loadError)
        }
}
