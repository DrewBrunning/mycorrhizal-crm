package com.mycorrhizal.crm.feature.timeline

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.NoteRepository
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

/**
 * M9: the "Notes" drawer entry — the N4 unfiled-notes inbox (`GET /notes`, `contact_id IS NULL`
 * only), matching web's `NotesPage.tsx`. Distinct from [NotesViewModel], which is the per-contact
 * notes list — this is a contact-agnostic queue, not "all notes across all contacts".
 */
data class NotesInboxUiState(
    val notes: List<Note> = emptyList(),
    val total: Int = 0,
    val nextCursor: String? = null,
    val isLoading: Boolean = false,
    val isLoadingMore: Boolean = false,
    val error: String? = null,
)

@HiltViewModel
class NotesInboxViewModel @Inject constructor(
    private val noteRepository: NoteRepository,
) : ViewModel() {

    private val _uiState = MutableStateFlow(NotesInboxUiState())
    val uiState: StateFlow<NotesInboxUiState> = _uiState.asStateFlow()

    init {
        load()
    }

    fun load() {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            noteRepository.listUnfiled().foldApiError(
                onSuccess = { page ->
                    _uiState.update {
                        it.copy(isLoading = false, notes = page.notes, total = page.total, nextCursor = page.nextCursor)
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
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
                            nextCursor = page.nextCursor,
                        )
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoadingMore = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null) }
    }
}
