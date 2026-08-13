package com.mycorrhizal.crm.feature.relationships

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.ContactsPage
import com.mycorrhizal.crm.domain.repository.RelationshipEdgeRepository
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.model.network.RelationshipEdge
import com.mycorrhizal.crm.model.network.RelationshipEdgeSensitivities
import com.mycorrhizal.crm.model.network.RelationshipEdgeStatuses
import com.mycorrhizal.crm.model.network.RelationshipEdgeTypes
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import com.mycorrhizal.crm.ui.R
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
        // Sane default so tests that don't care about name resolution don't
        // need their own stub; tests that do override it below.
        coEvery { contactRepository.resolveByUid(any()) } returns Result.success(emptyMap())
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
        assertEquals(R.string.relationships_error_missing_contact_id, vm.uiState.value.errorRes)
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

    // --- Name resolution (test case 1 & 2 from the M21 implementation contract) ---

    @Test
    fun `load resolves the other party's name into contactsByUid`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact()
        coEvery { edgeRepository.listForContact(viewedUid, null, null) } returns Result.success(
            listOf(RelationshipEdge(id = "e1", sourceId = otherUid, targetId = viewedUid, type = RelationshipEdgeTypes.FRIEND_OF)),
        )
        val otherContact = ContactSummary(id = 9, uid = otherUid, fn = "Pat Lee")
        coEvery { contactRepository.resolveByUid(listOf(otherUid)) } returns Result.success(mapOf(otherUid to otherContact))

        val vm = viewModel()
        advanceUntilIdle()

        assertEquals("Pat Lee", vm.uiState.value.contactsByUid[otherUid]?.fn)
    }

    @Test
    fun `a UID the batch resolve couldn't find is simply absent, not an error`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact()
        coEvery { edgeRepository.listForContact(viewedUid, null, null) } returns Result.success(
            listOf(RelationshipEdge(id = "e1", sourceId = otherUid, targetId = viewedUid, type = RelationshipEdgeTypes.FRIEND_OF)),
        )
        coEvery { contactRepository.resolveByUid(listOf(otherUid)) } returns Result.success(emptyMap())

        val vm = viewModel()
        advanceUntilIdle()

        assertNull(vm.uiState.value.error)
        assertTrue(vm.uiState.value.contactsByUid.isEmpty())
    }

    @Test
    fun `a resolveByUid network failure does not surface as the screen's error`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact()
        coEvery { edgeRepository.listForContact(viewedUid, null, null) } returns Result.success(
            listOf(RelationshipEdge(id = "e1", sourceId = otherUid, targetId = viewedUid, type = RelationshipEdgeTypes.FRIEND_OF)),
        )
        coEvery { contactRepository.resolveByUid(listOf(otherUid)) } returns Result.failure(ApiError.Client(500, "boom"))

        val vm = viewModel()
        advanceUntilIdle()

        assertNull(vm.uiState.value.error)
        assertTrue(vm.uiState.value.contactsByUid.isEmpty())
    }

    // --- Confirmed/suggested split (test case 4) ---

    @Test
    fun `confirmedEdges and suggestedEdges split the loaded edges by status`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact()
        val confirmed = RelationshipEdge(
            id = "e1", sourceId = otherUid, targetId = viewedUid,
            type = RelationshipEdgeTypes.FRIEND_OF, status = RelationshipEdgeStatuses.CONFIRMED,
        )
        val suggested = RelationshipEdge(
            id = "e2", sourceId = otherUid, targetId = viewedUid,
            type = RelationshipEdgeTypes.PARENT_OF, status = RelationshipEdgeStatuses.SUGGESTED,
        )
        coEvery { edgeRepository.listForContact(viewedUid, null, null) } returns Result.success(listOf(confirmed, suggested))

        val vm = viewModel()
        advanceUntilIdle()

        assertEquals(listOf("e1"), vm.uiState.value.confirmedEdges.map { it.id })
        assertEquals(listOf("e2"), vm.uiState.value.suggestedEdges.map { it.id })
    }

    // --- create ---

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
    fun `create sends gender, birthday and sensitivity on a manual (thin) entry`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact()
        coEvery { edgeRepository.listForContact(viewedUid, null, null) } returns Result.success(emptyList())
        coEvery {
            edgeRepository.create(
                match { input ->
                    input.targetId == viewedUid &&
                        input.sourceThin?.name == "Jamie Fox" &&
                        input.sourceThin?.gender == "non_binary" &&
                        input.sourceThin?.birthday == "1990-06-15" &&
                        input.sensitivity == RelationshipEdgeSensitivities.SECRET
                },
            )
        } returns Result.success(
            RelationshipEdge(id = "e9", sourceId = "", targetId = viewedUid, type = RelationshipEdgeTypes.FRIEND_OF),
        )

        val vm = viewModel()
        advanceUntilIdle()

        vm.create(
            type = RelationshipEdgeTypes.FRIEND_OF,
            otherPartyVCardUid = "",
            otherPartyName = "Jamie Fox",
            gender = "non_binary",
            birthday = "1990-06-15",
            sensitivity = RelationshipEdgeSensitivities.SECRET,
        )
        advanceUntilIdle()

        assertEquals(1, vm.uiState.value.edges.size)
    }

    @Test
    fun `create via linked search merges the selected contact into contactsByUid immediately`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact()
        coEvery { edgeRepository.listForContact(viewedUid, null, null) } returns Result.success(emptyList())
        val linked = ContactSummary(id = 4, uid = otherUid, fn = "Robin Lee")
        coEvery {
            edgeRepository.create(match { it.sourceId == otherUid && it.targetId == viewedUid })
        } returns Result.success(
            RelationshipEdge(id = "e9", sourceId = otherUid, targetId = viewedUid, type = RelationshipEdgeTypes.FRIEND_OF),
        )

        val vm = viewModel()
        advanceUntilIdle()

        vm.create(RelationshipEdgeTypes.FRIEND_OF, otherUid, "", linkedContact = linked)
        advanceUntilIdle()

        assertEquals("Robin Lee", vm.uiState.value.contactsByUid[otherUid]?.fn)
    }

    // --- update (test case 5) ---

    @Test
    fun `update converts a viewer-relative type back to source-relative before sending`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact()
        val edge = RelationshipEdge(id = "e1", sourceId = viewedUid, targetId = otherUid, type = RelationshipEdgeTypes.PARENT_OF)
        coEvery { edgeRepository.listForContact(viewedUid, null, null) } returns Result.success(listOf(edge))
        val updated = edge.copy(type = RelationshipEdgeTypes.CHILD_OF, sensitivity = RelationshipEdgeSensitivities.PRIVATE)
        coEvery {
            edgeRepository.update(
                "e1",
                match { input ->
                    input.sourceId == viewedUid &&
                        input.targetId == otherUid &&
                        input.type == RelationshipEdgeTypes.CHILD_OF &&
                        input.sensitivity == RelationshipEdgeSensitivities.PRIVATE
                },
            )
        } returns Result.success(updated)

        val vm = viewModel()
        advanceUntilIdle()

        // Dropdown token PARENT_OF ("the other party is my parent"); viewed
        // contact is the edge's source, so the backend-relative type must
        // invert to CHILD_OF ("I am the other party's child").
        vm.update("e1", RelationshipEdgeTypes.PARENT_OF, RelationshipEdgeSensitivities.PRIVATE)
        advanceUntilIdle()

        assertEquals(RelationshipEdgeTypes.CHILD_OF, vm.uiState.value.edges[0].type)
        assertEquals(RelationshipEdgeSensitivities.PRIVATE, vm.uiState.value.edges[0].sensitivity)
        coVerify { edgeRepository.update("e1", any()) }
    }

    // --- search (linked-entry mode) ---

    @Test
    fun `searchContacts debounces and only the latest query reaches the repository`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact()
        coEvery { edgeRepository.listForContact(viewedUid, null, null) } returns Result.success(emptyList())
        coEvery { contactRepository.listContacts(search = "al", limit = 25) } returns Result.success(
            ContactsPage(contacts = listOf(ContactSummary(id = 1, uid = "u9", fn = "Alan")), nextCursor = "", limit = 25, sync = null),
        )

        val vm = viewModel()
        advanceUntilIdle()

        vm.searchContacts("a")
        vm.searchContacts("al") // supersedes the "a" search before its debounce fires
        advanceUntilIdle()

        assertEquals(1, vm.uiState.value.contactSearchResults.size)
        assertEquals("Alan", vm.uiState.value.contactSearchResults[0].fn)
        coVerify(exactly = 0) { contactRepository.listContacts(search = "a", limit = 25) }
    }

    @Test
    fun `searchContacts excludes the viewed contact from results`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact()
        coEvery { edgeRepository.listForContact(viewedUid, null, null) } returns Result.success(emptyList())
        coEvery { contactRepository.listContacts(search = "d", limit = 25) } returns Result.success(
            ContactsPage(
                contacts = listOf(
                    ContactSummary(id = 5, uid = viewedUid, fn = "Dana White"),
                    ContactSummary(id = 9, uid = otherUid, fn = "Dana Carvey"),
                ),
                nextCursor = "",
                limit = 25,
                sync = null,
            ),
        )

        val vm = viewModel()
        advanceUntilIdle()

        vm.searchContacts("d")
        advanceUntilIdle()

        assertEquals(listOf(otherUid), vm.uiState.value.contactSearchResults.map { it.uid })
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

    // --- toBackendType (direction, test case 3) ---

    @Test
    fun `toBackendType inverts when the viewed contact is the source`() {
        assertEquals(RelationshipEdgeTypes.CHILD_OF, toBackendType(RelationshipEdgeTypes.PARENT_OF, viewedIsSource = true))
    }

    @Test
    fun `toBackendType is identity when the viewed contact is the target`() {
        assertEquals(RelationshipEdgeTypes.PARENT_OF, toBackendType(RelationshipEdgeTypes.PARENT_OF, viewedIsSource = false))
    }

    @Test
    fun `toBackendType degrades an unknown token to related_to`() {
        assertEquals(RelationshipEdgeTypes.RELATED_TO, toBackendType("future_token", viewedIsSource = true))
    }
}
