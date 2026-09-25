package com.mycorrhizal.crm.feature.occasions

import androidx.annotation.StringRes
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.CircleRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.OccasionEventRepository
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.InviteeSuggestion
import com.mycorrhizal.crm.model.network.OccasionEventAttendeeInput
import com.mycorrhizal.crm.model.network.OccasionEventAttendeeView
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

data class OccasionAttendeesUiState(
    val eventId: String = "",
    val eventTitle: String = "",
    val attendees: List<OccasionEventAttendeeView> = emptyList(),
    val isLoading: Boolean = false,
    val isMutating: Boolean = false,
    /** The user's circles, offered as suggestion sources. */
    val circles: List<Circle> = emptyList(),
    val selectedCircleIds: Set<String> = emptySet(),
    val suggestions: List<InviteeSuggestion> = emptyList(),
    val suggestionsLoading: Boolean = false,
    /** Free-text search over the user's contacts to add one directly. */
    val contactQuery: String = "",
    val contactResults: List<ContactSummary> = emptyList(),
    val contactSearchLoading: Boolean = false,
    @StringRes val errorRes: Int? = null,
    val error: String? = null,
)

/**
 * One event's invitee/RSVP ledger (docs/adrs/0026-occasions-events.md, issue
 * #1228). RSVP is what the user reports the contact told them — nothing is
 * sent. Invitees come from three places: an existing attendee list, a
 * circle-based suggestion, or a direct contact search.
 */
@HiltViewModel
class OccasionAttendeesViewModel @Inject constructor(
    private val occasionEventRepository: OccasionEventRepository,
    private val circleRepository: CircleRepository,
    private val contactRepository: ContactRepository,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {

    private val eventId: String = savedStateHandle.get<String>("eventId") ?: ""

    private val _uiState = MutableStateFlow(OccasionAttendeesUiState(eventId = eventId))
    val uiState: StateFlow<OccasionAttendeesUiState> = _uiState.asStateFlow()

    private var searchJob: Job? = null

    init {
        load()
        loadCircles()
    }

    fun load() {
        if (eventId.isBlank()) {
            _uiState.update { it.copy(isLoading = false, errorRes = R.string.occasions_error_missing_event_id) }
            return
        }
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null, errorRes = null) }
            occasionEventRepository.get(eventId).foldApiError(
                onSuccess = { detail ->
                    _uiState.update {
                        it.copy(
                            isLoading = false,
                            eventTitle = detail.occasionEvent?.title ?: it.eventTitle,
                            attendees = detail.attendees,
                        )
                    }
                },
                onError = { error -> _uiState.update { it.copy(isLoading = false, error = error.displayMessage) } },
            )
        }
    }

    private fun loadCircles() {
        viewModelScope.launch {
            circleRepository.list().foldApiError(
                onSuccess = { circles -> _uiState.update { it.copy(circles = circles) } },
                onError = { /* Suggestions are optional; a circle-load failure is non-fatal. */ },
            )
        }
    }

    fun toggleCircle(circleId: String) {
        _uiState.update { state ->
            val next = state.selectedCircleIds.toMutableSet()
            if (!next.add(circleId)) next.remove(circleId)
            state.copy(selectedCircleIds = next, suggestions = emptyList())
        }
    }

    /** Fetch candidate invitees from the selected circles, excluding existing attendees. */
    fun suggest() {
        val selected = _uiState.value.selectedCircleIds
        if (selected.isEmpty() || eventId.isBlank()) {
            _uiState.update { it.copy(suggestions = emptyList()) }
            return
        }
        viewModelScope.launch {
            _uiState.update { it.copy(suggestionsLoading = true, error = null) }
            occasionEventRepository.suggestInvitees(selected.toList(), eventId).foldApiError(
                onSuccess = { suggestions -> _uiState.update { it.copy(suggestionsLoading = false, suggestions = suggestions) } },
                onError = { error ->
                    _uiState.update { it.copy(suggestionsLoading = false, suggestions = emptyList(), error = error.displayMessage) }
                },
            )
        }
    }

    fun clearSuggestions() {
        _uiState.update { it.copy(suggestions = emptyList()) }
    }

    /** Debounced contact search for the direct-add field. */
    fun onContactQueryChange(query: String) {
        _uiState.update { it.copy(contactQuery = query) }
        searchJob?.cancel()
        if (query.isBlank()) {
            _uiState.update { it.copy(contactResults = emptyList(), contactSearchLoading = false) }
            return
        }
        searchJob = viewModelScope.launch {
            delay(SEARCH_DEBOUNCE_MS)
            _uiState.update { it.copy(contactSearchLoading = true) }
            contactRepository.listContacts(search = query, limit = 25).fold(
                onSuccess = { page ->
                    val attending = _uiState.value.attendees.map { it.entityId }.toSet()
                    _uiState.update {
                        it.copy(
                            contactSearchLoading = false,
                            contactResults = page.contacts.filter { c -> c.uid != null && c.uid !in attending },
                        )
                    }
                },
                onFailure = {
                    _uiState.update { it.copy(contactSearchLoading = false, contactResults = emptyList()) }
                },
            )
        }
    }

    /** Invite a contact (by VCardUID) and refresh the roster. */
    fun addAttendee(entityId: String) {
        if (eventId.isBlank() || _uiState.value.isMutating) return
        viewModelScope.launch {
            _uiState.update { it.copy(isMutating = true, error = null) }
            occasionEventRepository.addAttendee(eventId, OccasionEventAttendeeInput(entityId)).foldApiError(
                onSuccess = {
                    _uiState.update { it.copy(isMutating = false, suggestions = emptyList(), contactResults = emptyList(), contactQuery = "") }
                    load()
                },
                onError = { error -> _uiState.update { it.copy(isMutating = false, error = error.displayMessage) } },
            )
        }
    }

    /** Record an attendee's RSVP status. */
    fun updateRsvp(vcardUid: String, rsvp: String) {
        if (eventId.isBlank() || _uiState.value.isMutating) return
        viewModelScope.launch {
            _uiState.update { it.copy(isMutating = true, error = null) }
            occasionEventRepository.updateAttendee(eventId, vcardUid, rsvp).foldApiError(
                onSuccess = {
                    _uiState.update { it.copy(isMutating = false) }
                    load()
                },
                onError = { error -> _uiState.update { it.copy(isMutating = false, error = error.displayMessage) } },
            )
        }
    }

    /** Remove an attendee from the event. */
    fun removeAttendee(vcardUid: String) {
        if (eventId.isBlank() || _uiState.value.isMutating) return
        viewModelScope.launch {
            _uiState.update { it.copy(isMutating = true, error = null) }
            occasionEventRepository.removeAttendee(eventId, vcardUid).foldApiError(
                onSuccess = {
                    _uiState.update { it.copy(isMutating = false) }
                    load()
                },
                onError = { error -> _uiState.update { it.copy(isMutating = false, error = error.displayMessage) } },
            )
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(errorRes = null, error = null) }
    }

    private companion object {
        const val SEARCH_DEBOUNCE_MS = 300L
    }
}
