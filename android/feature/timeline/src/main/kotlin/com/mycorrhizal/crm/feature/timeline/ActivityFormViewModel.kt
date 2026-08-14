package com.mycorrhizal.crm.feature.timeline

import androidx.annotation.StringRes
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ActivityRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.ActivityInput
import com.mycorrhizal.crm.model.network.ContactFlat
import com.mycorrhizal.crm.model.network.ContactSummary
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
    val description: String = "",
    val location: String = "",
    /** Participant contacts, all sent on save (M19 — the one-per-route-contact
     *  limit was the bug). In create mode this is seeded with the route contact. */
    val participants: List<ContactFlat> = emptyList(),
    val contactSearchQuery: String = "",
    val contactSearchResults: List<ContactSummary> = emptyList(),
    val contactSearchLoading: Boolean = false,
    val isLoading: Boolean = false,
    val isSaving: Boolean = false,
    @StringRes val errorRes: Int? = null,
    val error: String? = null,
    /** Carried from the loaded activity so an edit doesn't wipe an external
     *  ref the form doesn't render (e.g. a calendar/ExternalActivity link). */
    val externalRef: String? = null,
) {
    val isEdit: Boolean get() = activityId != null
    val hasTitle: Boolean get() = title.isNotBlank()

    fun toInput(): ActivityInput = ActivityInput(
        title = title.trim(),
        type = type.trim().ifBlank { null },
        date = date.trim().ifBlank { null },
        description = description.trim().ifBlank { null },
        location = location.trim().ifBlank { null },
        contactIds = participantIds(),
        externalRef = externalRef,
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
        date.isNotBlank() && !date.matches(ISO_DATETIME_REGEX) -> R.string.activity_error_date
        else -> null
    }

    companion object {
        /** Loose RFC 3339 / ISO 8601 date-time check matching the backend's
         *  time.Time unmarshal (which requires a 'T' separator and offset). */
        val ISO_DATETIME_REGEX = Regex(
            """\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})""",
        )
    }
}

sealed interface ActivityFormEvent {
    data object Saved : ActivityFormEvent
}

@HiltViewModel
class ActivityFormViewModel @Inject constructor(
    private val activityRepository: ActivityRepository,
    private val contactRepository: ContactRepository,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {

    private val contactId: Int = run {
        val raw: Any? = savedStateHandle["contactId"]
        (raw as? Int) ?: (raw as? String)?.toIntOrNull() ?: 0
    }

    private val activityId: Int? = run {
        val raw: Any? = savedStateHandle["activityId"]
        (raw as? Int) ?: (raw as? String)?.toIntOrNull()
    }

    private val _uiState = MutableStateFlow(ActivityFormState(contactId = contactId, activityId = activityId))
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
                    _uiState.update { it.toFormState(activity).copy(isLoading = false) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
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
                    _uiState.update { it.copy(participants = listOf(ContactFlat(id = contactId, firstname = name))) }
                },
                onFailure = { _uiState.update { it.copy(participants = listOf(fallback)) } },
            )
        }
    }

    fun onTitleChange(value: String) = _uiState.update { it.copy(title = value) }
    fun onTypeChange(value: String) = _uiState.update { it.copy(type = value) }
    fun onDateChange(value: String) = _uiState.update { it.copy(date = value) }
    fun onDescriptionChange(value: String) = _uiState.update { it.copy(description = value) }
    fun onLocationChange(value: String) = _uiState.update { it.copy(location = value) }
    fun onErrorShown() = _uiState.update { it.copy(errorRes = null, error = null) }

    /**
     * Debounced contact search for the participant picker — same
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
                    uid = contact.uid,
                ),
                contactSearchQuery = "",
                contactSearchResults = emptyList(),
                contactSearchLoading = false,
            )
        }
    }

    /** Remove a participant by id. */
    fun onRemoveParticipant(id: Int) {
        _uiState.update { it.copy(participants = it.participants.filterNot { p -> p.id == id }) }
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
            val result = if (state.activityId != null) {
                activityRepository.update(state.activityId, input)
            } else {
                activityRepository.create(input)
            }
            result.foldApiError(
                onSuccess = { _uiState.update { it.copy(isSaving = false) }; _events.value = ActivityFormEvent.Saved },
                onError = { error ->
                    _uiState.update { it.copy(isSaving = false, error = error.displayMessage) }
                },
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
        description = activity.description.orEmpty(),
        location = activity.location.orEmpty(),
        participants = activity.contacts.orEmpty(),
        externalRef = activity.externalRef,
    )

    private companion object {
        const val SEARCH_DEBOUNCE_MS = 300L
    }
}
