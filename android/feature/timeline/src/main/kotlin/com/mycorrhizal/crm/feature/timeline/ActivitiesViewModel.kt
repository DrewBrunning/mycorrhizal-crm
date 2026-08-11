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
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class ActivitiesUiState(
    val contactId: Int = 0,
    val activities: List<Activity> = emptyList(),
    val isLoading: Boolean = false,
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

    init {
        load()
    }

    fun load() {
        if (contactId == 0) {
            _uiState.update { it.copy(isLoading = false, errorRes = R.string.activity_error_missing_id, error = null) }
            return
        }
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            activityRepository.listForContact(contactId).foldApiError(
                onSuccess = { activities ->
                    _uiState.update { it.copy(isLoading = false, activities = activities) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(errorRes = null, error = null) }
    }
}
