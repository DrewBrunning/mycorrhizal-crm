package com.mycorrhizal.crm.feature.timeline

import androidx.annotation.StringRes
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.NoteRepository
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.model.network.NoteInput
import com.mycorrhizal.crm.network.foldApiError
import com.mycorrhizal.crm.ui.R
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

/**
 * Editable form state for a note. `content` and `date` are required by the
 * backend; the date uses the same loose ISO-8601 check as activities.
 *
 * M19 contact reassignment: [targetContactId] is the note's contact — the
 * route's contact in create mode, the loaded note's contact in edit mode —
 * and may be changed to any contact (or cleared to "unfiled") via
 * [NoteFormViewModel.searchContacts]/[NoteFormViewModel.selectContact].
 */
data class NoteFormState(
    val contactId: Int = 0,
    val noteId: Int? = null,
    val content: String = "",
    val date: String = "",
    /** Currently selected target contact; null means an unassigned note. */
    val targetContactId: Int? = null,
    /** Display name of [targetContactId], for the selected-contact chip. */
    val targetContactName: String? = null,
    val contactSearchQuery: String = "",
    val contactSearchResults: List<ContactSummary> = emptyList(),
    val contactSearchLoading: Boolean = false,
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
        contactId = targetContactId,
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
    private val contactRepository: ContactRepository,
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

    private val _uiState = MutableStateFlow(
        NoteFormState(
            contactId = contactId,
            noteId = noteId,
            targetContactId = contactId.takeIf { it != 0 },
        ),
    )
    val uiState: StateFlow<NoteFormState> = _uiState.asStateFlow()

    private val _events = MutableStateFlow<NoteFormEvent?>(null)
    val events: StateFlow<NoteFormEvent?> = _events

    private var searchJob: Job? = null

    init {
        if (noteId != null) {
            loadExisting()
        } else {
            resolveRouteContact()
        }
    }

    /**
     * Create mode: resolve the route contact's name so the reassignment chip
     * shows who the note will be attached to. Best-effort — a failure falls
     * back to a bare "#id" label.
     */
    private fun resolveRouteContact() {
        if (contactId == 0) return
        resolveContactName(contactId)
    }

    /**
     * Best-effort name resolution for a contact id. The note API never
     * populates a note's nested `contact` (no Preload in GetNote/UpdateNote/
     * CreateNote — it serializes a zero-valued struct), so a note's assigned
     * contact name can't be read off the note itself; it must be resolved
     * here, the same way create mode resolves the route contact. A failure
     * falls back to a bare "#id" chip, which still carries the correct
     * targetContactId for saving.
     */
    private fun resolveContactName(id: Int) {
        viewModelScope.launch {
            contactRepository.getContact(id).fold(
                onSuccess = { record ->
                    val name = record.card?.displayName?.takeIf { it.isNotBlank() } ?: "#$id"
                    _uiState.update { it.copy(targetContactName = name) }
                },
                onFailure = { _uiState.update { it.copy(targetContactName = "#$id") } },
            )
        }
    }

    fun loadExisting() {
        val id = noteId ?: return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, errorRes = null, error = null) }
            noteRepository.get(id).foldApiError(
                onSuccess = { note ->
                    _uiState.update { it.toFormState(note).copy(isLoading = false) }
                    note.contactId?.let { contactId -> resolveContactName(contactId) }
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

    /**
     * Debounced contact search for the reassignment picker — same
     * cancel-and-relaunch shape as RelationshipsViewModel.searchContacts.
     */
    fun searchContacts(query: String) {
        _uiState.update { it.copy(contactSearchQuery = query) }
        searchJob?.cancel()
        if (query.isBlank()) {
            _uiState.update { it.copy(contactSearchResults = emptyList(), contactSearchLoading = false) }
            return
        }
        searchJob = viewModelScope.launch {
            delay(SEARCH_DEBOUNCE_MS)
            _uiState.update { it.copy(contactSearchLoading = true) }
            contactRepository.listContacts(search = query, limit = 25).fold(
                onSuccess = { page ->
                    _uiState.update {
                        it.copy(contactSearchResults = page.contacts, contactSearchLoading = false)
                    }
                },
                onFailure = {
                    _uiState.update { it.copy(contactSearchLoading = false) }
                },
            )
        }
    }

    /** Reassign the note to [contact] (matching web's EditTimelineItemDialog). */
    fun selectContact(contact: ContactSummary) {
        _uiState.update {
            it.copy(
                targetContactId = contact.id,
                targetContactName = contact.displayName,
                contactSearchQuery = "",
                contactSearchResults = emptyList(),
                contactSearchLoading = false,
            )
        }
    }

    /** Clear the assignment, moving the note back to the unfiled inbox. */
    fun clearContact() {
        _uiState.update { it.copy(targetContactId = null, targetContactName = null) }
    }

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
            val result = when {
                state.noteId != null -> noteRepository.update(state.noteId, input)
                state.targetContactId != null -> noteRepository.create(state.targetContactId, input)
                else -> noteRepository.createUnassigned(input)
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
        targetContactId = note.contactId,
        // Deliberately NOT read from note.contact — the note API never
        // populates it (zero-valued struct); the name is resolved via
        // ContactRepository by [loadExisting] instead.
        targetContactName = null,
    )

    private companion object {
        const val SEARCH_DEBOUNCE_MS = 300L
    }
}
