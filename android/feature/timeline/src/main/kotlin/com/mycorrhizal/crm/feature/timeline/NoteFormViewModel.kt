package com.mycorrhizal.crm.feature.timeline

import androidx.annotation.StringRes
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.data.share.ShareDraftHolder
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.NoteRepository
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.model.network.NoteImage
import com.mycorrhizal.crm.model.network.NoteInput
import com.mycorrhizal.crm.model.util.DateFieldValue
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.foldApiError
import com.mycorrhizal.crm.ui.R
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** One picked image awaiting the note id (create mode). */
data class PendingNoteImage(val fileName: String, val mimeType: String, val bytes: ByteArray)

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
    /** The loaded note's stable uuid (edit mode); what a standing share is keyed by. */
    val uuid: String? = null,
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
    val error: ApiError? = null,
    /** ADR 0008: the `revision` the record had when this edit began; sent as `If-Match` on save. */
    val revision: Int? = null,
    /** ADR 0008: the save was rejected with 412 — the record changed elsewhere since it loaded. */
    val conflict: Boolean = false,
    /** ADR 0040: the note's server-side images (edit mode). */
    val images: List<NoteImage> = emptyList(),
    /** Picked in create mode; uploaded after the note exists. */
    val pendingImages: List<PendingNoteImage> = emptyList(),
    val imagesLoading: Boolean = false,
    val imagesBusy: Boolean = false,
    /** #2369: the image whose last sensitivity change failed; its viewer shows the error. */
    val sensitivityFailedImageId: Int? = null,
    /** Issue #1751: the editable state the form opened with, for [isDirty]. */
    val baseline: NoteFormState? = null
) {
    val isEdit: Boolean get() = noteId != null
    val hasContent: Boolean get() = content.isNotBlank()

    val isDirty: Boolean get() = baseline != null && editableSnapshot() != baseline

    private fun editableSnapshot(): NoteFormState = copy(
        targetContactName = null,
        contactSearchQuery = "",
        contactSearchResults = emptyList(),
        contactSearchLoading = false,
        isLoading = false,
        isSaving = false,
        errorRes = null,
        error = null,
        conflict = false,
        images = emptyList(),
        pendingImages = emptyList(),
        imagesLoading = false,
        imagesBusy = false,
        sensitivityFailedImageId = null,
        baseline = null
    )

    fun snapshotForBaseline(): NoteFormState = editableSnapshot()

    /**
     * An image-only note (ADR 0040 decision 11): no text, photos queued in
     * create mode. The server accepts the empty note for a one-hour window
     * while the photos upload.
     */
    val isImageOnlyCreate: Boolean get() = !isEdit && !hasContent && pendingImages.isNotEmpty()

    fun toInput(): NoteInput = NoteInput(
        content = content.trim(),
        date = DateFieldValue.normalizeCalendarDayInstant(date),
        contactId = targetContactId,
        awaitingImages = if (isImageOnlyCreate) true else null
    )

    fun validate(): Int? = when {
        // Edit mode lets the server decide: it accepts empty text only while the
        // note holds a live image (note_requires_content_or_image).
        !hasContent && !isImageOnlyCreate && !isEdit -> R.string.note_error_content

        date.isBlank() -> R.string.note_error_date_required

        // DateField only emits a backend-accepted shape, so no format check.
        else -> null
    }
}

sealed interface NoteFormEvent {
    data object Saved : NoteFormEvent
}

