package com.mycorrhizal.crm.feature.contacts

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.BulkOperationRepository
import com.mycorrhizal.crm.domain.repository.CircleRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.ContactsPage
import com.mycorrhizal.crm.domain.repository.TagRepository
import com.mycorrhizal.crm.model.network.BulkContactOperationInput
import com.mycorrhizal.crm.model.network.BulkOperationResult
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.SearchResult
import com.mycorrhizal.crm.model.network.Tag
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.delay
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class PaginationState(
    val nextCursor: String? = null,
    val limit: Int = 50,
    val isLoadingMore: Boolean = false,
    val hasMore: Boolean = true,
)

data class ContactListUiState(
    val contacts: List<ContactSummary> = emptyList(),
    val isLoading: Boolean = false,
    val error: String? = null,
    val searchQuery: String = "",
    val pagination: PaginationState = PaginationState(),
    /**
     * T87: cross-entity (notes/activities) matches for [searchQuery], from `GET /search`. Null
     * means "nothing to show" — no query, a query below the two-character gate, or the request
     * failed (offline: hide the section, don't error; see [loadCrossEntitySearch]'s doc comment).
     * Never routed through the local FTS4 cache — that mirror only covers cached contact rows.
     */
    val searchResult: SearchResult? = null,
    // M23: list-breadth state. Circle filter + archived toggle both narrow the same row set
    // the list already queries; the selection set is inline bulk-selection (web parity) so a
    // bulk action runs against exactly the contacts the user can see.
    val circles: List<Circle> = emptyList(),
    /** The circle NAME the list is filtered by (the backend's `?circle=` matches names), or null for all. */
    val circleFilter: String? = null,
    val includeArchived: Boolean = false,
    val tags: List<Tag> = emptyList(),
    /** Selected contact ids for inline bulk actions. Cleared whenever the visible set changes. */
    val selected: Set<Int> = emptySet(),
    val isBulkRunning: Boolean = false,
    val bulkResult: BulkOperationResult? = null,
)

sealed interface ContactListEvent {
    data class NavigateToContact(val contactId: Int) : ContactListEvent
    data object ForceLogout : ContactListEvent
}

