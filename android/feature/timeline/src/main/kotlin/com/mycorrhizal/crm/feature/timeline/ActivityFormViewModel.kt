package com.mycorrhizal.crm.feature.timeline

import androidx.annotation.StringRes
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ActivityRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.ActivityImage
import com.mycorrhizal.crm.model.network.ActivityInput
import com.mycorrhizal.crm.model.network.ContactFlat
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.ErrorReasonCode
import com.mycorrhizal.crm.model.util.ActivityDateRange
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

/** One picked image awaiting the activity id (create mode). */
data class PendingActivityImage(val fileName: String, val mimeType: String, val bytes: ByteArray)

/**
 * Editable form state for an activity (Interaction). The `type` classifier is
 * open (the backend does not validate it), so the form offers the well-known
 * InteractionType tokens but the field is free-text.
 *
 * M19 multi-participant: [participants] is the full participant set — the
 * loaded activity's contacts in edit mode, the route contact in create mode —
 * and can be extended/removed via the picker, so an activity created or
 * edited on Android can represent more than one contact.
 */
data class ActivityFormState(
    val contactId: Int = 0,
    val activityId: Int? = null,
    val title: String = "",
    val type: String = "",
    val date: String = "",
    /** Issue #2006: optional last day of a multi-day activity (UTC-midnight instant), "" for none. */
    val endDate: String = "",
    /** The server refused [endDate] as before the start (`activity_end_before_start`). */
    val endDateRejected: Boolean = false,
    val description: String = "",
    val location: String = "",
    /** Participant contacts, all sent on save (M19 — the one-per-route-contact
     *  limit was the bug). In create mode this is seeded with the route contact. */
    val participants: List<ContactFlat> = emptyList(),
    /** ADR 0040: the activity's server-side images (edit mode). */
    val images: List<ActivityImage> = emptyList(),
    /** Picked in create mode; uploaded after the activity exists. */
    val pendingImages: List<PendingActivityImage> = emptyList(),
    val imagesLoading: Boolean = false,
    val imagesBusy: Boolean = false,
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
    /** Carried from the loaded activity so an edit doesn't wipe an external
     *  ref the form doesn't render (e.g. a calendar/ExternalActivity link). */
    val externalRef: String? = null,
    /** Issue #1751: the editable state the form opened with, for [isDirty]. */
    val baseline: ActivityFormState? = null
) {
    val isEdit: Boolean get() = activityId != null
    val hasTitle: Boolean get() = title.isNotBlank()

    /** Issue #2006: the picked end day precedes the start day (equal days are fine). */
    val endBeforeStart: Boolean get() = ActivityDateRange.endBeforeStart(date, endDate)

    /** The end field shows "cannot be before the start date" (client check or server refusal). */
    val endDateError: Boolean get() = endBeforeStart || endDateRejected

    val isDirty: Boolean get() = baseline != null && editableSnapshot() != baseline

    private fun editableSnapshot(): ActivityFormState = copy(
        // Search/loading state is transient; participant display names resolve
        // asynchronously, so compare the ids that a save would actually send.
        participants = effectiveParticipants().map { ContactFlat(id = it.id) },
        contactSearchQuery = "",
        contactSearchResults = emptyList(),
        contactSearchLoading = false,
        isLoading = false,
        isSaving = false,
        errorRes = null,
        error = null,
        endDateRejected = false,
        conflict = false,
        images = emptyList(),
        imagesLoading = false,
        imagesBusy = false,
        baseline = null
    )

    fun snapshotForBaseline(): ActivityFormState = editableSnapshot()

    private fun effectiveParticipants(): List<ContactFlat> = if (isEdit) {
        participants
    } else {
        participants.ifEmpty {
            listOf(
                ContactFlat(id = contactId)
            ).filter { it.id != 0 }
        }
    }

    fun toInput(): ActivityInput = ActivityInput(
        title = title.trim(),
        type = type.trim().ifBlank { null },
        date = DateFieldValue.normalizeCalendarDayInstant(date),
        // Always present: an update with a null end clears the stored one (omitting keeps it).
        endDate = DateFieldValue.normalizeCalendarDayInstant(endDate),
        description = description.trim().ifBlank { null },
        location = location.trim().ifBlank { null },
        contactIds = participantIds(),
        externalRef = externalRef
    )

    /**
     * The participant ids to send on save. The route-contact fallback exists
     * ONLY in create mode (a brand-new activity from a contact's page is
     * about that contact — the pre-M19 default). In edit mode removing every
     * participant must clear the set (`contact_ids: []`, which the backend
     * `UpdateActivity` honors as `Association.Replace(nil)`), not silently
     * re-add the route contact or, from the inbox route, send `null` and
     * change nothing.
     */
    private fun participantIds(): List<Int>? = if (isEdit) {
        participants.map { it.id }
    } else {
        participants.map { it.id }.ifEmpty { listOf(contactId).takeIf { contactId != 0 } }
    }

    fun validate(): Int? = when {
        !hasTitle -> R.string.activity_error_title

        // The backend requires Activity.Date; the picker makes it explicit.
        date.isBlank() -> R.string.activity_error_date_required

        // DateField only emits a backend-accepted shape (RFC 3339, normalised
        // to the UTC-midnight instant in toInput), so no format re-validation.
        else -> null
    }
}

