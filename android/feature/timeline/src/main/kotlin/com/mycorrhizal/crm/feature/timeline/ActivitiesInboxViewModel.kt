package com.mycorrhizal.crm.feature.timeline

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ActivityRepository
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

/**
 * M9: the "Activities" drawer entry — every activity across every contact (`GET
 * /activities?include=contacts`), matching web's `ActivitiesPage.tsx`. Distinct from
 * [ActivitiesViewModel], which is the per-contact activities list.
 */
data class ActivitiesInboxUiState(
    val activities: List<Activity> = emptyList(),
    val nextCursor: String? = null,
    val isLoading: Boolean = false,
    val isLoadingMore: Boolean = false,
    /** Id of the activity currently being deleted, so the row can show a spinner. */
    val deletingId: Int? = null,
    val error: String? = null,
)

@HiltViewModel
class ActivitiesInboxViewModel @Inject constructor(
    private val activityRepository: ActivityRepository,
) : ViewModel() {

    private val _uiState = MutableStateFlow(ActivitiesInboxUiState())
    val uiState: StateFlow<ActivitiesInboxUiState> = _uiState.asStateFlow()

    init {
        load()
    }

    fun load() {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            activityRepository.listAll().foldApiError(
                onSuccess = { page ->
                    _uiState.update {
                        it.copy(isLoading = false, activities = page.activities, nextCursor = page.nextCursor)
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun loadMore() {
        val cursor = _uiState.value.nextCursor
        if (cursor.isNullOrEmpty() || _uiState.value.isLoadingMore) return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoadingMore = true, error = null) }
            activityRepository.listAll(cursor = cursor).foldApiError(
                onSuccess = { page ->
                    _uiState.update {
                        it.copy(
                            isLoadingMore = false,
                            activities = it.activities + page.activities,
                            nextCursor = page.nextCursor,
                        )
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoadingMore = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null) }
    }

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
}