@HiltViewModel
class ContactListViewModel @Inject constructor(
    private val contactRepository: ContactRepository,
    // T87: /search is cross-entity (notes + activities), not owned by any one repository —
    // DashboardViewModel injects ApiClient directly for the same reason (a composite endpoint
    // with no single-entity home).
    private val apiClient: ApiClient,
    // M23: circles feed the list's filter dropdown; the bulk-operation repository runs the
    // inline bulk actions (the same surface web's BulkActionsBar drives).
    private val circleRepository: CircleRepository,
    private val bulkOperationRepository: BulkOperationRepository,
    private val tagRepository: TagRepository,
) : ViewModel() {

    private val _uiState = MutableStateFlow(ContactListUiState())
    val uiState: StateFlow<ContactListUiState> = _uiState.asStateFlow()

    private val _events = Channel<ContactListEvent>(Channel.BUFFERED)
    val events = _events.receiveAsFlow()

    init {
        // Keep the list populated from the local cache even when the network
        // is unreachable (online-first with graceful offline degradation).
        viewModelScope.launch {
            contactRepository.observeContacts().collect { cached ->
                // Only surface cached rows when the live list is empty (i.e.
                // the fetch has not populated it yet) — never override fresh
                // network results with stale cache.
                //
                // M23: the Room mirror is the unfiltered whole table, so under
                // an active circle/archived/search filter it must not be
                // offered as the answer — showing an unfiltered page against a
                // circle filter would be silently wrong data.
                _uiState.update {
                    if (it.contacts.isEmpty() && !hasActiveFilter(it)) it.copy(contacts = cached) else it
                }
            }
        }
        loadCircles()
        loadTags()
        loadContacts()
    }

    private fun hasActiveFilter(state: ContactListUiState): Boolean =
        state.circleFilter != null || state.includeArchived || state.searchQuery.isNotBlank()

    fun loadContacts() {
        val state = _uiState.value
        if (state.isLoading) return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null, contacts = emptyList()) }
            val page = contactRepository.listContacts(
                cursor = null,
                limit = _uiState.value.pagination.limit,
                search = _uiState.value.searchQuery.takeIf { it.isNotBlank() },
                circle = _uiState.value.circleFilter,
                includeArchived = _uiState.value.includeArchived.takeIf { it },
            )
            page.foldApiError(
                onSuccess = { result ->
                    _uiState.update {
                        it.copy(
                            isLoading = false,
                            contacts = result.contacts,
                            pagination = it.pagination.copy(
                                nextCursor = result.nextCursor,
                                isLoadingMore = false,
                                hasMore = !result.nextCursor.isNullOrEmpty(),
                            ),
                        )
                    }
                },
                onError = { error ->
                    _uiState.update {
                        it.copy(
                            isLoading = false,
                            contacts = if (it.contacts.isEmpty()) {
                                // Fall back to the Room cache so offline list view still works.
                                // A search query matches locally via the FTS mirror (Phase 2 item 13).
                                emptyList()
                            } else {
                                it.contacts
                            },
                            error = error.displayMessage,
                        )
                    }
                    handleAuthError(error)
                    // Surface cached rows for the current query (FTS) so the
                    // offline list isn't blank when the fetch fails.
                    val query = _uiState.value.searchQuery
                    viewModelScope.launch {
                        val cached = contactRepository.searchLocal(query)
                        if (cached.isNotEmpty()) {
                            _uiState.update { it.copy(contacts = cached) }
                        }
                    }
                },
            )
        }
    }

    fun loadNextPage() {
        val state = _uiState.value
        if (state.isLoading || state.pagination.isLoadingMore || !state.pagination.hasMore) return
        _uiState.update { it.copy(pagination = it.pagination.copy(isLoadingMore = true)) }
        viewModelScope.launch {
            val page = contactRepository.listContacts(
                cursor = state.pagination.nextCursor,
                limit = state.pagination.limit,
                search = state.searchQuery.takeIf { it.isNotBlank() },
                circle = state.circleFilter,
                includeArchived = state.includeArchived.takeIf { it },
            )
            page.foldApiError(
                onSuccess = { result ->
                    _uiState.update {
                        it.copy(
                            contacts = it.contacts + result.contacts,
                            pagination = it.pagination.copy(
                                nextCursor = result.nextCursor,
                                isLoadingMore = false,
                                hasMore = !result.nextCursor.isNullOrEmpty(),
                            ),
                        )
                    }
                },
                onError = { error ->
                    _uiState.update {
                        it.copy(pagination = it.pagination.copy(isLoadingMore = false), error = error.displayMessage)
                    }
                    handleAuthError(error)
                },
            )
        }
    }

    private fun loadCircles() {
        viewModelScope.launch {
            circleRepository.list().foldApiError(
                onSuccess = { circles -> _uiState.update { it.copy(circles = circles) } },
                onError = { /* The dropdown just shows "All circles"; the list's own error already surfaces. */ },
            )
        }
    }

    private fun loadTags() {
        viewModelScope.launch {
            tagRepository.list().foldApiError(
                onSuccess = { tags -> _uiState.update { it.copy(tags = tags) } },
                onError = { /* The picker just shows empty; the list's own error already surfaces. */ },
            )
        }
    }

    // --- M23: filters -------------------------------------------------------

    /** M23: selecting a circle narrows the list; a null/blank choice resets to all. */
    fun onCircleFilterChange(circleName: String?) {
        val next = circleName?.takeIf { it.isNotBlank() }
        if (next == _uiState.value.circleFilter) return
        // A stale selection lets a bulk action (including delete) run against
        // contacts the user can no longer see — web clears it deliberately
        // (ContactsPage.tsx) and so must we (M23 test case 3).
        _uiState.update { it.copy(circleFilter = next, selected = emptySet()) }
        loadContacts()
    }

    /** M23: toggling the archived switch widens/narrows the row set; same selection-clearing rule. */
    fun onIncludeArchivedChange(include: Boolean) {
        if (include == _uiState.value.includeArchived) return
        _uiState.update { it.copy(includeArchived = include, selected = emptySet()) }
        loadContacts()
    }

    // --- M23: inline bulk selection ------------------------------------------

    fun toggleSelection(id: Int) {
        _uiState.update { state ->
            val next = state.selected.toMutableSet()
            if (!next.add(id)) next.remove(id)
            state.copy(selected = next)
        }
    }

    /** Select all currently-loaded rows (web's BulkActionsBar "select all" is page-scoped too). */
    fun toggleSelectAll() {
        _uiState.update { state ->
            val loaded = state.contacts.map { it.id }
            if (state.contacts.isNotEmpty() && state.selected.containsAll(loaded)) {
                state.copy(selected = emptySet())
            } else {
                state.copy(selected = state.selected + loaded)
            }
        }
    }

    /** Runs a bulk action over the current selection; clears it on success and reloads. */
    fun runBulkAction(action: String, circleId: String? = null, tagId: String? = null) {
        val uids = selectedUids()
        if (uids.isEmpty() || _uiState.value.isBulkRunning) return
        viewModelScope.launch {
            _uiState.update { it.copy(isBulkRunning = true, error = null, bulkResult = null) }
            bulkOperationRepository.run(
                BulkContactOperationInput(
                    action = action,
                    vcardUids = uids,
                    circleId = circleId,
                    tagId = tagId,
                ),
            ).foldApiError(
                onSuccess = { result ->
                    _uiState.update {
                        it.copy(isBulkRunning = false, bulkResult = result, selected = emptySet())
                    }
                    loadContacts()
                    loadCircles()
                    loadTags()
                },
                onError = { error ->
                    _uiState.update { it.copy(isBulkRunning = false, error = error.displayMessage) }
                    handleAuthError(error)
                },
            )
        }
    }

    private fun selectedUids(): List<String> {
        val selectedIds = _uiState.value.selected
        return _uiState.value.contacts.filter { it.id in selectedIds }.mapNotNull { it.uid }
    }

    private var searchJob: Job? = null

    fun onSearchQueryChange(query: String) {
        _uiState.update { it.copy(searchQuery = query) }
        // Debounce keystrokes so a fast typist doesn't fire one request per
        // character; also reload immediately when cleared.
        //
        // T87: the cross-entity search shares this same debounce/cancellation rather than
        // running its own timer — a second independent debounce is the specific way this trap
        // was flagged to break (rapid typing firing two out-of-order request streams).
        //
        // M23: a search change swaps the visible set, so it clears selection just like the
        // circle/archived filters (web's selection-clearing effect lists searchQuery first).
        _uiState.update { it.copy(selected = emptySet()) }
        searchJob?.cancel()
        searchJob = viewModelScope.launch {
            if (query.isBlank()) {
                _uiState.update { it.copy(searchResult = null) }
                loadContacts()
            } else {
                delay(SEARCH_DEBOUNCE_MS)
                loadContacts()
                loadCrossEntitySearch(query)
            }
        }
    }

    /**
     * T87: notes/activities matches for [query] from `GET /search`, rendered as a collapsed
     * section below the contact list. Two deliberate decisions:
     * - **Client-side two-character gate**, matching the backend's own rather than relying on
     *   it alone — a one-character query fires no request at all.
     * - **Offline hides the section, never errors.** `/search` has no local mirror (unlike the
     *   contact list, which falls back to the Room FTS4 cache) — any failure, not just a
     *   connectivity one, just clears [ContactListUiState.searchResult]. This is a structural
     *   guarantee, not a tested one: this function never writes [ContactListUiState.error] at
     *   all, on either branch, so it cannot show its own error surface by construction. (A
     *   runtime assertion that it stays null after a failure here is not reliable — `loadContacts`
     *   unconditionally resets `error` at the very start of its own next run, in the same tick,
     *   which would mask a regression here regardless of assertion placement.)
     */
    private suspend fun loadCrossEntitySearch(query: String) {
        if (query.trim().length < 2) {
            _uiState.update { it.copy(searchResult = null) }
            return
        }
        val result = apiClient.search(query)
        result.foldApiError(
            onSuccess = { search -> _uiState.update { it.copy(searchResult = search) } },
            onError = { _uiState.update { it.copy(searchResult = null) } },
        )
    }

    fun onContactClick(contactId: Int) {
        viewModelScope.launch { _events.send(ContactListEvent.NavigateToContact(contactId)) }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null) }
    }

    fun onBulkResultShown() {
        _uiState.update { it.copy(bulkResult = null) }
    }

    private fun handleAuthError(error: ApiError) {
        if (error is ApiError.Client && error.code == 401) {
            viewModelScope.launch { _events.send(ContactListEvent.ForceLogout) }
        }
    }

    companion object {
        private const val SEARCH_DEBOUNCE_MS = 300L
    }
}
