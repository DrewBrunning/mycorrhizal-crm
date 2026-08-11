package com.mycorrhizal.crm.feature.timeline

import androidx.annotation.StringRes
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.NoteRepository
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.model.network.NoteInput
import com.mycorrhizal.crm.network.foldApiError
import com.mycorrhizal.crm.ui.R
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

/**
 * Editable form state for a note. `content` and `date` are required by the
 * backend; the date uses the same loose ISO-8601 check as activities.
 */
data class NoteFormState(
    val contactId: Int = 0,
    val noteId: Int? = null,
    val content: String = "",
    val date: String = "",
    val isLoading: Boolean = false,
    val isSaving: Boolean = false,
    @StringRes val errorRes: Int? = null,
    val error: String? = null,
) {
    val isEdit: Boolean get() = noteId != null
    val hasContent: Boolean get() = content.isNotBlank()

    fun toInput(): NoteInput = NoteInput(
        content = content.trim(),
        date = date.trim().ifBlank { null },
        contactId = contactId.takeIf { it != 0 },
    )

    fun validate(): Int? = when {
        !hasContent -> R.string.note_error_content
        date.isBlank() -> R.string.note_error_date_required
        date.isNotBlank() && !date.matches(ISO_DATETIME_REGEX) -> R.string.note_error_date
        else -> null
    }

    companion object {
        val ISO_DATETIME_REGEX = Regex(
            """\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})""",
        )
    }
}

sealed interface NoteFormEvent {
    data object Saved : NoteFormEvent
}

@HiltViewModel
class NoteFormViewModel @Inject constructor(
    private val noteRepository: NoteRepository,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {

    private val contactId: Int = run {
        val raw: Any? = savedStateHandle["contactId"]
        (raw as? Int) ?: (raw as? String)?.toIntOrNull() ?: 0
    }

    private val noteId: Int? = run {
        val raw: Any? = savedStateHandle["noteId"]
        (raw as? Int) ?: (raw as? String)?.toIntOrNull()
    }

    private val _uiState = MutableStateFlow(NoteFormState(contactId = contactId, noteId = noteId))
    val uiState: StateFlow<NoteFormState> = _uiState.asStateFlow()

    private val _events = MutableStateFlow<NoteFormEvent?>(null)
    val events: StateFlow<NoteFormEvent?> = _events

    init {
        if (noteId != null) loadExisting()
    }

    fun loadExisting() {
        val id = noteId ?: return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, errorRes = null, error = null) }
            noteRepository.get(id).foldApiError(
                onSuccess = { note ->
                    _uiState.update { it.toFormState(note).copy(isLoading = false) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun onContentChange(value: String) = _uiState.update { it.copy(content = value) }
    fun onDateChange(value: String) = _uiState.update { it.copy(date = value) }
    fun onErrorShown() = _uiState.update { it.copy(errorRes = null, error = null) }

    fun save() {
        val state = _uiState.value
        if (state.isSaving) return

        val problem = state.validate()
        if (problem != null) {
            _uiState.update { it.copy(errorRes = problem, error = null) }
            return
        }

        val input = state.toInput()
        _uiState.update { it.copy(isSaving = true, errorRes = null, error = null) }
        viewModelScope.launch {
            val result = if (state.noteId != null) {
                noteRepository.update(state.noteId, input)
            } else {
                noteRepository.create(state.contactId, input)
            }
            result.foldApiError(
                onSuccess = { _uiState.update { it.copy(isSaving = false) }; _events.value = NoteFormEvent.Saved },
                onError = { error ->
                    _uiState.update { it.copy(isSaving = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun onSaveShown() {
        _events.value = null
    }

    private fun NoteFormState.toFormState(note: Note): NoteFormState = copy(
        content = note.content.orEmpty(),
        date = note.date.orEmpty(),
    )
}
