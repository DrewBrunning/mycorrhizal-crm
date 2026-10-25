package com.mycorrhizal.crm.feature.timeline

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.NoteRepository
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * M9: the unassigned-notes drawer entry — the N4 unfiled-notes inbox (`GET /notes`,
 * `contact_id IS NULL` only), matching web's `NotesPage.tsx`. Distinct from [NotesViewModel], which
 * is the per-contact notes list — this is a contact-agnostic queue, not "all notes across all
 * contacts". Issue #1757: because it is only the unassigned subset, the destination, its title and
 * its empty state all say "unassigned" (a user whose notes are all on contacts must not read
 * "No notes yet").
 */
data class NotesInboxUiState(
    val notes: List<Note> = emptyList(),
    val total: Int = 0,
    val nextCursor: String? = null,
    val isLoading: Boolean = false,
    val isLoadingMore: Boolean = false,
    val error: String? = null
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

    fun load() {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            noteRepository.listUnfiled().foldApiError(
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
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
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
        if (cursor.isNullOrEmpty() || _uiState.value.isLoadingMore) return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoadingMore = true, error = null) }
            noteRepository.listUnfiled(cursor = cursor).foldApiError(
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
                    _uiState.update { it.copy(isLoadingMore = false, error = error.displayMessage) }
                }
            )
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null) }
    }
}
