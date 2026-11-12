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
    val error: ApiError? = null
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
    savedStateHandle: SavedStateHandle
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
            _uiState.update {
                it.copy(
                    isLoading = false,
                    errorRes = R.string.occasions_error_missing_event_id
                )
            }
            return
        }
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null, errorRes = null) }
            val error = fetchDetail()
            _uiState.update { it.copy(isLoading = false, error = error) }
        }
    }

    /** Fetch the event and its roster into state; returns the failure, if any. */
    private suspend fun fetchDetail(): ApiError? {
        var failure: ApiError? = null
        occasionEventRepository.get(eventId).foldApiError(
            onSuccess = { detail ->
                _uiState.update {
                    it.copy(
                        eventTitle = detail.occasionEvent?.title ?: it.eventTitle,
                        attendees = detail.attendees
                    )
                }
            },
            onError = { failure = it }
        )
        return failure
    }

    private fun loadCircles() {
        viewModelScope.launch {
            circleRepository.list().foldApiError(
                onSuccess = { circles -> _uiState.update { it.copy(circles = circles) } },
                onError = { /* Suggestions are optional; a circle-load failure is non-fatal. */ }
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
                onSuccess = { suggestions ->
                    _uiState.update {
                        it.copy(
                            suggestionsLoading = false,
                            suggestions = suggestions
                        )
                    }
                },
                onError = { error ->
                    _uiState.update {
                        it.copy(
                            suggestionsLoading = false,
                            suggestions = emptyList(),
                            error = error
                        )
                    }
                }
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
                            contactResults = page.contacts.filter { c ->
                                c.uid != null &&
                                    c.uid !in attending
                            }
                        )
                    }
                },
                onFailure = {
                    _uiState.update {
                        it.copy(
                            contactSearchLoading = false,
                            contactResults = emptyList()
                        )
                    }
                }
            )
        }
    }

    /** Invite a contact (by VCardUID) and refresh the roster. */
    fun addAttendee(entityId: String) {
        if (eventId.isBlank() || _uiState.value.isMutating) return
        viewModelScope.launch {
            _uiState.update { it.copy(isMutating = true, error = null) }
            occasionEventRepository.addAttendee(
                eventId,
                OccasionEventAttendeeInput(entityId)
            ).foldApiError(
                onSuccess = {
                    _uiState.update {
                        it.copy(
                            isMutating = false,
                            suggestions = emptyList(),
                            contactResults = emptyList(),
                            contactQuery = ""
                        )
                    }
                    load()
                },
                onError = { error ->
                    _uiState.update {
                        it.copy(
                            isMutating = false,
                            error = error
                        )
                    }
                }
            )
        }
    }

    /**
     * Invite every current suggestion (issue #2526; web `handleAddEveryone`). Sequential, and it
     * stops at the first failure so a later add never lands after a failed one. Suggestions that
     * were added leave the list; the failed one and any not yet tried stay so the user can retry.
     * The roster is refreshed either way.
     */
    fun addAllSuggested() {
        val pending = _uiState.value.suggestions
        if (eventId.isBlank() || pending.isEmpty() || _uiState.value.isMutating) return
        viewModelScope.launch {
            _uiState.update { it.copy(isMutating = true, error = null) }
            var remaining = pending
            var failure: ApiError? = null
            for (suggestion in pending) {
                occasionEventRepository.addAttendee(
                    eventId,
                    OccasionEventAttendeeInput(suggestion.entityId)
                ).foldApiError(
                    onSuccess = { remaining = remaining.drop(1) },
                    onError = { failure = it }
                )
                if (failure != null) break
            }
            // Refresh inline (not load()): load() clears `error`, which would erase the failure.
            val refreshError = fetchDetail()
            _uiState.update {
                it.copy(
                    isMutating = false,
                    suggestions = remaining,
                    error = failure ?: refreshError,
                    contactResults = emptyList(),
                    contactQuery = ""
                )
            }
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
                onError = { error ->
                    _uiState.update {
                        it.copy(
                            isMutating = false,
                            error = error
                        )
                    }
                }
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
                onError = { error ->
                    _uiState.update {
                        it.copy(
                            isMutating = false,
                            error = error
                        )
                    }
                }
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
