package com.mycorrhizal.crm.feature.contacts

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.ContactsPage
import com.mycorrhizal.crm.model.network.ContactSummary
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
)

sealed interface ContactListEvent {
    data class NavigateToContact(val contactId: Int) : ContactListEvent
    data object ForceLogout : ContactListEvent
}

@HiltViewModel
class ContactListViewModel @Inject constructor(
    private val contactRepository: ContactRepository,
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
                _uiState.update {
                    if (it.contacts.isEmpty()) it.copy(contacts = cached) else it
                }
            }
        }
        loadContacts()
    }

    fun loadContacts() {
        val state = _uiState.value
        if (state.isLoading) return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null, contacts = emptyList()) }
            val page = contactRepository.listContacts(
                cursor = null,
                limit = _uiState.value.pagination.limit,
                search = _uiState.value.searchQuery.takeIf { it.isNotBlank() },
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

    private var searchJob: Job? = null

    fun onSearchQueryChange(query: String) {
        _uiState.update { it.copy(searchQuery = query) }
        // Debounce keystrokes so a fast typist doesn't fire one request per
        // character; also reload immediately when cleared.
        searchJob?.cancel()
        searchJob = viewModelScope.launch {
            if (query.isBlank()) {
                loadContacts()
            } else {
                delay(SEARCH_DEBOUNCE_MS)
                loadContacts()
            }
        }
    }

    fun onContactClick(contactId: Int) {
        viewModelScope.launch { _events.send(ContactListEvent.NavigateToContact(contactId)) }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null) }
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
