package com.mycorrhizal.crm.feature.timeline

import androidx.annotation.StringRes
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.NoteRepository
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.network.foldApiError
import com.mycorrhizal.crm.ui.R
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class NotesUiState(
    val contactId: Int = 0,
    val notes: List<Note> = emptyList(),
    val isLoading: Boolean = false,
    @StringRes val errorRes: Int? = null,
    val error: String? = null,
)


@HiltViewModel
class NotesViewModel @Inject constructor(
    private val noteRepository: NoteRepository,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {

    private val contactId: Int = run {
        val raw: Any? = savedStateHandle["contactId"]
        (raw as? Int) ?: (raw as? String)?.toIntOrNull() ?: 0
    }

    private val _uiState = MutableStateFlow(NotesUiState(contactId = contactId))
    val uiState: StateFlow<NotesUiState> = _uiState.asStateFlow()

    init {
        load()
    }

    fun load() {
        if (contactId == 0) {
            _uiState.update { it.copy(isLoading = false, errorRes = R.string.note_error_missing_id, error = null) }
            return
        }
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            noteRepository.listForContact(contactId).foldApiError(
                onSuccess = { notes ->
                    _uiState.update { it.copy(isLoading = false, notes = notes) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(errorRes = null, error = null) }
    }
}
