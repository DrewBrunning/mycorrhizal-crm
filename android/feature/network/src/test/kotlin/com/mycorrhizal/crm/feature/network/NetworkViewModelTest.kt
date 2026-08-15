package com.mycorrhizal.crm.feature.network

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.CircleWithMembers
import com.mycorrhizal.crm.domain.repository.GraphRepository
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.GraphChain
import com.mycorrhizal.crm.model.network.GraphChainStep
import com.mycorrhizal.crm.model.network.GraphConnectionsResponse
import com.mycorrhizal.crm.model.network.Name
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
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class NetworkViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val graphRepository = mockk<GraphRepository>()
    private val contactRepository = mockk<ContactRepository>()

    private fun viewModel(contactId: Int? = null): NetworkViewModel {
        val handle = if (contactId != null) {
            SavedStateHandle(mapOf("contactId" to contactId))
        } else {
            SavedStateHandle()
        }
        return NetworkViewModel(graphRepository, contactRepository, handle)
    }

    private fun record(id: Int, uid: String, name: String) = ContactRecordResponse(
        id = id,
        card = Card(uid = uid, name = Name(full = name)),
    )

    private fun chain(
        targetId: Int,
        uid: String,
        name: String,
        depth: Int,
        steps: List<GraphChainStep>,
    ) = GraphChain(targetId = targetId, targetVCardUid = uid, targetName = name, depth = depth, steps = steps)

    private fun stubFrom(contactId: Int = 1, uid: String = "uid-1", name: String = "Alice") {
        coEvery { contactRepository.getContact(contactId) } returns Result.success(record(contactId, uid, name))
    }

    private fun stubConnections(
        from: String = "uid-1",
        response: GraphConnectionsResponse = GraphConnectionsResponse(chains = emptyList()),
    ) {
        coEvery { graphRepository.getConnections(from = from, depth = any(), relation = any()) } returns
            Result.success(response)
    }

    @Test
    fun `loads connections for the initial contact and groups them by depth`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubFrom()
            coEvery { graphRepository.circlesWithMembers() } returns Result.success(emptyList())
            val response = GraphConnectionsResponse(
                fromVCardUid = "uid-1",
                fromName = "Alice",
                depth = 2,
                chains = listOf(
                    chain(10, "t1", "Carol", depth = 1, steps = listOf(GraphChainStep(10, "t1", "Carol", "child_of"))),
                    chain(20, "t2", "Dave", depth = 2, steps = listOf(GraphChainStep(20, "t2", "Dave", "spouse_of"))),
                    chain(30, "t3", "Eve", depth = 1, steps = listOf(GraphChainStep(30, "t3", "Eve", "sibling_of"))),
                ),
            )
            stubConnections(response = response)

            val vm = viewModel(contactId = 1)
            advanceUntilIdle()

            val state = vm.uiState.value
            assertFalse(state.isLoading)
            assertEquals("uid-1", state.fromVCardUid)
            assertEquals(listOf(1, 2), state.groupedChains.keys.sorted())
            assertEquals(listOf("Carol", "Eve"), state.groupedChains[1]?.map { it.targetName })
            assertEquals(listOf("Dave"), state.groupedChains[2]?.map { it.targetName })
            assertNull(state.error)
            coVerify { graphRepository.getConnections(from = "uid-1", depth = 2, relation = null) }
        }

    @Test
    fun `defaults to the self contact when started from the drawer`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { graphRepository.circlesWithMembers() } returns Result.success(emptyList())
            coEvery { graphRepository.selfContactVCardUid() } returns Result.success("self-uid")
            coEvery { contactRepository.resolveByUid(listOf("self-uid")) } returns Result.success(
                mapOf("self-uid" to ContactSummary(id = 9, uid = "self-uid", fn = "Me")),
            )
            stubConnections(from = "self-uid")

            val vm = viewModel(contactId = null)
            advanceUntilIdle()

            val state = vm.uiState.value
            assertFalse(state.isLoading)
            assertEquals("self-uid", state.fromVCardUid)
            assertEquals("Me", state.fromName)
            coVerify { graphRepository.getConnections(from = "self-uid", depth = 2, relation = null) }
        }

    @Test
    fun `empty graph yields an empty list without an error`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubFrom()
            coEvery { graphRepository.circlesWithMembers() } returns Result.success(emptyList())
            // chains absent AND [] both decode to an empty list (trap #8).
            stubConnections(response = GraphConnectionsResponse(chains = null))

            val vm = viewModel(contactId = 1)
            advanceUntilIdle()

            assertFalse(vm.uiState.value.isLoading)
            assertTrue(vm.uiState.value.allChains.isEmpty())
            assertTrue(vm.uiState.value.groupedChains.isEmpty())
            assertNull(vm.uiState.value.error)
        }

    @Test
    fun `changing the depth re-fetches with the new depth and keeps the relation filter`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubFrom()
            coEvery { graphRepository.circlesWithMembers() } returns Result.success(emptyList())
            stubConnections()
            coEvery { graphRepository.getConnections(from = "uid-1", depth = 3, relation = "brother") } returns
                Result.success(GraphConnectionsResponse(chains = emptyList()))

            val vm = viewModel(contactId = 1)
            advanceUntilIdle()
            vm.onRelationInputChange("brother")
            vm.applyRelation()
            advanceUntilIdle()
            coVerify { graphRepository.getConnections(from = "uid-1", depth = 2, relation = "brother") }

            vm.setDepth(3)
            advanceUntilIdle()

            assertEquals(3, vm.uiState.value.depth)
            coVerify { graphRepository.getConnections(from = "uid-1", depth = 3, relation = "brother") }
        }

    @Test
    fun `relation filter is passed through verbatim, never resolved`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubFrom()
            coEvery { graphRepository.circlesWithMembers() } returns Result.success(emptyList())
            stubConnections()
            coEvery { graphRepository.getConnections(from = "uid-1", depth = 2, relation = "brother") } returns
                Result.success(GraphConnectionsResponse(chains = emptyList()))

            val vm = viewModel(contactId = 1)
            advanceUntilIdle()

            vm.onRelationInputChange("brother")
            vm.applyRelation()
            advanceUntilIdle()

            assertEquals("brother", vm.uiState.value.appliedRelation)
            coVerify { graphRepository.getConnections(from = "uid-1", depth = 2, relation = "brother") }
        }

    @Test
    fun `clearing the relation filter drops it from the request`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubFrom()
            coEvery { graphRepository.circlesWithMembers() } returns Result.success(emptyList())
            stubConnections()

            val vm = viewModel(contactId = 1)
            advanceUntilIdle()
            vm.onRelationInputChange("brother")
            vm.applyRelation()
            advanceUntilIdle()

            vm.onRelationInputChange("  ")
            vm.applyRelation()
            advanceUntilIdle()

            assertNull(vm.uiState.value.appliedRelation)
            coVerify { graphRepository.getConnections(from = "uid-1", depth = 2, relation = null) }
        }

    @Test
    fun `circle filter narrows the chains client-side`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubFrom()
            coEvery { graphRepository.circlesWithMembers() } returns Result.success(
                listOf(CircleWithMembers(id = "c1", name = "Family", memberVCardUids = setOf("t1"))),
            )
            stubConnections(
                response = GraphConnectionsResponse(
                    chains = listOf(
                        chain(10, "t1", "Carol", depth = 1, steps = listOf(GraphChainStep(10, "t1", "Carol", "child_of"))),
                        chain(20, "t2", "Dave", depth = 1, steps = listOf(GraphChainStep(20, "t2", "Dave", "spouse_of"))),
                    ),
                ),
            )

            val vm = viewModel(contactId = 1)
            advanceUntilIdle()
            assertEquals(2, vm.uiState.value.allChains.size)

            vm.selectCircle("c1")
            advanceUntilIdle()

            assertEquals(listOf("Carol"), vm.uiState.value.filteredChains.map { it.targetName })
            // "All circles" restores the full set.
            vm.selectCircle(null)
            assertEquals(2, vm.uiState.value.filteredChains.size)
        }

    @Test
    fun `a contact without a vcard uid errors without firing a request`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { contactRepository.getContact(1) } returns Result.success(
                ContactRecordResponse(id = 1, card = Card(name = Name(full = "No UID"))),
            )
            coEvery { graphRepository.circlesWithMembers() } returns Result.success(emptyList())

            val vm = viewModel(contactId = 1)
            advanceUntilIdle()

            assertFalse(vm.uiState.value.isLoading)
            assertTrue(vm.uiState.value.errorRes != null)
            assertTrue(vm.uiState.value.hasFrom.not())
            coVerify(exactly = 0) { graphRepository.getConnections(any(), any(), any()) }
        }

    @Test
    fun `search results exclude the current from contact`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubFrom(uid = "uid-1", name = "Alice")
            coEvery { graphRepository.circlesWithMembers() } returns Result.success(emptyList())
            stubConnections()
            coEvery {
                contactRepository.listContacts(search = "bo", limit = 25)
            } returns Result.success(
                com.mycorrhizal.crm.domain.repository.ContactsPage(
                    contacts = listOf(
                        ContactSummary(id = 1, uid = "uid-1", fn = "Alice"),
                        ContactSummary(id = 2, uid = "uid-2", fn = "Bob"),
                    ),
                    nextCursor = null,
                    limit = 25,
                    sync = null,
                ),
            )

            val vm = viewModel(contactId = 1)
            advanceUntilIdle()

            vm.searchContacts("bo")
            advanceUntilIdle()

            assertEquals(listOf("Bob"), vm.uiState.value.contactSearchResults.map { it.displayName })
        }

    @Test
    fun `selecting a from contact reloads connections with the new uid`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubFrom()
            coEvery { graphRepository.circlesWithMembers() } returns Result.success(emptyList())
            stubConnections()
            coEvery { graphRepository.getConnections(from = "uid-2", depth = 2, relation = null) } returns
                Result.success(GraphConnectionsResponse(chains = emptyList()))

            val vm = viewModel(contactId = 1)
            advanceUntilIdle()

            vm.selectFrom(ContactSummary(id = 2, uid = "uid-2", fn = "Bob"))
            advanceUntilIdle()

            assertEquals("uid-2", vm.uiState.value.fromVCardUid)
            assertEquals("Bob", vm.uiState.value.fromName)
            assertFalse(vm.uiState.value.pickerOpen)
            coVerify { graphRepository.getConnections(from = "uid-2", depth = 2, relation = null) }
        }

    @Test
    fun `a circle filter load failure does not block the graph`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubFrom()
            coEvery { graphRepository.circlesWithMembers() } returns
                Result.failure(ApiError.Client(500, "boom"))
            stubConnections(
                response = GraphConnectionsResponse(
                    chains = listOf(chain(10, "t1", "Carol", 1, steps = listOf(GraphChainStep(10, "t1", "Carol", "child_of")))),
                ),
            )

            val vm = viewModel(contactId = 1)
            advanceUntilIdle()

            assertFalse(vm.uiState.value.isLoading)
            assertEquals(1, vm.uiState.value.allChains.size)
            assertTrue(vm.uiState.value.circles.isEmpty())
            assertNull(vm.uiState.value.error)
        }

    @Test
    fun `a graph load failure surfaces the error`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubFrom()
            coEvery { graphRepository.circlesWithMembers() } returns Result.success(emptyList())
            coEvery { graphRepository.getConnections(from = "uid-1", depth = any(), relation = any()) } returns
                Result.failure(ApiError.Client(400, "depth must be at most 5"))

            val vm = viewModel(contactId = 1)
            advanceUntilIdle()

            assertFalse(vm.uiState.value.isLoading)
            assertEquals("depth must be at most 5", vm.uiState.value.error)
        }
}
