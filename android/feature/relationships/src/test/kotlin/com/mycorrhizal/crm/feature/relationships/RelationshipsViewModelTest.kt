package com.mycorrhizal.crm.feature.relationships

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.RelationshipEdgeRepository
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.model.network.RelationshipEdge
import com.mycorrhizal.crm.model.network.RelationshipEdgeStatuses
import com.mycorrhizal.crm.model.network.RelationshipEdgeTypes
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

class RelationshipsViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val edgeRepository = mockk<RelationshipEdgeRepository>()
    private val contactRepository = mockk<ContactRepository>()

    private fun viewModel(id: Int = 5): RelationshipsViewModel =
        RelationshipsViewModel(edgeRepository, contactRepository, SavedStateHandle(mapOf("contactId" to id)))

    private val viewedUid = "11111111-1111-1111-1111-111111111111"
    private val otherUid = "22222222-2222-2222-2222-222222222222"

    private fun stubContact() {
        coEvery { contactRepository.getContact(5) } returns Result.success(
            ContactRecordResponse(id = 5, card = Card(uid = viewedUid, name = Name(full = "Dana White"))),
        )
    }

    @Test
    fun `loads edges after resolving the contact's vcard uid`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact()
        coEvery { edgeRepository.listForContact(viewedUid, null, null) } returns Result.success(
            listOf(
                RelationshipEdge(id = "e1", sourceId = otherUid, targetId = viewedUid, type = RelationshipEdgeTypes.FRIEND_OF),
            ),
        )

        val vm = viewModel()
        advanceUntilIdle()

        assertFalse(vm.uiState.value.isLoading)
        assertEquals(viewedUid, vm.uiState.value.contactVCardUid)
        assertEquals(1, vm.uiState.value.edges.size)
        assertNull(vm.uiState.value.error)
    }

    @Test
    fun `missing contact id sets an error`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = viewModel(id = 0)
        advanceUntilIdle()

        assertFalse(vm.uiState.value.isLoading)
        assertEquals("Missing contact id", vm.uiState.value.error)
    }

    @Test
    fun `load failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact()
        coEvery { edgeRepository.listForContact(viewedUid, null, null) } returns Result.failure(
            ApiError.Client(500, "boom"),
        )

        val vm = viewModel()
        advanceUntilIdle()

        assertEquals("boom", vm.uiState.value.error)
    }

    @Test
    fun `create sends the viewed contact as target`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact()
        coEvery { edgeRepository.listForContact(viewedUid, null, null) } returns Result.success(emptyList())
        coEvery {
            edgeRepository.create(
                match { input ->
                    input.targetId == viewedUid &&
                        input.sourceId == otherUid &&
                        input.type == RelationshipEdgeTypes.SPOUSE_OF
                },
            )
        } returns Result.success(
            RelationshipEdge(id = "e9", sourceId = otherUid, targetId = viewedUid, type = RelationshipEdgeTypes.SPOUSE_OF),
        )

        val vm = viewModel()
        advanceUntilIdle()

        vm.create(RelationshipEdgeTypes.SPOUSE_OF, otherUid, "")
        advanceUntilIdle()

        assertEquals(1, vm.uiState.value.edges.size)
    }

    @Test
    fun `accept promotes a suggested edge`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact()
        val suggested = RelationshipEdge(
            id = "e1", sourceId = otherUid, targetId = viewedUid,
            type = RelationshipEdgeTypes.PARENT_OF, status = RelationshipEdgeStatuses.SUGGESTED,
        )
        coEvery { edgeRepository.listForContact(viewedUid, null, null) } returns Result.success(listOf(suggested))
        coEvery { edgeRepository.accept("e1") } returns Result.success(suggested.copy(status = RelationshipEdgeStatuses.CONFIRMED))

        val vm = viewModel()
        advanceUntilIdle()

        vm.accept("e1")
        advanceUntilIdle()

        assertEquals(RelationshipEdgeStatuses.CONFIRMED, vm.uiState.value.edges[0].status)
    }

    @Test
    fun `delete removes the edge`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact()
        coEvery { edgeRepository.listForContact(viewedUid, null, null) } returns Result.success(
            listOf(
                RelationshipEdge(id = "e1", sourceId = otherUid, targetId = viewedUid, type = RelationshipEdgeTypes.FRIEND_OF),
                RelationshipEdge(id = "e2", sourceId = viewedUid, targetId = otherUid, type = RelationshipEdgeTypes.SIBLING_OF),
            ),
        )
        coEvery { edgeRepository.delete("e1") } returns Result.success(Unit)

        val vm = viewModel()
        advanceUntilIdle()

        vm.delete("e1")
        advanceUntilIdle()

        assertEquals(listOf("e2"), vm.uiState.value.edges.map { it.id })
        coVerify { edgeRepository.delete("e1") }
    }
}

class RelationshipEdgeSemanticsTest {

    @Test
    fun `effective type inverts when viewed contact is the source`() {
        val viewed = "aaa"
        val other = "bbb"
        val edge = RelationshipEdge(id = "e", sourceId = viewed, targetId = other, type = RelationshipEdgeTypes.PARENT_OF)

        // Viewed is the parent, so the other party is the child.
        assertEquals(RelationshipEdgeTypes.CHILD_OF, effectiveType(edge, viewed))
    }

    @Test
    fun `effective type is identity when viewed contact is the target`() {
        val viewed = "bbb"
        val other = "aaa"
        val edge = RelationshipEdge(id = "e", sourceId = other, targetId = viewed, type = RelationshipEdgeTypes.PARENT_OF)

        // Viewed is the child; the other party (source) is the parent.
        assertEquals(RelationshipEdgeTypes.PARENT_OF, effectiveType(edge, viewed))
    }

    @Test
    fun `unknown type degrades to related_to`() {
        val edge = RelationshipEdge(id = "e", sourceId = "a", targetId = "b", type = "future_token")
        assertEquals(RelationshipEdgeTypes.RELATED_TO, effectiveType(edge, "b"))
    }
}
