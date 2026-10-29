package com.mycorrhizal.crm.feature.contacts

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ActivityRepository
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class AddToActivitiesUiState(
    val query: String = "",
    val activities: List<Activity> = emptyList(),
    val isLoading: Boolean = true,
    val selected: Set<Int> = emptySet(),
    val isSubmitting: Boolean = false,
    /** Titles of activities the server reported `not_found` (or an unknown status). */
    val failedTitles: List<String> = emptyList(),
    val error: String? = null,
    /** Every selected activity now has the contact: the host closes the sheet and refreshes. */
    val done: Boolean = false
) {
    val canSubmit: Boolean get() = selected.isNotEmpty() && !isSubmitting
}

/**
 * Issue #2136: "Add to existing activity" picker. Searches activities, multi-selects, and
 * issues exactly one `POST /contacts/{id}/activities`; never a whole-activity PUT.
 */
@HiltViewModel
class AddToActivitiesViewModel @Inject constructor(
    private val activityRepository: ActivityRepository
) : ViewModel() {

    private val _uiState = MutableStateFlow(AddToActivitiesUiState())
    val uiState: StateFlow<AddToActivitiesUiState> = _uiState.asStateFlow()

    private var searchJob: Job? = null

    init {
        search("")
    }

    fun onQueryChange(query: String) {
        _uiState.update { it.copy(query = query) }
        searchJob?.cancel()
        searchJob = viewModelScope.launch {
            delay(SEARCH_DEBOUNCE_MS)
            search(query)
        }
    }

    private fun search(query: String) {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            activityRepository.listAll(
                cursor = null,
                limit = PAGE_SIZE,
                search = query.trim().ifEmpty { null }
            ).foldApiError(
                onSuccess = { page ->
                    _uiState.update { it.copy(isLoading = false, activities = page.activities) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                }
            )
        }
    }

    fun toggle(activityId: Int) {
        _uiState.update { state ->
            val next = state.selected.toMutableSet()
            if (!next.add(activityId)) next.remove(activityId)
            state.copy(selected = next)
        }
    }

    fun submit(contactId: Int) {
        val state = _uiState.value
        if (!state.canSubmit) return
        // Request order = list order, so the single POST is deterministic.
        val ids = state.activities.map { it.id }.filter { it in state.selected } +
            state.selected.filter { id -> state.activities.none { it.id == id } }
        viewModelScope.launch {
            _uiState.update {
                it.copy(
                    isSubmitting = true,
                    error = null,
                    failedTitles = emptyList()
                )
            }
            activityRepository.addContactToActivities(contactId, ids).foldApiError(
                onSuccess = { response ->
                    val failedIds = response.results
                        .filterNot { it.outcome.isSuccess }
                        .map { it.activityId }
                        .toSet()
                    _uiState.update { current ->
                        val titles = failedIds.map { id ->
                            current.activities.firstOrNull { it.id == id }?.title.orEmpty()
                        }
                        current.copy(
                            isSubmitting = false,
                            failedTitles = titles,
                            selected = current.selected.filter { it in failedIds }.toSet(),
                            done = failedIds.isEmpty()
                        )
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(isSubmitting = false, error = error.displayMessage) }
                }
            )
        }
    }

    private companion object {
        const val PAGE_SIZE = 50
        const val SEARCH_DEBOUNCE_MS = 300L
    }
}
