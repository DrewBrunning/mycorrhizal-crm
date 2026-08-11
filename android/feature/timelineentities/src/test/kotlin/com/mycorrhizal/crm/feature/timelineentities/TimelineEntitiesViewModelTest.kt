package com.mycorrhizal.crm.feature.timelineentities

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.GiftRepository
import com.mycorrhizal.crm.domain.repository.LifeEventRepository
import com.mycorrhizal.crm.domain.repository.PreferenceRepository
import com.mycorrhizal.crm.domain.repository.ConversationAgendaRepository
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.Gift
import com.mycorrhizal.crm.model.network.LifeEvent
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.model.network.Preference
import com.mycorrhizal.crm.model.network.ConversationAgenda
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test

private const val UID = "11111111-1111-1111-1111-111111111111"

private fun stubContact(contactRepository: ContactRepository) {
    coEvery { contactRepository.getContact(5) } returns Result.success(
        ContactRecordResponse(id = 5, card = Card(uid = UID, name = Name(full = "Dana White"))),
    )
}

class LifeEventsViewModelTest {
    @get:Rule val mainDispatcherRule = MainDispatcherRule()
    private val repo = mockk<LifeEventRepository>()
    private val contacts = mockk<ContactRepository>()
    private fun vm() = LifeEventsViewModel(repo, contacts, SavedStateHandle(mapOf("contactId" to 5)))

    @Test fun `loads life events after resolving the uid`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact(contacts)
        coEvery { repo.listForContact(UID) } returns Result.success(
            listOf(LifeEvent(id = "e1", entityId = UID, type = "moved", description = "Moved to Madison")),
        )
        val vm = vm(); advanceUntilIdle()
        assertFalse(vm.uiState.value.isLoading)
        assertEquals(UID, vm.uiState.value.entityId)
        assertEquals(1, vm.uiState.value.items.size)
        assertNull(vm.uiState.value.error)
    }

    @Test fun `create posts the entity id`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact(contacts)
        coEvery { repo.listForContact(UID) } returns Result.success(emptyList())
        coEvery {
            repo.create(match { it.entityId == UID && it.type == "moved" && it.description == "Moved" })
        } returns Result.success(LifeEvent(id = "e1", entityId = UID, type = "moved"))
        val vm = vm(); advanceUntilIdle()
        vm.create("moved", "Moved"); advanceUntilIdle()
        coVerify { repo.create(match { it.entityId == UID }) }
    }
}

class GiftsViewModelTest {
    @get:Rule val mainDispatcherRule = MainDispatcherRule()
    private val repo = mockk<GiftRepository>()
    private val contacts = mockk<ContactRepository>()
    private fun vm() = GiftsViewModel(repo, contacts, SavedStateHandle(mapOf("contactId" to 5)))

    @Test fun `loads gifts and deletes by id`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact(contacts)
        coEvery { repo.listForContact(UID) } returns Result.success(
            listOf(Gift(id = "g1", entityId = UID, description = "Socks")),
        )
        val vm = vm(); advanceUntilIdle()
        assertEquals(1, vm.uiState.value.items.size)
        assertEquals("Socks", vm.uiState.value.items[0].label)

        coEvery { repo.listForContact(UID) } returns Result.success(emptyList())
        coEvery { repo.delete("g1") } returns Result.success(Unit)
        vm.delete("g1"); advanceUntilIdle()
        assertEquals(0, vm.uiState.value.items.size)
        coVerify { repo.delete("g1") }
    }

    @Test fun `missing contact id surfaces an error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { contacts.getContact(0) } returns Result.failure(ApiError.Client(404, "gone"))
        val vm = GiftsViewModel(repo, contacts, SavedStateHandle(mapOf("contactId" to 0)))
        advanceUntilIdle()
        assertFalse(vm.uiState.value.isLoading)
    }
}

class PreferencesViewModelTest {
    @get:Rule val mainDispatcherRule = MainDispatcherRule()
    private val repo = mockk<PreferenceRepository>()
    private val contacts = mockk<ContactRepository>()
    private fun vm() = PreferencesViewModel(repo, contacts, SavedStateHandle(mapOf("contactId" to 5)))

    @Test fun `loads preferences`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact(contacts)
        coEvery { repo.listForContact(UID) } returns Result.success(
            listOf(Preference(id = "p1", entityId = UID, category = "food", key = "allergy", value = "peanuts")),
        )
        val vm = vm(); advanceUntilIdle()
        assertEquals(1, vm.uiState.value.items.size)
        assertEquals("food: allergy = peanuts", vm.uiState.value.items[0].label)
    }
}

class ConversationAgendaViewModelTest {
    @get:Rule val mainDispatcherRule = MainDispatcherRule()
    private val repo = mockk<ConversationAgendaRepository>()
    private val contacts = mockk<ContactRepository>()
    private fun vm() = ConversationAgendaViewModel(repo, contacts, SavedStateHandle(mapOf("contactId" to 5)))

    @Test fun `loads agenda items`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact(contacts)
        coEvery { repo.listForContact(UID) } returns Result.success(
            listOf(ConversationAgenda(id = "a1", entityId = UID, content = "Ask about the move")),
        )
        val vm = vm(); advanceUntilIdle()
        assertEquals("Ask about the move", vm.uiState.value.items[0].label)
    }
}
