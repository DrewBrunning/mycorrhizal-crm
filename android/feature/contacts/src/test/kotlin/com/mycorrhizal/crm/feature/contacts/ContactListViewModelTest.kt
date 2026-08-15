package com.mycorrhizal.crm.feature.contacts

import app.cash.turbine.test
import com.mycorrhizal.crm.domain.repository.BulkOperationRepository
import com.mycorrhizal.crm.domain.repository.CircleRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.ContactsPage
import com.mycorrhizal.crm.domain.repository.TagRepository
import com.mycorrhizal.crm.model.network.BulkOperationResult
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.SearchNoteHit
import com.mycorrhizal.crm.model.network.SearchResult
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.flow.emptyFlow
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class ContactListViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private fun page(vararg contacts: ContactSummary, nextCursor: String? = null): ContactsPage =
        ContactsPage(contacts = contacts.toList(), nextCursor = nextCursor, limit = 50, sync = null)

    /** Fresh repository + ApiClient mocks and ViewModel; stubs the cache stream the VM collects
     *  on init, defaults /search to an empty (never-called-in-most-tests) result, and stubs the
     *  M23 circle/tag list loads so the dropdown/picker state is deterministic. */
    private fun newViewModel(): Triple<ContactListViewModel, ContactRepository, ApiClient> {
        val repo = mockk<ContactRepository>()
        val apiClient = mockk<ApiClient>()
        val circleRepository = mockk<CircleRepository>()
        val bulkRepository = mockk<BulkOperationRepository>()
        val tagRepository = mockk<TagRepository>()
        coEvery { repo.observeContacts() } returns emptyFlow()
        coEvery { repo.searchLocal(any()) } returns emptyList()
        coEvery { apiClient.search(any(), any(), any()) } returns Result.success(SearchResult())
        coEvery { circleRepository.list() } returns Result.success(emptyList())
        coEvery { tagRepository.list() } returns Result.success(emptyList())
        return Triple(ContactListViewModel(repo, apiClient, circleRepository, bulkRepository, tagRepository), repo, apiClient)
    }

    @Test
    fun `initial load fetches the first page`() = runTest(mainDispatcherRule.testDispatcher) {
        val (viewModel, contactRepository, _) = newViewModel()
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = null) } returns
            Result.success(page(ContactSummary(id = 1, fn = "Alice")))

        advanceUntilIdle()

        val state = viewModel.uiState.value
        assertFalse(state.isLoading)
        assertEquals(1, state.contacts.size)
        assertEquals("Alice", state.contacts[0].fn)
    }

    @Test
    fun `error state sets the display message`() = runTest(mainDispatcherRule.testDispatcher) {
        val (viewModel, contactRepository, _) = newViewModel()
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = null) } returns
            Result.failure(ApiError.Server(500, "boom"))

        advanceUntilIdle()

        val state = viewModel.uiState.value
        assertFalse(state.isLoading)
        assertEquals("Server error (500)", state.error)
    }

    @Test
    fun `401 error emits ForceLogout`() = runTest(mainDispatcherRule.testDispatcher) {
        val (viewModel, contactRepository, _) = newViewModel()
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = null) } returns
            Result.failure(ApiError.Client(401, "Invalid token"))

        viewModel.events.test {
            advanceUntilIdle()
            val event = awaitItem()
            assertTrue(event is ContactListEvent.ForceLogout)
        }
    }

    @Test
    fun `cursor pagination appends to the existing list`() = runTest(mainDispatcherRule.testDispatcher) {
        val (viewModel, contactRepository, _) = newViewModel()
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = null) } returns
            Result.success(page(ContactSummary(id = 1, fn = "Alice"), nextCursor = "cursor2"))
        coEvery { contactRepository.listContacts(cursor = "cursor2", limit = 50, search = null) } returns
            Result.success(page(ContactSummary(id = 2, fn = "Bob"), nextCursor = null))

        advanceUntilIdle()
        assertEquals(1, viewModel.uiState.value.contacts.size)

        viewModel.loadNextPage()
        advanceUntilIdle()

        val state = viewModel.uiState.value
        assertEquals(2, state.contacts.size)
        assertFalse(state.pagination.hasMore)
    }

    @Test
    fun `loadNextPage is a no-op while already loading`() = runTest(mainDispatcherRule.testDispatcher) {
        val (viewModel, contactRepository, _) = newViewModel()
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = null) } returns
            Result.success(page(ContactSummary(id = 1, fn = "Alice"), nextCursor = "cursor2"))
        coEvery { contactRepository.listContacts(cursor = "cursor2", limit = 50, search = null) } returns
            Result.success(page(ContactSummary(id = 2, fn = "Bob"), nextCursor = null))

        advanceUntilIdle()

        viewModel.loadNextPage()
        viewModel.loadNextPage() // second call while the first page request is in flight
        advanceUntilIdle()

        // Exactly one page-2 fetch appends Bob; the second call bailed on isLoadingMore.
        assertEquals(2, viewModel.uiState.value.contacts.size)
    }

    @Test
    fun `search query is forwarded to the repository`() = runTest(mainDispatcherRule.testDispatcher) {
        val (viewModel, contactRepository, _) = newViewModel()
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = null) } returns
            Result.success(page())
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = "ali") } returns
            Result.success(page(ContactSummary(id = 3, fn = "Alicia")))

        advanceUntilIdle()

        viewModel.onSearchQueryChange("ali")
        advanceUntilIdle()

        val state = viewModel.uiState.value
        assertEquals("ali", state.searchQuery)
        assertEquals(1, state.contacts.size)
        assertEquals("Alicia", state.contacts[0].fn)
    }

    @Test
    fun `clearing the search reloads the full list`() = runTest(mainDispatcherRule.testDispatcher) {
        val (viewModel, contactRepository, _) = newViewModel()
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = null) } returns
            Result.success(page(ContactSummary(id = 1, fn = "Alice")))
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = "bob") } returns
            Result.success(page())

        advanceUntilIdle()
        viewModel.onSearchQueryChange("bob")
        advanceUntilIdle()
        assertEquals(0, viewModel.uiState.value.contacts.size)

        viewModel.onSearchQueryChange("")
        advanceUntilIdle()
        assertEquals(1, viewModel.uiState.value.contacts.size)
    }

    @Test
    fun `clicking a contact emits navigation event`() = runTest(mainDispatcherRule.testDispatcher) {
        val (viewModel, contactRepository, _) = newViewModel()
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = null) } returns
            Result.success(page(ContactSummary(id = 42, fn = "Alice")))

        viewModel.events.test {
            advanceUntilIdle()
            viewModel.onContactClick(42)
            val event = awaitItem()
            assertTrue(event is ContactListEvent.NavigateToContact)
            assertEquals(42, (event as ContactListEvent.NavigateToContact).contactId)
        }
    }

    @Test
    fun `network failure falls back to the local FTS cache for the query`() = runTest(mainDispatcherRule.testDispatcher) {
        val (viewModel, contactRepository, _) = newViewModel()
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = "dav") } returns
            Result.failure(ApiError.Network(java.io.IOException("offline")))
        coEvery { contactRepository.searchLocal("dav") } returns
            listOf(ContactSummary(id = 1, fn = "David Smith"))

        viewModel.onSearchQueryChange("dav")
        advanceUntilIdle()

        // The offline FTS results surface into the list.
        assertEquals("David Smith", viewModel.uiState.value.contacts.firstOrNull()?.fn)
    }

    @Test
    fun `network failure with no local matches leaves the list empty`() = runTest(mainDispatcherRule.testDispatcher) {
        val (viewModel, contactRepository, _) = newViewModel()
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = "zzz") } returns
            Result.failure(ApiError.Network(java.io.IOException("offline")))
        coEvery { contactRepository.searchLocal("zzz") } returns emptyList()

        viewModel.onSearchQueryChange("zzz")
        advanceUntilIdle()

        assertTrue(viewModel.uiState.value.contacts.isEmpty())
    }

    @Test
    fun `a note match populates the cross-entity search result`() = runTest(mainDispatcherRule.testDispatcher) {
        val (viewModel, contactRepository, apiClient) = newViewModel()
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = "mom") } returns
            Result.success(page())
        coEvery { apiClient.search("mom", null, null) } returns Result.success(
            SearchResult(
                resolvedRelation = "parent_of",
                notes = listOf(SearchNoteHit(id = 1, content = "called mom", contactId = 5, contactName = "Dana")),
            ),
        )

        viewModel.onSearchQueryChange("mom")
        advanceUntilIdle()

        val state = viewModel.uiState.value
        assertEquals("parent_of", state.searchResult?.resolvedRelation)
        assertEquals(1, state.searchResult?.notes?.size)
        assertEquals("Dana", state.searchResult?.notes?.first()?.contactName)
    }

    @Test
    fun `a one-character query fires no cross-entity search request`() = runTest(mainDispatcherRule.testDispatcher) {
        val (viewModel, contactRepository, apiClient) = newViewModel()
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = "a") } returns
            Result.success(page())

        viewModel.onSearchQueryChange("a")
        advanceUntilIdle()

        coVerify(exactly = 0) { apiClient.search(any(), any(), any()) }
        assertNull(viewModel.uiState.value.searchResult)
    }

    @Test
    fun `offline hides the cross-entity section without a section-level error`() = runTest(mainDispatcherRule.testDispatcher) {
        // T87: /search has no local mirror — unlike the contact list (which falls back to the
        // Room FTS4 cache via searchLocal), a genuine offline failure just clears the section
        // rather than showing its own error state. A realistic airplane-mode scenario: both the
        // primary list fetch and the cross-entity search fail; the list still resolves from
        // cache, and the section is absent (SearchNotesActivitiesSection's `if (searchResult ==
        // null) return` renders nothing, never an error) rather than double-erroring.
        val (viewModel, contactRepository, apiClient) = newViewModel()
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = "dav") } returns
            Result.failure(ApiError.Network(java.io.IOException("offline")))
        coEvery { contactRepository.searchLocal("dav") } returns
            listOf(ContactSummary(id = 1, fn = "David Smith"))
        coEvery { apiClient.search("dav", null, null) } returns
            Result.failure(ApiError.Network(java.io.IOException("offline")))

        viewModel.onSearchQueryChange("dav")
        advanceUntilIdle()

        val state = viewModel.uiState.value
        assertEquals("David Smith", state.contacts.firstOrNull()?.fn)
        assertNull(state.searchResult)
    }

    @Test
    fun `an empty cross-entity result is not treated as an error`() = runTest(mainDispatcherRule.testDispatcher) {
        val (viewModel, contactRepository, apiClient) = newViewModel()
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = "zzz") } returns
            Result.success(page())
        coEvery { apiClient.search("zzz", null, null) } returns Result.success(SearchResult())

        viewModel.onSearchQueryChange("zzz")
        advanceUntilIdle()

        val state = viewModel.uiState.value
        assertNull(state.error)
        assertEquals(0, state.searchResult?.notes?.size)
        assertEquals(0, state.searchResult?.activities?.size)
    }

    @Test
    fun `rapid typing debounces to one cross-entity search request`() = runTest(mainDispatcherRule.testDispatcher) {
        val (viewModel, contactRepository, apiClient) = newViewModel()
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = any()) } returns
            Result.success(page())

        viewModel.onSearchQueryChange("d")
        viewModel.onSearchQueryChange("da")
        viewModel.onSearchQueryChange("dav")
        advanceUntilIdle()

        // Each keystroke cancels the prior debounce (same searchJob) — only the last
        // query's request survives, not one per keystroke.
        coVerify(exactly = 1) { apiClient.search(any(), any(), any()) }
        coVerify(exactly = 1) { apiClient.search("dav", null, null) }
    }

    @Test
    fun `clearing the search also clears the cross-entity result`() = runTest(mainDispatcherRule.testDispatcher) {
        val (viewModel, contactRepository, apiClient) = newViewModel()
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = "mom") } returns
            Result.success(page())
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = null) } returns
            Result.success(page())
        coEvery { apiClient.search("mom", null, null) } returns
            Result.success(SearchResult(notes = listOf(SearchNoteHit(id = 1, content = "called mom"))))

        viewModel.onSearchQueryChange("mom")
        advanceUntilIdle()
        assertEquals(1, viewModel.uiState.value.searchResult?.notes?.size)

        viewModel.onSearchQueryChange("")
        advanceUntilIdle()
        assertNull(viewModel.uiState.value.searchResult)
    }

    // --- M23: circle filter -------------------------------------------------

    @Test
    fun `circle filter is forwarded to the repository and resets pagination`() = runTest(mainDispatcherRule.testDispatcher) {
        val (viewModel, contactRepository) = newViewModel()
        // First unfiltered page pages forward; the filtered request must start fresh (cursor null).
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = null) } returns
            Result.success(page(ContactSummary(id = 1, fn = "Alice"), nextCursor = "cursor2"))
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = null, circle = "Friends") } returns
            Result.success(page(ContactSummary(id = 2, fn = "Bob")))

        advanceUntilIdle()
        assertEquals(1, viewModel.uiState.value.contacts.size)

        viewModel.onCircleFilterChange("Friends")
        advanceUntilIdle()

        val state = viewModel.uiState.value
        assertEquals("Friends", state.circleFilter)
        assertEquals(1, state.contacts.size)
        assertEquals("Bob", state.contacts[0].fn)
        // A filter change is a fresh query, not a filtered page appended onto the previous list.
        assertNull(state.pagination.nextCursor)
        coVerify { contactRepository.listContacts(cursor = null, limit = 50, search = null, circle = "Friends") }
    }

    @Test
    fun `clearing the circle filter reloads the full list`() = runTest(mainDispatcherRule.testDispatcher) {
        val (viewModel, contactRepository) = newViewModel()
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = null, circle = "Friends") } returns
            Result.success(page(ContactSummary(id = 2, fn = "Bob")))
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = null) } returns
            Result.success(page(ContactSummary(id = 1, fn = "Alice")))

        viewModel.onCircleFilterChange("Friends")
        advanceUntilIdle()
        assertEquals("Bob", viewModel.uiState.value.contacts.firstOrNull()?.fn)

        viewModel.onCircleFilterChange(null)
        advanceUntilIdle()

        val state = viewModel.uiState.value
        assertNull(state.circleFilter)
        assertEquals("Alice", state.contacts.firstOrNull()?.fn)
    }

    // --- M23: archived toggle -------------------------------------------------

    @Test
    fun `archived toggle is forwarded to the repository`() = runTest(mainDispatcherRule.testDispatcher) {
        val (viewModel, contactRepository) = newViewModel()
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = null, includeArchived = true) } returns
            Result.success(page(ContactSummary(id = 1, fn = "Alice", archived = true)))

        viewModel.onIncludeArchivedChange(true)
        advanceUntilIdle()

        val state = viewModel.uiState.value
        assertTrue(state.includeArchived)
        assertEquals(1, state.contacts.size)
        coVerify { contactRepository.listContacts(cursor = null, limit = 50, search = null, includeArchived = true) }
    }

    // --- M23: selection is cleared when the visible set changes (test case 3) ---

    @Test
    fun `changing the circle filter clears the selection`() = runTest(mainDispatcherRule.testDispatcher) {
        val (viewModel, contactRepository) = newViewModel()
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = null) } returns
            Result.success(page(ContactSummary(id = 1, uid = "u1", fn = "Alice"), ContactSummary(id = 2, uid = "u2", fn = "Bob")))
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = null, circle = "Friends") } returns
            Result.success(page(ContactSummary(id = 3, uid = "u3", fn = "Carol")))

        advanceUntilIdle()
        viewModel.toggleSelection(1)
        assertEquals(setOf(1), viewModel.uiState.value.selected)

        viewModel.onCircleFilterChange("Friends")
        advanceUntilIdle()

        assertTrue(viewModel.uiState.value.selected.isEmpty())
    }

    @Test
    fun `changing the archived toggle clears the selection`() = runTest(mainDispatcherRule.testDispatcher) {
        val (viewModel, contactRepository) = newViewModel()
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = null) } returns
            Result.success(page(ContactSummary(id = 1, uid = "u1", fn = "Alice")))
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = null, includeArchived = true) } returns
            Result.success(page())

        advanceUntilIdle()
        viewModel.toggleSelection(1)
        assertEquals(setOf(1), viewModel.uiState.value.selected)

        viewModel.onIncludeArchivedChange(true)
        advanceUntilIdle()

        assertTrue(viewModel.uiState.value.selected.isEmpty())
    }

    @Test
    fun `changing the search query clears the selection`() = runTest(mainDispatcherRule.testDispatcher) {
        val (viewModel, contactRepository) = newViewModel()
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = null) } returns
            Result.success(page(ContactSummary(id = 1, uid = "u1", fn = "Alice")))
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = "ali") } returns
            Result.success(page())

        advanceUntilIdle()
        viewModel.toggleSelection(1)
        assertEquals(setOf(1), viewModel.uiState.value.selected)

        viewModel.onSearchQueryChange("ali")
        advanceUntilIdle()

        assertTrue(viewModel.uiState.value.selected.isEmpty())
    }

    // --- M23: inline bulk selection --------------------------------------------

    @Test
    fun `select all selects every loaded contact`() = runTest(mainDispatcherRule.testDispatcher) {
        val (viewModel, contactRepository) = newViewModel()
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = null) } returns
            Result.success(page(ContactSummary(id = 1, fn = "Alice"), ContactSummary(id = 2, fn = "Bob")))

        advanceUntilIdle()
        viewModel.toggleSelectAll()

        assertEquals(setOf(1, 2), viewModel.uiState.value.selected)
    }

    @Test
    fun `select all toggles off when every loaded contact is selected`() = runTest(mainDispatcherRule.testDispatcher) {
        val (viewModel, contactRepository) = newViewModel()
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = null) } returns
            Result.success(page(ContactSummary(id = 1, fn = "Alice"), ContactSummary(id = 2, fn = "Bob")))

        advanceUntilIdle()
        viewModel.toggleSelectAll()
        viewModel.toggleSelectAll()

        assertTrue(viewModel.uiState.value.selected.isEmpty())
    }

    @Test
    fun `select all preserves out-of-page selections while adding the loaded page`() = runTest(mainDispatcherRule.testDispatcher) {
        val (viewModel, contactRepository) = newViewModel()
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = null) } returns
            Result.success(page(ContactSummary(id = 1, fn = "Alice"), ContactSummary(id = 2, fn = "Bob")))

        advanceUntilIdle()
        // A selection can outlive the rows that carried it (web keys by uid precisely
        // because of this) — select an id not on the loaded page, then select-all must ADD
        // the page to the existing selection, never replace it.
        viewModel.toggleSelection(99)
        viewModel.toggleSelectAll()

        assertEquals(setOf(99, 1, 2), viewModel.uiState.value.selected)
    }

    @Test
    fun `a bulk action runs with the selected uids and clears the selection on success`() = runTest(mainDispatcherRule.testDispatcher) {
        val (viewModel, contactRepository) = newViewModel()
        val bulkRepository = mockk<BulkOperationRepository>()
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = null) } returns
            Result.success(page(ContactSummary(id = 1, uid = "u1", fn = "Alice"), ContactSummary(id = 2, uid = "u2", fn = "Bob")))
        coEvery {
            bulkRepository.run(match { it.action == "archive" && it.vcardUids.toSet() == setOf("u1", "u2") })
        } returns Result.success(BulkOperationResult(action = "archive", total = 2, succeeded = 2, failed = 0))

        // Recreate with the stubbed bulk repo so runBulkAction verifies against it.
        // (The helper's bulk repo is anonymous, so this test wires its own.)
        val apiClient = mockk<ApiClient>()
        coEvery { apiClient.search(any(), any(), any()) } returns Result.success(SearchResult())
        val circleRepository = mockk<CircleRepository>()
        coEvery { circleRepository.list() } returns Result.success(emptyList())
        val tagRepository = mockk<TagRepository>()
        coEvery { tagRepository.list() } returns Result.success(emptyList())
        val vm = ContactListViewModel(contactRepository, apiClient, circleRepository, bulkRepository, tagRepository)

        advanceUntilIdle()
        vm.toggleSelection(1)
        vm.toggleSelection(2)
        vm.runBulkAction("archive")
        advanceUntilIdle()

        assertEquals(2, vm.uiState.value.bulkResult?.succeeded)
        assertTrue(vm.uiState.value.selected.isEmpty())
    }

    @Test
    fun `a failed bulk action leaves the selection untouched and surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        val (viewModel, contactRepository) = newViewModel()
        val bulkRepository = mockk<BulkOperationRepository>()
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = null) } returns
            Result.success(page(ContactSummary(id = 1, uid = "u1", fn = "Alice")))
        coEvery {
            bulkRepository.run(match { it.action == "archive" })
        } returns Result.failure(ApiError.Client(400, "boom"))
        val apiClient = mockk<ApiClient>()
        coEvery { apiClient.search(any(), any(), any()) } returns Result.success(SearchResult())
        val circleRepository = mockk<CircleRepository>()
        coEvery { circleRepository.list() } returns Result.success(emptyList())
        val tagRepository = mockk<TagRepository>()
        coEvery { tagRepository.list() } returns Result.success(emptyList())
        val vm = ContactListViewModel(contactRepository, apiClient, circleRepository, bulkRepository, tagRepository)

        advanceUntilIdle()
        vm.toggleSelection(1)
        vm.runBulkAction("archive")
        advanceUntilIdle()

        assertEquals(setOf(1), vm.uiState.value.selected)
        assertEquals("boom", vm.uiState.value.error)
    }
}