@HiltViewModel
class NoteFormViewModel @Inject constructor(
    private val noteRepository: NoteRepository,
    private val contactRepository: ContactRepository,
    private val shareDrafts: ShareDraftHolder,
    savedStateHandle: SavedStateHandle
) : ViewModel() {

    private val contactId: Int = run {
        val raw: Any? = savedStateHandle["contactId"]
        (raw as? Int) ?: (raw as? String)?.toIntOrNull() ?: 0
    }

    private val noteId: Int? = run {
        val raw: Any? = savedStateHandle["noteId"]
        (raw as? Int) ?: (raw as? String)?.toIntOrNull()
    }

    // ADR 0029 §6 (issue #1271): text shared into the app prefills a *new* note once. The
    // route carries only an opaque key; the text itself was stashed in-memory by the intake
    // and is consumed here, so a second read (recreate, back-stack replay) gets nothing.
    // Edit mode never reads it — a stray key must not overwrite an existing note.
    private val prefill: String? = if (noteId == null) {
        (savedStateHandle.get<String>("prefill"))?.let(shareDrafts::take)
    } else {
        null
    }

    private val _uiState = MutableStateFlow(
        NoteFormState(
            contactId = contactId,
            noteId = noteId,
            content = prefill.orEmpty(),
            targetContactId = contactId.takeIf { it != 0 }
        ).let { it.copy(baseline = it.snapshotForBaseline()) }
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
                onFailure = { _uiState.update { it.copy(targetContactName = "#$id") } }
            )
        }
    }

    fun loadExisting() {
        val id = noteId ?: return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, errorRes = null, error = null) }
            noteRepository.get(id).foldApiError(
                onSuccess = { note ->
                    _uiState.update { state ->
                        val loaded = state.toFormState(note).copy(isLoading = false)
                        loaded.copy(baseline = loaded.snapshotForBaseline())
                    }
                    note.contactId?.let { contactId -> resolveContactName(contactId) }
                    loadImages()
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error) }
                }
            )
        }
    }

    fun onContentChange(value: String) = _uiState.update { it.copy(content = value) }
    fun onDateChange(value: String) = _uiState.update { it.copy(date = value) }
    fun onErrorShown() = _uiState.update { it.copy(errorRes = null, error = null) }

    /** Conflict dialog "Keep editing": leave the form as typed (nothing was saved). */
    fun onConflictKeepEditing() = _uiState.update { it.copy(conflict = false) }

    /** Conflict dialog "Load latest version": discard the edit and refetch the server's copy. */
    fun onConflictLoadLatest() {
        _uiState.update { it.copy(conflict = false) }
        loadExisting()
    }

    /**
     * Debounced contact search for the reassignment picker — same
     * cancel-and-relaunch shape as RelationshipsViewModel.searchContacts.
     */
    fun searchContacts(query: String) {
        _uiState.update { it.copy(contactSearchQuery = query) }
        searchJob?.cancel()
        if (query.isBlank()) {
            _uiState.update {
                it.copy(
                    contactSearchResults = emptyList(),
                    contactSearchLoading = false
                )
            }
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
                }
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
                contactSearchLoading = false
            )
        }
    }

    /** Clear the assignment, moving the note back to the unfiled inbox. */
    fun clearContact() {
        _uiState.update { it.copy(targetContactId = null, targetContactName = null) }
    }

    /**
     * Queue picked images. An edit uploads immediately (the note id exists);
     * a create keeps them pending until [save] creates the note.
     */
    fun addImages(picked: List<PendingNoteImage>) {
        if (picked.isEmpty()) return
        val id = noteId
        if (id == null) {
            _uiState.update { it.copy(pendingImages = it.pendingImages + picked) }
            return
        }
        viewModelScope.launch {
            _uiState.update { it.copy(imagesBusy = true, error = null) }
            var failed: ApiError? = null
            for (image in picked) {
                noteRepository.uploadImage(id, image.fileName, image.mimeType, image.bytes)
                    .onFailure { failed = it as? ApiError }
            }
            _uiState.update { it.copy(imagesBusy = false, error = failed) }
            loadImages()
        }
    }

    /**
     * #2369: set a server-side image's sensitivity (edit mode). Like a delete it
     * applies at once, not on save; the row is replaced with the server's copy.
     */
    fun setImageSensitivity(imageId: Int, sensitivity: String) {
        val id = noteId ?: return
        val current = _uiState.value.images.firstOrNull { it.id == imageId } ?: return
        if (current.sensitivity.orEmpty().ifEmpty { SENSITIVITY_NORMAL } == sensitivity) return
        viewModelScope.launch {
            _uiState.update { it.copy(imagesBusy = true, sensitivityFailedImageId = null) }
            noteRepository.setImageSensitivity(id, imageId, sensitivity).fold(
                onSuccess = { updated ->
                    _uiState.update { state ->
                        state.copy(
                            images = state.images.map { if (it.id == imageId) updated else it }
                        )
                    }
                },
                onFailure = {
                    _uiState.update { it.copy(sensitivityFailedImageId = imageId) }
                }
            )
            _uiState.update { it.copy(imagesBusy = false) }
        }
    }

    /** Remove a server-side image (edit mode). */
    fun deleteImage(imageId: Int) {
        val id = noteId ?: return
        viewModelScope.launch {
            _uiState.update { it.copy(imagesBusy = true, error = null) }
            noteRepository.deleteImage(id, imageId).fold(
                onSuccess = {
                    _uiState.update { state ->
                        state.copy(images = state.images.filterNot { it.id == imageId })
                    }
                },
                onFailure = { error ->
                    _uiState.update { it.copy(error = error as? ApiError) }
                }
            )
            _uiState.update { it.copy(imagesBusy = false) }
        }
    }

    private fun loadImages() {
        val id = noteId ?: return
        viewModelScope.launch {
            _uiState.update { it.copy(imagesLoading = true) }
            val loaded = loadOwnerImages { noteRepository.listImages(id) }
            _uiState.update {
                it.copy(
                    images = loaded.images ?: it.images,
                    imagesLoading = false,
                    error = loaded.error ?: it.error
                )
            }
        }
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
                state.noteId != null -> noteRepository.update(state.noteId, input, state.revision)
                state.targetContactId != null -> noteRepository.create(state.targetContactId, input)
                else -> noteRepository.createUnassigned(input)
            }
            var savedId: Int? = null
            result.foldApiError(
                onSuccess = { saved -> savedId = saved.id },
                onError = { error ->
                    _uiState.update {
                        if (error is ApiError.PreconditionFailed) {
                            it.copy(isSaving = false, conflict = true)
                        } else {
                            it.copy(isSaving = false, error = error)
                        }
                    }
                }
            )
            val id = savedId ?: return@launch
            val pending = _uiState.value.pendingImages
            var failed: ApiError? = null
            if (state.noteId == null && pending.isNotEmpty()) {
                for (image in pending) {
                    noteRepository.uploadImage(
                        id,
                        image.fileName,
                        image.mimeType,
                        image.bytes
                    ).onFailure { failed = it as? ApiError }
                }
            }
            _uiState.update {
                it.copy(
                    isSaving = false,
                    error = failed
                )
            }
            _events.value = NoteFormEvent.Saved
        }
    }

    fun onSaveShown() {
        _events.value = null
    }

    private fun NoteFormState.toFormState(note: Note): NoteFormState = copy(
        uuid = note.uuid,
        content = note.content.orEmpty(),
        date = note.date.orEmpty(),
        targetContactId = note.contactId,
        revision = note.revision,
        // Deliberately NOT read from note.contact — the note API never
        // populates it (zero-valued struct); the name is resolved via
        // ContactRepository by [loadExisting] instead.
        targetContactName = null
    )

    private companion object {
        const val SEARCH_DEBOUNCE_MS = 300L
    }
}
