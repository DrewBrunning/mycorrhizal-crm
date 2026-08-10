package com.mycorrhizal.crm.feature.timeline

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ActivityRepository
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.ActivityInput
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
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
 */
data class ActivityFormState(
    val contactId: Int = 0,
    val activityId: Int? = null,
    val title: String = "",
    val type: String = "",
    val date: String = "",
    val description: String = "",
    val location: String = "",
    val isSaving: Boolean = false,
    val error: String? = null,
) {
    val isEdit: Boolean get() = activityId != null
    val hasTitle: Boolean get() = title.isNotBlank()

    fun toInput(): ActivityInput = ActivityInput(
        title = title.trim(),
        type = type.trim().ifBlank { null },
        date = date.trim().ifBlank { null },
        description = description.trim().ifBlank { null },
        location = location.trim().ifBlank { null },
        contactIds = listOf(contactId).takeIf { contactId != 0 },
    )

    fun validate(): String? = when {
        !hasTitle -> "Title is required"
        else -> null
    }
}

sealed interface ActivityFormEvent {
    data object Saved : ActivityFormEvent
}

@HiltViewModel
class ActivityFormViewModel @Inject constructor(
    private val activityRepository: ActivityRepository,
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

    init {
        // Edit mode hydrates from the contact's activities (the backend has no
        // per-activity GET that fits a simple edit flow here; the list carries
        // the full record).
        if (activityId != null) loadExisting()
    }

    fun loadExisting() {
        viewModelScope.launch {
            activityRepository.listForContact(contactId).foldApiError(
                onSuccess = { activities ->
                    activities.firstOrNull { it.id == activityId }?.let { activity ->
                        _uiState.update { it.toFormState(activity) }
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(error = error.displayMessage) }
                },
            )
        }
    }

    fun onTitleChange(value: String) = _uiState.update { it.copy(title = value) }
    fun onTypeChange(value: String) = _uiState.update { it.copy(type = value) }
    fun onDateChange(value: String) = _uiState.update { it.copy(date = value) }
    fun onDescriptionChange(value: String) = _uiState.update { it.copy(description = value) }
    fun onLocationChange(value: String) = _uiState.update { it.copy(location = value) }
    fun onErrorShown() = _uiState.update { it.copy(error = null) }

    fun save() {
        val state = _uiState.value
        if (state.isSaving) return

        val problem = state.validate()
        if (problem != null) {
            _uiState.update { it.copy(error = problem) }
            return
        }

        val input = state.toInput()
        _uiState.update { it.copy(isSaving = true, error = null) }
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
    )
}
