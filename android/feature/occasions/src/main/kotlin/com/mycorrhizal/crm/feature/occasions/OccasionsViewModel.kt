package com.mycorrhizal.crm.feature.occasions

import androidx.annotation.StringRes
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.OccasionEventRepository
import com.mycorrhizal.crm.model.network.OccasionEvent
import com.mycorrhizal.crm.model.network.OccasionEventInput
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class OccasionsUiState(
    val events: List<OccasionEvent> = emptyList(),
    val isLoading: Boolean = false,
    val isMutating: Boolean = false,
    @StringRes val errorRes: Int? = null,
    val error: String? = null,
)

/**
 * The events list (docs/adrs/0026-occasions-events.md, issue #1228): create,
 * edit, delete. Attendee/RSVP management lives in
 * [OccasionAttendeesViewModel], reached per event.
 */
@HiltViewModel
class OccasionsViewModel @Inject constructor(
    private val repository: OccasionEventRepository,
) : ViewModel() {

    private val _uiState = MutableStateFlow(OccasionsUiState())
    val uiState: StateFlow<OccasionsUiState> = _uiState.asStateFlow()

    init {
        load()
    }

    fun load() {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null, errorRes = null) }
            repository.list().foldApiError(
                onSuccess = { events -> _uiState.update { it.copy(isLoading = false, events = events) } },
                onError = { error -> _uiState.update { it.copy(isLoading = false, error = error.displayMessage) } },
            )
        }
    }

    /** Create an event. [startsAt]/[endsAt] are RFC 3339 instants. */
    fun create(
        title: String,
        startsAt: String,
        endsAt: String?,
        location: String?,
        sensitivity: String,
        notes: String?,
    ) {
        if (_uiState.value.isMutating) return
        viewModelScope.launch {
            _uiState.update { it.copy(isMutating = true, error = null) }
            var failed = false
            repository.create(OccasionEventInput(title, startsAt, endsAt, location, sensitivity, notes)).foldApiError(
                onSuccess = { _uiState.update { it.copy(isMutating = false) } },
                onError = { error ->
                    failed = true
                    _uiState.update { it.copy(isMutating = false, error = error.displayMessage) }
                },
            )
            if (!failed) reload()
        }
    }

    /** Update an event (full replace). */
    fun update(
        id: String,
        title: String,
        startsAt: String,
        endsAt: String?,
        location: String?,
        sensitivity: String,
        notes: String?,
    ) {
        if (_uiState.value.isMutating) return
        viewModelScope.launch {
            _uiState.update { it.copy(isMutating = true, error = null) }
            var failed = false
            repository.update(id, OccasionEventInput(title, startsAt, endsAt, location, sensitivity, notes)).foldApiError(
                onSuccess = { _uiState.update { it.copy(isMutating = false) } },
                onError = { error ->
                    failed = true
                    _uiState.update { it.copy(isMutating = false, error = error.displayMessage) }
                },
            )
            if (!failed) reload()
        }
    }

    /** Delete an event (and, server-side, its attendee rows). */
    fun delete(id: String) {
        if (_uiState.value.isMutating) return
        viewModelScope.launch {
            _uiState.update { it.copy(isMutating = true, error = null) }
            var failed = false
            repository.delete(id).foldApiError(
                onSuccess = { _uiState.update { it.copy(isMutating = false) } },
                onError = { error ->
                    failed = true
                    _uiState.update { it.copy(isMutating = false, error = error.displayMessage) }
                },
            )
            if (!failed) reload()
        }
    }

    private suspend fun reload() {
        repository.list().foldApiError(
            onSuccess = { events -> _uiState.update { it.copy(events = events) } },
            onError = { error -> _uiState.update { it.copy(error = error.displayMessage) } },
        )
    }

    fun onErrorShown() {
        _uiState.update { it.copy(errorRes = null, error = null) }
    }
}
