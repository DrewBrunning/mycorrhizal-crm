package com.mycorrhizal.crm.feature.contacts.entities

import com.mycorrhizal.crm.domain.repository.EntityRepository
import com.mycorrhizal.crm.model.network.Entity
import com.mycorrhizal.crm.model.network.EntityFromContactsCreated
import com.mycorrhizal.crm.model.network.EntityFromContactsInput
import com.mycorrhizal.crm.model.network.EntityFromContactsResponse
import com.mycorrhizal.crm.model.network.EntityFromContactsSkip
import com.mycorrhizal.crm.model.network.EntityFromContactsSkipReason
import com.mycorrhizal.crm.model.network.EntityKind
import com.mycorrhizal.crm.model.network.EntitySuggestionsResponse
import com.mycorrhizal.crm.model.network.EntityValueGroup
import com.mycorrhizal.crm.model.network.EntityValueGroupContact
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import io.mockk.slot
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

/** #2513 (web #2301): the create-entities-from-contacts helper. */
class EntitiesFromContactsViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val repository = mockk<EntityRepository>()

    private fun contact(id: Int, first: String, primary: Boolean = false) = EntityValueGroupContact(
        contactId = id,
        firstname = first,
        lastname = "Lovelace",
        valueName = "ACME",
        hasPrimary = primary
    )

    private val acme = EntityValueGroup(
        key = "k-acme",
        kind = EntityKind.ORGANISATION,
        name = "Acme",
        reason = "exact_name",
        variants = listOf("Acme", "ACME"),
        contacts = listOf(contact(7, "Ada", primary = true), contact(8, "Bo"), contact(9, "Cy"))
    )
    private val globex = EntityValueGroup(
        key = "k-globex",
        kind = EntityKind.ORGANISATION,
        name = "Globex",
        reason = "normalized_name",
        contacts = listOf(contact(10, "Di"))
    )
    private val cabin = EntityValueGroup(
        key = "k-cabin",
        kind = EntityKind.PLACE,
        name = "Cabin",
        reason = "address_components",
        contacts = listOf(contact(11, "Ed"))
    )

    private fun groups(vararg g: EntityValueGroup, next: String? = null) = Result.success(
        EntitySuggestionsResponse(mode = "contact_values", groups = g.toList(), nextCursor = next)
    )

    private fun stubGroups() {
        coEvery { repository.valueGroups(EntityKind.ORGANISATION, null) } returns
            groups(acme, globex)
        coEvery { repository.valueGroups(EntityKind.PLACE, null) } returns groups(cabin)
    }

    private fun vm(): EntitiesFromContactsViewModel {
        stubGroups()
        return EntitiesFromContactsViewModel(repository)
    }

    @Test
    fun `loads organisation groups with nothing selected and the first role by default`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = vm()
            advanceUntilIdle()
            val state = vm.uiState.value
            assertEquals(EntityKind.ORGANISATION, state.kind)
            assertEquals(listOf("k-acme", "k-globex"), state.groups.map { it.key })
            assertTrue(state.loaded)
            assertFalse(state.isLoading)
            val edit = state.edit(acme)
            assertFalse(edit.selected)
            assertEquals("Acme", edit.name)
            assertEquals("employee", edit.role)
            assertFalse(edit.setPrimary)
            assertEquals(0, state.chosenCount)
        }

    @Test
    fun `selecting the place kind reloads with place roles and clears the previous result`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = vm()
            advanceUntilIdle()
            vm.selectKind(EntityKind.PLACE)
            advanceUntilIdle()
            val state = vm.uiState.value
            assertEquals(EntityKind.PLACE, state.kind)
            assertEquals(listOf("k-cabin"), state.groups.map { it.key })
            assertEquals("resident", state.edit(cabin).role)
            assertNull(state.result)
        }

    @Test
    fun `a kind without a helper is ignored`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = vm()
        advanceUntilIdle()
        vm.selectKind(EntityKind.PET)
        advanceUntilIdle()
        assertEquals(EntityKind.ORGANISATION, vm.uiState.value.kind)
        coVerify(exactly = 1) { repository.valueGroups(EntityKind.ORGANISATION, null) }
    }

    @Test
    fun `a slow response for the previous kind never overwrites the current one`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val slow = CompletableDeferred<Result<EntitySuggestionsResponse>>()
            coEvery { repository.valueGroups(EntityKind.ORGANISATION, null) } coAnswers
                { slow.await() }
            coEvery { repository.valueGroups(EntityKind.PLACE, null) } returns groups(cabin)
            val vm = EntitiesFromContactsViewModel(repository)
            vm.selectKind(EntityKind.PLACE)
            advanceUntilIdle()
            slow.complete(groups(acme))
            advanceUntilIdle()
            assertEquals(listOf("k-cabin"), vm.uiState.value.groups.map { it.key })
            assertFalse(vm.uiState.value.isLoading)
        }

    @Test
    fun `load more appends the next page and keeps edits already made`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.valueGroups(EntityKind.ORGANISATION, null) } returns
                groups(acme, next = "c2")
            coEvery { repository.valueGroups(EntityKind.ORGANISATION, "c2") } returns
                groups(globex)
            val vm = EntitiesFromContactsViewModel(repository)
            advanceUntilIdle()
            assertEquals("c2", vm.uiState.value.nextCursor)
            vm.setSelected("k-acme", true)
            vm.loadMore()
            advanceUntilIdle()
            val state = vm.uiState.value
            assertEquals(listOf("k-acme", "k-globex"), state.groups.map { it.key })
            assertNull(state.nextCursor)
            assertTrue(state.edit(acme).selected)
            assertFalse(state.edit(globex).selected)
        }

    @Test
    fun `a load failure keeps the error and retry recovers`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.valueGroups(EntityKind.ORGANISATION, null) } returnsMany listOf(
                Result.failure(ApiError.Network(java.io.IOException("offline"))),
                groups(acme)
            )
            val vm = EntitiesFromContactsViewModel(repository)
            advanceUntilIdle()
            assertNotNull(vm.uiState.value.loadError)
            assertFalse(vm.uiState.value.loaded)
            vm.refresh()
            advanceUntilIdle()
            assertNull(vm.uiState.value.loadError)
            assertEquals(listOf("k-acme"), vm.uiState.value.groups.map { it.key })
        }

    @Test
    fun `per-group choices are tracked and only included contacts count`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = vm()
            advanceUntilIdle()
            vm.setSelected("k-acme", true)
            vm.setName("k-acme", "Acme Inc")
            vm.setRole("k-acme", "owner")
            vm.setPrimary("k-acme", true)
            vm.setContactIncluded("k-acme", 8, false)
            val state = vm.uiState.value
            val edit = state.edit(acme)
            assertTrue(edit.selected)
            assertEquals("Acme Inc", edit.name)
            assertEquals("owner", edit.role)
            assertTrue(edit.setPrimary)
            assertEquals(setOf(8), edit.excluded)
            assertEquals(1, state.chosenCount)
            vm.setContactIncluded("k-acme", 8, true)
            assertTrue(vm.uiState.value.edit(acme).excluded.isEmpty())
        }

    @Test
    fun `a name longer than the server limit is cut`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = vm()
            advanceUntilIdle()
            vm.setName("k-acme", "x".repeat(250))
            assertEquals(200, vm.uiState.value.edit(acme).name.length)
        }

    @Test
    fun `create posts only chosen groups with the chosen contacts, role, primary and name`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = vm()
            advanceUntilIdle()
            val sent = slot<EntityFromContactsInput>()
            coEvery { repository.createFromContacts(capture(sent)) } returns Result.success(
                EntityFromContactsResponse(
                    created = listOf(
                        EntityFromContactsCreated(
                            key = "k-acme",
                            entity = Entity(id = "e1", name = "Acme Inc")
                        )
                    )
                )
            )
            vm.setSelected("k-acme", true)
            vm.setName("k-acme", "  Acme Inc  ")
            vm.setRole("k-acme", "owner")
            vm.setPrimary("k-acme", true)
            vm.setContactIncluded("k-acme", 8, false)
            vm.create()
            advanceUntilIdle()

            assertEquals(EntityKind.ORGANISATION, sent.captured.kind)
            val group = sent.captured.groups.single()
            assertEquals("k-acme", group.key)
            assertEquals("Acme Inc", group.name)
            assertEquals("owner", group.role)
            assertTrue(group.setPrimary)
            assertEquals(listOf(7, 9), group.contactIds)
        }

    @Test
    fun `a blank name falls back to the proposed one`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = vm()
            advanceUntilIdle()
            val sent = slot<EntityFromContactsInput>()
            coEvery { repository.createFromContacts(capture(sent)) } returns
                Result.success(EntityFromContactsResponse())
            vm.setSelected("k-globex", true)
            vm.setName("k-globex", "   ")
            vm.create()
            advanceUntilIdle()
            assertEquals("Globex", sent.captured.groups.single().name)
        }

    @Test
    fun `a group whose contacts are all unticked is not sent and nothing is posted when none remain`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = vm()
            advanceUntilIdle()
            vm.setSelected("k-globex", true)
            vm.setContactIncluded("k-globex", 10, false)
            assertEquals(0, vm.uiState.value.chosenCount)
            vm.create()
            advanceUntilIdle()
            coVerify(exactly = 0) { repository.createFromContacts(any()) }
        }

    @Test
    fun `success shows the results summary, resolves skip names and rescans`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.valueGroups(EntityKind.ORGANISATION, null) } returnsMany listOf(
                groups(acme, globex),
                groups(globex)
            )
            val vm = EntitiesFromContactsViewModel(repository)
            advanceUntilIdle()
            coEvery { repository.createFromContacts(any()) } returns Result.success(
                EntityFromContactsResponse(
                    created = listOf(
                        EntityFromContactsCreated(
                            key = "k-acme",
                            entity = Entity(id = "e1", name = "Acme")
                        )
                    ),
                    skipped = listOf(
                        EntityFromContactsSkip(
                            key = "k-globex",
                            reason = EntityFromContactsSkipReason.GROUP_NOT_FOUND
                        ),
                        EntityFromContactsSkip(
                            key = "k-acme",
                            contactId = 8,
                            reason = EntityFromContactsSkipReason.CONTACT_NOT_IN_GROUP
                        ),
                        EntityFromContactsSkip(key = "gone", reason = "a_future_reason")
                    )
                )
            )
            vm.setSelected("k-acme", true)
            vm.setSelected("k-globex", true)
            vm.create()
            advanceUntilIdle()

            val state = vm.uiState.value
            assertFalse(state.isCreating)
            assertNull(state.createError)
            assertNotNull(state.result)
            val result = state.result!!
            assertEquals(EntityKind.ORGANISATION, result.kind)
            assertEquals(listOf("Acme"), result.createdNames)
            assertEquals(1, result.createdCount)
            assertEquals(
                listOf("Globex", "Acme", "gone"),
                result.skipped.map { it.groupName }
            )
            assertEquals(listOf(null, "Bo Lovelace", null), result.skipped.map { it.contactName })
            assertEquals("a_future_reason", result.skipped[2].reason)
            // Resumable: only the still-unmatched group is offered again, unselected.
            assertEquals(listOf("k-globex"), state.groups.map { it.key })
            assertFalse(state.edit(globex).selected)
            assertEquals(0, state.chosenCount)
        }

    @Test
    fun `a failed create keeps every choice, reports the error and shows no summary`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = vm()
            advanceUntilIdle()
            val failure = ApiError.Network(java.io.IOException("offline"))
            coEvery { repository.createFromContacts(any()) } returns Result.failure(failure)
            vm.setSelected("k-acme", true)
            vm.setName("k-acme", "Mine")
            vm.create()
            advanceUntilIdle()
            val state = vm.uiState.value
            assertEquals(failure, state.createError)
            assertFalse(state.isCreating)
            assertNull(state.result)
            assertTrue(state.edit(acme).selected)
            assertEquals("Mine", state.edit(acme).name)
            // Nothing was rescanned: the groups on screen are still the ones the user chose.
            coVerify(exactly = 1) { repository.valueGroups(EntityKind.ORGANISATION, null) }

            // Editing clears the stale error; a retry can then succeed.
            vm.setName("k-acme", "Mine 2")
            assertNull(vm.uiState.value.createError)
        }

    @Test
    fun `a second create while one is in flight is ignored`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = vm()
            advanceUntilIdle()
            val slow = CompletableDeferred<Result<EntityFromContactsResponse>>()
            coEvery { repository.createFromContacts(any()) } coAnswers { slow.await() }
            vm.setSelected("k-acme", true)
            vm.create()
            vm.create()
            advanceUntilIdle()
            assertTrue(vm.uiState.value.isCreating)
            slow.complete(Result.success(EntityFromContactsResponse()))
            advanceUntilIdle()
            coVerify(exactly = 1) { repository.createFromContacts(any()) }
        }

    @Test
    fun `dismissing the summary clears it`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = vm()
        advanceUntilIdle()
        coEvery { repository.createFromContacts(any()) } returns
            Result.success(EntityFromContactsResponse())
        vm.setSelected("k-acme", true)
        vm.create()
        advanceUntilIdle()
        assertNotNull(vm.uiState.value.result)
        vm.dismissResult()
        assertNull(vm.uiState.value.result)
    }
}
