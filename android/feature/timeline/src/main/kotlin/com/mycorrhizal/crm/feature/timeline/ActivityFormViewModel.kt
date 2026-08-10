package com.mycorrhizal.crm.feature.timeline

import androidx.annotation.StringRes
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ActivityRepository
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.ActivityInput
import com.mycorrhizal.crm.network.foldApiError
import com.mycorrhizal.crm.ui.R
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
    val isLoading: Boolean = false,
    val isSaving: Boolean = false,
    @StringRes val errorRes: Int? = null,
    val error: String? = null,
    /** Participant contact IDs carried from the loaded activity. When editing,
     *  the full participant set is preserved (an activity may span contacts);
     *  only a brand-new activity defaults to the current contact. */
    private val participantIds: List<Int> = emptyList(),
    /** Carried from the loaded activity so an edit doesn't wipe an external
     *  ref the form doesn't render (e.g. a calendar/ExternalActivity link). */
    private val externalRef: String? = null,
) {
    val isEdit: Boolean get() = activityId != null
    val hasTitle: Boolean get() = title.isNotBlank()

    fun toInput(): ActivityInput = ActivityInput(
        title = title.trim(),
        type = type.trim().ifBlank { null },
        date = date.trim().ifBlank { null },
        description = description.trim().ifBlank { null },
        location = location.trim().ifBlank { null },
        contactIds = (participantIds.takeIf { it.isNotEmpty() } ?: listOf(contactId).takeIf { contactId != 0 }),
        externalRef = externalRef,
    )

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
        if (activityId != null) loadExisting()
    }

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

    fun onTitleChange(value: String) = _uiState.update { it.copy(title = value) }
    fun onTypeChange(value: String) = _uiState.update { it.copy(type = value) }
    fun onDateChange(value: String) = _uiState.update { it.copy(date = value) }
    fun onDescriptionChange(value: String) = _uiState.update { it.copy(description = value) }
    fun onLocationChange(value: String) = _uiState.update { it.copy(location = value) }
    fun onErrorShown() = _uiState.update { it.copy(errorRes = null, error = null) }

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
        participantIds = activity.contacts.orEmpty().mapNotNull { it.id.takeIf { id -> id != 0 } },
        externalRef = activity.externalRef,
    )
}
