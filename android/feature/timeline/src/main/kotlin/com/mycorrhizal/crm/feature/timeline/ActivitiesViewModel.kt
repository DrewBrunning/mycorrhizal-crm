package com.mycorrhizal.crm.feature.timeline

import androidx.annotation.StringRes
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ActivityRepository
import com.mycorrhizal.crm.model.network.Activity
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

data class ActivitiesUiState(
    val contactId: Int = 0,
    val activities: List<Activity> = emptyList(),
    /** M19: T17 cursor-pagination state — null/empty means no more pages. */
    val nextCursor: String? = null,
    val searchQuery: String = "",
    /** `YYYY-MM-DD` bounds sent to the server's fromDate/toDate filters. */
    val fromDate: String = "",
    val toDate: String = "",
    val isLoading: Boolean = false,
    val isLoadingMore: Boolean = false,
    /** Id of the activity currently being deleted, so the row can show a spinner. */
    val deletingId: Int? = null,
    @StringRes val errorRes: Int? = null,
    val error: String? = null,
)

@HiltViewModel
class ActivitiesViewModel @Inject constructor(
    private val activityRepository: ActivityRepository,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {

    private val contactId: Int = run {
        val raw: Any? = savedStateHandle["contactId"]
        (raw as? Int) ?: (raw as? String)?.toIntOrNull() ?: 0
    }

    private val _uiState = MutableStateFlow(ActivitiesUiState(contactId = contactId))
    val uiState: StateFlow<ActivitiesUiState> = _uiState.asStateFlow()

    private var reloadJob: Job? = null
    private var loadJob: Job? = null

    init {
        load()
    }

    /**
     * Debounced filter change (search text or a date bound): updates the
     * query state, then reloads the first page once the user stops typing.
     * Mirrors ContactListViewModel's search debounce.
     */
    fun onSearchChange(query: String) {
        _uiState.update { it.copy(searchQuery = query) }
        scheduleReload()
    }

    fun onFromDateChange(value: String) {
        _uiState.update { it.copy(fromDate = value) }
        scheduleReload()
    }

    fun onToDateChange(value: String) {
        _uiState.update { it.copy(toDate = value) }
        scheduleReload()
    }

    fun load() {
        if (contactId == 0) {
            _uiState.update { it.copy(isLoading = false, errorRes = R.string.activity_error_missing_id, error = null) }
            return
        }
        // A superseding load cancels the previous one so an older, slower
        // request can never land last and overwrite a newer filter's results.
        loadJob?.cancel()
        val filters = _uiState.value
        loadJob = viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            activityRepository.listForContact(
                contactId,
                search = filters.searchQuery.ifBlank { null },
                fromDate = filters.fromDate.ifBlank { null },
                toDate = filters.toDate.ifBlank { null },
            ).foldApiError(
                onSuccess = { page ->
                    _uiState.update { it.copy(isLoading = false, activities = page.activities, nextCursor = page.nextCursor) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun loadMore() {
        val state = _uiState.value
        if (state.nextCursor.isNullOrEmpty() || state.isLoadingMore || state.isLoading) return
        loadJob?.cancel()
        loadJob = viewModelScope.launch {
            _uiState.update { it.copy(isLoadingMore = true, error = null) }
            activityRepository.listForContact(
                contactId,
                cursor = state.nextCursor,
                search = state.searchQuery.ifBlank { null },
                fromDate = state.fromDate.ifBlank { null },
                toDate = state.toDate.ifBlank { null },
            ).foldApiError(
                onSuccess = { page ->
                    _uiState.update {
                        it.copy(isLoadingMore = false, activities = it.activities + page.activities, nextCursor = page.nextCursor)
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoadingMore = false, error = error.displayMessage) }
                },
            )
        }
    }

    /** Delete an activity (M19). Confirmation is the screen's job — see M17's rule. */
    fun delete(id: Int) {
        if (_uiState.value.deletingId != null) return
        viewModelScope.launch {
            _uiState.update { it.copy(deletingId = id, error = null) }
            activityRepository.delete(id).foldApiError(
                onSuccess = {
                    _uiState.update { state ->
                        state.copy(deletingId = null, activities = state.activities.filterNot { it.id == id })
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(deletingId = null, error = error.displayMessage) }
                },
            )
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(errorRes = null, error = null) }
    }

    private fun scheduleReload() {
        reloadJob?.cancel()
        reloadJob = viewModelScope.launch {
            delay(SEARCH_DEBOUNCE_MS)
            load()
        }
    }

    private companion object {
        const val SEARCH_DEBOUNCE_MS = 300L
    }
}
