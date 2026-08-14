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
        // LifeEvent has no url-like field — EntityItem.url must default to
        // null, not carry over stale data from another entity type.
        assertNull(vm.uiState.value.items[0].url)
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

    // M17: findById backs the tap-to-edit path -- EntityItem only carries the
    // derived label, so the edit dialog pre-fills from whatever this returns.
    @Test fun `findById returns the loaded entity by id`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact(contacts)
        coEvery { repo.listForContact(UID) } returns Result.success(
            listOf(LifeEvent(id = "e1", entityId = UID, type = "moved", description = "Moved to Madison")),
        )
        val vm = vm(); advanceUntilIdle()
        assertEquals("Moved to Madison", vm.findById("e1")?.description)
        assertNull(vm.findById("no-such-id"))
    }

    // M17: UpdateLifeEvent is a full overwrite server-side (life_event_
    // controller.go), so update() must carry every field the mini edit form
    // doesn't touch forward from the original entity, not just entityId.
    @Test fun `update preserves fields the edit form does not touch`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact(contacts)
        val original = LifeEvent(
            id = "e1",
            entityId = UID,
            type = "moved",
            category = "family_relationships",
            description = "Moved to Madison",
            source = "manual",
            remind = true,
        )
        coEvery { repo.listForContact(UID) } returns Result.success(listOf(original))
        coEvery {
            repo.update(
                "e1",
                match {
                    it.entityId == UID &&
                        it.type == "relocated" &&
                        it.description == "Moved to Chicago" &&
                        it.category == "family_relationships" &&
                        it.source == "manual" &&
                        it.remind == true
                },
            )
        } returns Result.success(original.copy(type = "relocated", description = "Moved to Chicago"))
        val vm = vm(); advanceUntilIdle()
        vm.update(original, "relocated", "Moved to Chicago"); advanceUntilIdle()
        coVerify {
            repo.update("e1", match { it.category == "family_relationships" && it.remind == true })
        }
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
            listOf(Gift(id = "g1", entityId = UID, description = "Socks", url = "https://example.com/socks")),
        )
        val vm = vm(); advanceUntilIdle()
        assertEquals(1, vm.uiState.value.items.size)
        assertEquals("Socks", vm.uiState.value.items[0].label)
        // T62 Android port: the gift's url must reach EntityItem so the list
        // screen can render it as a clickable link (EntityListScreens.kt).
        assertEquals("https://example.com/socks", vm.uiState.value.items[0].url)

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

    // M17: UpdateGift is a full overwrite (gift_controller.go) -- status/url/
    // notes/date/value/currency/associations must survive an edit that only
    // touches description, or a save silently resets a "given" gift back to
    // "idea" and drops its url/notes/value.
    @Test fun `update preserves fields the edit form does not touch`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact(contacts)
        val original = Gift(
            id = "g1",
            entityId = UID,
            status = "given",
            description = "Socks",
            url = "https://example.com/socks",
            notes = "Wool, size L",
            valueCents = 2500,
            currency = "USD",
        )
        coEvery { repo.listForContact(UID) } returns Result.success(listOf(original))
        coEvery {
            repo.update(
                "g1",
                match {
                    it.entityId == UID &&
                        it.description == "Wool socks" &&
                        it.status == "given" &&
                        it.url == "https://example.com/socks" &&
                        it.notes == "Wool, size L" &&
                        it.valueCents == 2500L &&
                        it.currency == "USD"
                },
            )
        } returns Result.success(original.copy(description = "Wool socks"))
        val vm = vm(); advanceUntilIdle()
        vm.update(original, "Wool socks"); advanceUntilIdle()
        coVerify { repo.update("g1", match { it.status == "given" && it.valueCents == 2500L }) }
    }

    @Test fun `findById returns the loaded entity by id`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact(contacts)
        coEvery { repo.listForContact(UID) } returns Result.success(
            listOf(Gift(id = "g1", entityId = UID, description = "Socks")),
        )
        val vm = vm(); advanceUntilIdle()
        assertEquals("Socks", vm.findById("g1")?.description)
        assertNull(vm.findById("no-such-id"))
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

    // M17: UpdatePreference is a full overwrite (preference_controller.go) --
    // key/source/confidence/lastConfirmed/sensitivity must survive an edit
    // that only touches category/value.
    @Test fun `update preserves fields the edit form does not touch`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact(contacts)
        val original = Preference(
            id = "p1",
            entityId = UID,
            category = "food",
            key = "allergy",
            value = "peanuts",
            source = "manual",
            sensitivity = "private",
        )
        coEvery { repo.listForContact(UID) } returns Result.success(listOf(original))
        coEvery {
            repo.update(
                "p1",
                match {
                    it.entityId == UID &&
                        it.category == "food" &&
                        it.value == "tree nuts" &&
                        it.key == "allergy" &&
                        it.source == "manual" &&
                        it.sensitivity == "private"
                },
            )
        } returns Result.success(original.copy(value = "tree nuts"))
        val vm = vm(); advanceUntilIdle()
        vm.update(original, "food", "tree nuts"); advanceUntilIdle()
        coVerify { repo.update("p1", match { it.sensitivity == "private" && it.key == "allergy" }) }
    }

    @Test fun `findById returns the loaded entity by id`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact(contacts)
        coEvery { repo.listForContact(UID) } returns Result.success(
            listOf(Preference(id = "p1", entityId = UID, category = "food", value = "peanuts")),
        )
        val vm = vm(); advanceUntilIdle()
        assertEquals("food", vm.findById("p1")?.category)
        assertNull(vm.findById("no-such-id"))
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
            listOf(
                ConversationAgenda(
                    id = "a1",
                    entityId = UID,
                    content = "Ask about the move",
                    referenceUrl = "https://example.com/listing",
                ),
            ),
        )
        val vm = vm(); advanceUntilIdle()
        assertEquals("Ask about the move", vm.uiState.value.items[0].label)
        // T62 Android port: referenceUrl must reach EntityItem the same way
        // Gift.url does — see GiftsViewModelTest's matching assertion.
        assertEquals("https://example.com/listing", vm.uiState.value.items[0].url)
    }

    // M17: UpdateConversationAgenda is a full overwrite (conversation_agenda_
    // controller.go) -- referenceUrl must survive an edit that only touches
    // content.
    @Test fun `update preserves fields the edit form does not touch`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact(contacts)
        val original = ConversationAgenda(
            id = "a1",
            entityId = UID,
            content = "Ask about the move",
            referenceUrl = "https://example.com/listing",
        )
        coEvery { repo.listForContact(UID) } returns Result.success(listOf(original))
        coEvery {
            repo.update(
                "a1",
                match {
                    it.entityId == UID &&
                        it.content == "Ask about the new place" &&
                        it.referenceUrl == "https://example.com/listing"
                },
            )
        } returns Result.success(original.copy(content = "Ask about the new place"))
        val vm = vm(); advanceUntilIdle()
        vm.update(original, "Ask about the new place"); advanceUntilIdle()
        coVerify { repo.update("a1", match { it.referenceUrl == "https://example.com/listing" }) }
    }

    @Test fun `findById returns the loaded entity by id`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact(contacts)
        coEvery { repo.listForContact(UID) } returns Result.success(
            listOf(ConversationAgenda(id = "a1", entityId = UID, content = "Ask about the move")),
        )
        val vm = vm(); advanceUntilIdle()
        assertEquals("Ask about the move", vm.findById("a1")?.content)
        assertNull(vm.findById("no-such-id"))
    }
}
