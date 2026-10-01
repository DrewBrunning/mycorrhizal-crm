package com.mycorrhizal.crm.feature.timeline

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ContactTimelineRepository
import com.mycorrhizal.crm.domain.repository.ReminderRepository
import com.mycorrhizal.crm.model.network.TimelineBuckets
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class TimelineExplorerUiState(
    val contactId: Int = 0,
    val items: List<TimelineItem> = emptyList(),
    /** Cursor for the next page; null/empty means the end of the timeline. */
    val nextCursor: String? = null,
    /** Selected event types; empty means all six (the backend treats an absent `type` the same). */
    val types: Set<String> = emptySet(),
    val bucket: String = TimelineBuckets.ALL,
    val isLoading: Boolean = false,
    val isLoadingMore: Boolean = false,
    val error: String? = null,
)

/**
 * Issue #1401 (web T78 `useTimeline` parity): the full, paginated version of a contact's merged
 * timeline, driven by the T66 cursor endpoint. "View all" must not become a second unbounded
 * fetch — the list pages via `next_cursor` ("Load more"), and the type / recency filters are
 * server-side query params, so changing either restarts from the first page.
 */
@HiltViewModel
class TimelineExplorerViewModel @Inject constructor(
    private val timelineRepository: ContactTimelineRepository,
    private val reminderRepository: ReminderRepository,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {

    private val contactId: Int = run {
        val raw: Any? = savedStateHandle["contactId"]
        (raw as? Int) ?: (raw as? String)?.toIntOrNull() ?: 0
    }

    private val _uiState = MutableStateFlow(TimelineExplorerUiState(contactId = contactId))
    val uiState: StateFlow<TimelineExplorerUiState> = _uiState.asStateFlow()

    private var loadJob: Job? = null

    /**
     * Fetch a fresh first page with the current filters. Called on first show and on every
     * return to the screen (an edit/delete made from the explorer's row actions — or from the
     * contact page underneath — must not leave stale rows; web's `revision` counter).
     */
    fun refresh() {
        if (contactId == 0) return
        // A superseding load cancels the previous one so an older, slower response (an earlier
        // filter, or a stale page) can never land last and overwrite newer results.
        loadJob?.cancel()
        val state = _uiState.value
        loadJob = viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, isLoadingMore = false, error = null) }
            timelineRepository.page(contactId, state.types, state.bucket, cursor = null, limit = PAGE_SIZE)
                .foldApiError(
                    onSuccess = { page ->
                        _uiState.update {
                            it.copy(
                                isLoading = false,
                                items = page.items.mapNotNull { event -> event.toTimelineItem() },
                                nextCursor = page.nextCursor,
                            )
                        }
                    },
                    onError = { error -> _uiState.update { it.copy(isLoading = false, error = error.displayMessage) } },
                )
        }
    }

    /** Append the next page. A no-op at the end of the timeline or while another load is in flight. */
    fun loadMore() {
        val state = _uiState.value
        if (state.nextCursor.isNullOrEmpty() || state.isLoading || state.isLoadingMore) return
        loadJob = viewModelScope.launch {
            _uiState.update { it.copy(isLoadingMore = true, error = null) }
            timelineRepository.page(contactId, state.types, state.bucket, cursor = state.nextCursor, limit = PAGE_SIZE)
                .foldApiError(
                    onSuccess = { page ->
                        _uiState.update { current ->
                            val seen = current.items.map { it.key }.toSet()
                            current.copy(
                                isLoadingMore = false,
                                items = current.items +
                                    page.items.mapNotNull { event -> event.toTimelineItem() }.filter { it.key !in seen },
                                nextCursor = page.nextCursor,
                            )
                        }
                    },
                    // Keep the rows already shown; the Load more button stays available to retry.
                    onError = { error ->
                        _uiState.update { it.copy(isLoadingMore = false, error = error.displayMessage) }
                    },
                )
        }
    }

    fun setTypes(types: Set<String>) {
        if (types == _uiState.value.types) return
        _uiState.update { it.copy(types = types) }
        refresh()
    }

    fun setBucket(bucket: String) {
        if (bucket == _uiState.value.bucket) return
        _uiState.update { it.copy(bucket = bucket) }
        refresh()
    }

    /** Undo a completed reminder (delete its completion), then reload so the row disappears. */
    fun undoCompletion(id: Int) {
        viewModelScope.launch {
            reminderRepository.deleteCompletion(id).foldApiError(
                onSuccess = { refresh() },
                onError = { error -> _uiState.update { it.copy(error = error.displayMessage) } },
            )
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null) }
    }

    companion object {
        const val PAGE_SIZE = 25
    }
}
