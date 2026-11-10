package com.mycorrhizal.crm.feature.network

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.GraphRepository
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.GraphChain
import com.mycorrhizal.crm.model.network.GraphConnectionsResponse
import com.mycorrhizal.crm.model.network.GraphContactNode
import com.mycorrhizal.crm.model.network.GraphEntitiesResponse
import com.mycorrhizal.crm.model.network.GraphEntityEdge
import com.mycorrhizal.crm.model.network.GraphEntityNode
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
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

/** Issue #2302 / ADR 0043 decision 2: the graph's entity nodes are an opt-in, read-only overlay. */
class NetworkEntitiesOverlayTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val graphRepository = mockk<GraphRepository>()
    private val contactRepository = mockk<ContactRepository>()

    private val chains = listOf(
        GraphChain(targetId = 7, targetVCardUid = "t7", targetName = "Ada", depth = 1)
    )

    private val overlay = GraphEntitiesResponse(
        nodes = listOf(
            GraphContactNode("c-1", "contact", "Alice"),
            GraphContactNode("c-7", "contact", "Ada")
        ),
        entityNodes = listOf(
            GraphEntityNode("e-o1", "organisation", "Acme"),
            GraphEntityNode("e-p1", "place", "Cabin")
        ),
        entityEdges = listOf(
            GraphEntityEdge("x1", "c-1", "e-o1"),
            GraphEntityEdge("x2", "c-7", "e-o1"),
            GraphEntityEdge("x3", "c-7", "e-p1")
        )
    )

    private fun viewModel(): NetworkViewModel {
        coEvery { contactRepository.getContact(1) } returns Result.success(
            ContactRecordResponse(id = 1, card = Card(uid = "uid-1", name = Name(full = "Alice")))
        )
        coEvery { graphRepository.circlesWithMembers() } returns Result.success(emptyList())
        coEvery { graphRepository.getConnections("uid-1", any(), any()) } returns
            Result.success(GraphConnectionsResponse(chains = chains))
        return NetworkViewModel(
            graphRepository,
            contactRepository,
            SavedStateHandle(mapOf("contactId" to 1))
        )
    }

    @Test
    fun `entity nodes are never requested until the user opts in`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = viewModel()
            advanceUntilIdle()
            assertFalse(vm.uiState.value.showEntities)
            assertTrue(vm.uiState.value.entityOverlay.isEmpty())
            coVerify(exactly = 0) { graphRepository.entityOverlay(any(), any()) }
        }

    @Test
    fun `opting in loads the overlay and lists each entity with its contacts`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { graphRepository.entityOverlay("uid-1", 2) } returns Result.success(overlay)
            val vm = viewModel()
            advanceUntilIdle()
            vm.setShowEntities(true)
            advanceUntilIdle()
            val rows = vm.uiState.value.entityOverlay
            assertEquals(listOf("Acme", "Cabin"), rows.map { it.node.label })
            assertEquals(listOf("Alice", "Ada"), rows[0].contactNames)
            assertEquals(listOf("Ada"), rows[1].contactNames)
        }

    @Test
    fun `the overlay never changes the traversal results`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { graphRepository.entityOverlay(any(), any()) } returns Result.success(overlay)
        val vm = viewModel()
        advanceUntilIdle()
        val before = vm.uiState.value.allChains
        vm.setShowEntities(true)
        advanceUntilIdle()
        assertEquals(before, vm.uiState.value.allChains)
        coVerify(exactly = 1) { graphRepository.getConnections(any(), any(), any()) }
    }

    @Test
    fun `turning it off clears the overlay and a depth change reloads it while on`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery {
                graphRepository.entityOverlay(
                    "uid-1",
                    any()
                )
            } returns Result.success(overlay)
            val vm = viewModel()
            advanceUntilIdle()
            vm.setShowEntities(true)
            advanceUntilIdle()
            vm.setDepth(3)
            advanceUntilIdle()
            coVerify { graphRepository.entityOverlay("uid-1", 3) }
            vm.setShowEntities(false)
            assertTrue(vm.uiState.value.entityOverlay.isEmpty())
            assertFalse(vm.uiState.value.showEntities)
        }

    @Test
    fun `a failed overlay load is flagged without breaking the traversal`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { graphRepository.entityOverlay(any(), any()) } returns
                Result.failure(ApiError.Unknown(RuntimeException("x")))
            val vm = viewModel()
            advanceUntilIdle()
            vm.setShowEntities(true)
            advanceUntilIdle()
            assertTrue(vm.uiState.value.entitiesFailed)
            assertEquals(chains, vm.uiState.value.allChains)
            assertNull(vm.uiState.value.error)
        }

    private fun assertNull(value: Any?) = org.junit.Assert.assertNull(value)
}