sealed interface ActivityFormEvent {
    data object Saved : ActivityFormEvent
}

@HiltViewModel
class ActivityFormViewModel @Inject constructor(
    private val activityRepository: ActivityRepository,
    private val contactRepository: ContactRepository,
    private val savedStateHandle: SavedStateHandle
) : ViewModel() {

    private val contactId: Int = run {
        val raw: Any? = savedStateHandle["contactId"]
        (raw as? Int) ?: (raw as? String)?.toIntOrNull() ?: 0
    }

    private val activityId: Int? = run {
        val raw: Any? = savedStateHandle["activityId"]
        (raw as? Int) ?: (raw as? String)?.toIntOrNull()
    }

    // Issue #160 (ADR 0033): "Log from location history" opens the create form pre-filled from a
    // stay. Create mode only — an edit hydrates from the loaded activity. `externalRef` rides
    // through to save untouched (it is not an editable field); the server dedupes geopulse:stay refs.
    private fun prefillArg(key: String): String? =
        (savedStateHandle.get<Any?>(key) as? String)?.takeIf { it.isNotBlank() }

    private val _uiState = MutableStateFlow(
        ActivityFormState(contactId = contactId, activityId = activityId).let { base ->
            val initial = if (activityId != null) {
                base
            } else {
                base.copy(
                    location = prefillArg(PREFILL_LOCATION).orEmpty(),
                    date = prefillArg(PREFILL_DATE).orEmpty(),
                    externalRef = prefillArg(PREFILL_EXTERNAL_REF)
                )
            }
            initial.copy(baseline = initial.snapshotForBaseline())
        }
    )
    val uiState: StateFlow<ActivityFormState> = _uiState.asStateFlow()

    private val _events = MutableStateFlow<ActivityFormEvent?>(null)
    val events: StateFlow<ActivityFormEvent?> = _events

    private var searchJob: Job? = null

    init {
        if (activityId != null) {
            loadExisting()
        } else {
            resolveRouteContact()
        }
    }

    /** Edit mode: hydrate the whole form, including the full participant set. */
    fun loadExisting() {
        val id = activityId ?: return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, errorRes = null, error = null) }
            activityRepository.get(id).foldApiError(
                onSuccess = { activity ->
                    _uiState.update { state ->
                        val loaded = state.toFormState(activity).copy(isLoading = false)
                        loaded.copy(baseline = loaded.snapshotForBaseline())
                    }
                    loadImages()
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error) }
                }
            )
        }
    }

    /**
     * Create mode: seed the participant list with the route contact so the
     * new activity isn't accidentally contact-less. Best-effort name
     * resolution — a failure still seeds the participant, just with a bare
     * "#id" chip.
     */
    private fun resolveRouteContact() {
        if (contactId == 0) return
        viewModelScope.launch {
            val fallback = ContactFlat(id = contactId)
            contactRepository.getContact(contactId).fold(
                onSuccess = { record ->
                    val name = record.card?.displayName?.takeIf { it.isNotBlank() } ?: "#$contactId"
                    _uiState.update {
                        it.copy(
                            participants = listOf(ContactFlat(id = contactId, firstname = name))
                        )
                    }
                },
                onFailure = { _uiState.update { it.copy(participants = listOf(fallback)) } }
            )
        }
    }

    fun onTitleChange(value: String) = _uiState.update { it.copy(title = value) }
    fun onTypeChange(value: String) = _uiState.update { it.copy(type = value) }
    fun onDateChange(value: String) =
        _uiState.update { it.copy(date = value, endDateRejected = false) }
    fun onEndDateChange(value: String) =
        _uiState.update { it.copy(endDate = value, endDateRejected = false) }
    fun onDescriptionChange(value: String) = _uiState.update { it.copy(description = value) }
    fun onLocationChange(value: String) = _uiState.update { it.copy(location = value) }
    fun onErrorShown() = _uiState.update { it.copy(errorRes = null, error = null) }

    /** Conflict dialog "Keep editing": leave the form as typed (nothing was saved). */
    fun onConflictKeepEditing() = _uiState.update { it.copy(conflict = false) }

    /** Conflict dialog "Load latest version": discard the edit and refetch the server's copy. */
    fun onConflictLoadLatest() {
        _uiState.update { it.copy(conflict = false) }
        loadExisting()
    }

    /**
     * Debounced contact search for the participant picker — same
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

    /** Add a participant (no-op if already in the set). */
    fun onAddParticipant(contact: ContactSummary) {
        if (_uiState.value.participants.any { it.id == contact.id }) return
        _uiState.update {
            it.copy(
                participants = it.participants + ContactFlat(
                    id = contact.id,
                    firstname = contact.firstname,
                    lastname = contact.lastname,
                    nickname = contact.nickname,
                    uid = contact.uid
                ),
                contactSearchQuery = "",
                contactSearchResults = emptyList(),
                contactSearchLoading = false
            )
        }
    }

    /** Remove a participant by id. */
    fun onRemoveParticipant(id: Int) {
        _uiState.update { it.copy(participants = it.participants.filterNot { p -> p.id == id }) }
    }

    private fun loadImages() {
        val id = activityId ?: return
        viewModelScope.launch {
            _uiState.update { it.copy(imagesLoading = true) }
            runCatching { activityRepository.listImages(id) }.getOrNull()?.fold(
                onSuccess = { images ->
                    _uiState.update { it.copy(images = images, imagesLoading = false) }
                },
                onFailure = { _uiState.update { it.copy(imagesLoading = false) } }
            ) ?: _uiState.update { it.copy(imagesLoading = false) }
        }
    }

    /**
     * Queue picked images. An edit uploads immediately (the activity id exists);
     * a create keeps them pending until [save] creates the activity.
     */
    fun addImages(picked: List<PendingActivityImage>) {
        if (picked.isEmpty()) return
        val id = activityId
        if (id == null) {
            _uiState.update { it.copy(pendingImages = it.pendingImages + picked) }
            return
        }
        viewModelScope.launch {
            _uiState.update { it.copy(imagesBusy = true, error = null) }
            var failed: ApiError? = null
            for (image in picked) {
                activityRepository.uploadImage(id, image.fileName, image.mimeType, image.bytes)
                    .onFailure { failed = it as? ApiError }
            }
            _uiState.update { it.copy(imagesBusy = false, error = failed) }
            loadImages()
        }
    }

    /** Remove a server-side image (edit mode). */
    fun deleteImage(imageId: Int) {
        val id = activityId ?: return
        viewModelScope.launch {
            _uiState.update { it.copy(imagesBusy = true, error = null) }
            activityRepository.deleteImage(id, imageId).fold(
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

    fun save() {
        val state = _uiState.value
        if (state.isSaving) return

        // Issue #2006: a reversed range is already flagged on the end field; don't also toast.
        if (state.endBeforeStart) return

        val problem = state.validate()
        if (problem != null) {
            _uiState.update { it.copy(errorRes = problem, error = null) }
            return
        }

        val input = state.toInput()
        _uiState.update { it.copy(isSaving = true, errorRes = null, error = null) }
        viewModelScope.launch {
            val result = if (state.activityId != null) {
                activityRepository.update(state.activityId, input, state.revision)
            } else {
                activityRepository.create(input)
            }
            result.foldApiError(
                onSuccess = { saved ->
                    val pending = _uiState.value.pendingImages
                    if (state.activityId == null && pending.isNotEmpty()) {
                        viewModelScope.launch {
                            for (image in pending) {
                                activityRepository.uploadImage(
                                    saved.id,
                                    image.fileName,
                                    image.mimeType,
                                    image.bytes
                                )
                            }
                        }
                    }
                    _uiState.update {
                        it.copy(
                            isSaving = false
                        )
                    }
                    _events.value = ActivityFormEvent.Saved
                },
                onError = { error ->
                    _uiState.update {
                        if (error is ApiError.PreconditionFailed) {
                            it.copy(isSaving = false, conflict = true)
                        } else if (error.isEndBeforeStart()) {
                            // Shown on the end field itself, not as a generic snackbar.
                            it.copy(isSaving = false, endDateRejected = true)
                        } else {
                            it.copy(isSaving = false, error = error)
                        }
                    }
                }
            )
        }
    }

    fun onSaveShown() {
        _events.value = null
    }

    private fun ActivityFormState.toFormState(activity: Activity): ActivityFormState = copy(
        title = activity.title.orEmpty(),
        type = activity.type.orEmpty(),
        date = activity.date.orEmpty(),
        endDate = activity.endDate.orEmpty(),
        endDateRejected = false,
        description = activity.description.orEmpty(),
        location = activity.location.orEmpty(),
        participants = activity.contacts.orEmpty(),
        externalRef = activity.externalRef,
        revision = activity.revision
    )

    private fun ApiError.isEndBeforeStart(): Boolean =
        this is ApiError.Client && reason?.code == ErrorReasonCode.ACTIVITY_END_BEFORE_START

    companion object {
        private const val SEARCH_DEBOUNCE_MS = 300L

        /** Nav-arg keys for the optional create-mode prefill (issue #160). */
        const val PREFILL_LOCATION = "location"
        const val PREFILL_DATE = "date"
        const val PREFILL_EXTERNAL_REF = "externalRef"
    }
}
