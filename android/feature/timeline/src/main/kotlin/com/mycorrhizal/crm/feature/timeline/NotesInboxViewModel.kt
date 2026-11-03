package com.mycorrhizal.crm.feature.timeline

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.NoteRepository
import com.mycorrhizal.crm.domain.repository.NotesScope
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * The Notes drawer entry, matching web's `NotesPage.tsx`. Issue #2224: it lists every note by
 * default (like Activities and the web page, #2180) with an All notes / Unfiled filter; Unfiled is
 * the N4 inbox (`contact_id IS NULL`). Issue #1757: while the filter is Unfiled the title and empty
 * state say "unassigned" (a user whose notes are all on contacts must not read "No notes yet").
 * Distinct from [NotesViewModel], the per-contact notes list.
 */
data class NotesInboxUiState(
    val scope: NotesScope = NotesScope.ALL,
    val notes: List<Note> = emptyList(),
    val total: Int = 0,
    val nextCursor: String? = null,
    val isLoading: Boolean = false,
    val isLoadingMore: Boolean = false,
    val error: ApiError? = null
)

@HiltViewModel
class NotesInboxViewModel @Inject constructor(private val noteRepository: NoteRepository) :
    ViewModel() {

    private val _uiState = MutableStateFlow(NotesInboxUiState())
    val uiState: StateFlow<NotesInboxUiState> = _uiState.asStateFlow()

    // Declared before `init`: the first load can complete (and set it) before later initializers run.
    private var hasLoaded = false

    init {
        load()
    }

    /** Switch the All notes / Unfiled filter; re-selecting the active one is a no-op. */
    fun setScope(scope: NotesScope) {
        if (scope == _uiState.value.scope) return
        // Clear the list so the previous scope's notes are not shown under the new filter.
        _uiState.update {
            it.copy(
                scope = scope,
                notes = emptyList(),
                total = 0,
                nextCursor = null,
                isLoadingMore = false
            )
        }
        load()
    }

    fun load() {
        val scope = _uiState.value.scope
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            val result = noteRepository.listNotes(scope)
            // The filter changed while this request was in flight: its page is for the old scope.
            if (_uiState.value.scope != scope) return@launch
            result.foldApiError(
                onSuccess = { page ->
                    hasLoaded = true
                    _uiState.update {
                        it.copy(
                            isLoading = false,
                            notes = page.notes,
                            total = page.total,
                            nextCursor = page.nextCursor
                        )
                    }
                },
                onError = { error ->
                    hasLoaded = true
                    _uiState.update { it.copy(isLoading = false, error = error) }
                }
            )
        }
    }

    /**
     * Issue #1757: the screen resumed. After returning from the new-note form the retained list is
     * stale, so reload once a first load has finished (the first resume overlaps the init load and
     * is skipped).
     */
    fun onResumed() {
        val state = _uiState.value
        if (hasLoaded && !state.isLoading) load()
    }

    fun loadMore() {
        val cursor = _uiState.value.nextCursor
        val scope = _uiState.value.scope
        if (cursor.isNullOrEmpty() || _uiState.value.isLoadingMore) return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoadingMore = true, error = null) }
            val result = noteRepository.listNotes(scope, cursor = cursor)
            if (_uiState.value.scope != scope) return@launch
            result.foldApiError(
                onSuccess = { page ->
                    _uiState.update {
                        it.copy(
                            isLoadingMore = false,
                            notes = it.notes + page.notes,
                            total = page.total,
                            nextCursor = page.nextCursor
                        )
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoadingMore = false, error = error) }
                }
            )
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null) }
    }
}
