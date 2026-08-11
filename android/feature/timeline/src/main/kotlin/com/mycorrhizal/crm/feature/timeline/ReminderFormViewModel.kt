package com.mycorrhizal.crm.feature.timeline

import androidx.annotation.StringRes
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ReminderRepository
import com.mycorrhizal.crm.model.network.Reminder
import com.mycorrhizal.crm.model.network.ReminderRecurrence
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
 * Editable form state for a reminder. `message`, `remind_at`, `recurrence`
 * are required by the backend.
 */
data class ReminderFormState(
    val contactId: Int = 0,
    val reminderId: Int? = null,
    val message: String = "",
    val remindAt: String = "",
    val recurrence: String = ReminderRecurrence.ONCE,
    val byMail: Boolean = false,
    val isLoading: Boolean = false,
    val isSaving: Boolean = false,
    @StringRes val errorRes: Int? = null,
    val error: String? = null,
) {
    val isEdit: Boolean get() = reminderId != null
    val hasMessage: Boolean get() = message.isNotBlank()

    fun toReminder(): Reminder = Reminder(
        id = reminderId ?: 0,
        message = message.trim(),
        remindAt = remindAt.trim().ifBlank { null },
        recurrence = recurrence,
        byMail = byMail,
        contactId = contactId.takeIf { it != 0 },
    )

    fun validate(): Int? = when {
        !hasMessage -> R.string.reminder_error_message
        remindAt.isBlank() -> R.string.reminder_error_remind_at_required
        remindAt.isNotBlank() && !remindAt.matches(ISO_DATETIME_REGEX) -> R.string.reminder_error_remind_at
        recurrence !in ReminderRecurrence.ALL -> R.string.reminder_error_recurrence
        else -> null
    }

    companion object {
        val ISO_DATETIME_REGEX = Regex(
            """\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})""",
        )
    }
}

sealed interface ReminderFormEvent {
    data object Saved : ReminderFormEvent
}

@HiltViewModel
class ReminderFormViewModel @Inject constructor(
    private val reminderRepository: ReminderRepository,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {

    private val contactId: Int = run {
        val raw: Any? = savedStateHandle["contactId"]
        (raw as? Int) ?: (raw as? String)?.toIntOrNull() ?: 0
    }

    private val reminderId: Int? = run {
        val raw: Any? = savedStateHandle["reminderId"]
        (raw as? Int) ?: (raw as? String)?.toIntOrNull()
    }

    private val _uiState = MutableStateFlow(ReminderFormState(contactId = contactId, reminderId = reminderId))
    val uiState: StateFlow<ReminderFormState> = _uiState.asStateFlow()

    private val _events = MutableStateFlow<ReminderFormEvent?>(null)
    val events: StateFlow<ReminderFormEvent?> = _events

    init {
        if (reminderId != null) loadExisting()
    }

    fun loadExisting() {
        val id = reminderId ?: return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, errorRes = null, error = null) }
            reminderRepository.get(id).foldApiError(
                onSuccess = { reminder ->
                    _uiState.update { it.toFormState(reminder).copy(isLoading = false) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun onMessageChange(value: String) = _uiState.update { it.copy(message = value) }
    fun onRemindAtChange(value: String) = _uiState.update { it.copy(remindAt = value) }
    fun onRecurrenceChange(value: String) = _uiState.update { it.copy(recurrence = value) }
    fun onByMailChange(value: Boolean) = _uiState.update { it.copy(byMail = value) }
    fun onErrorShown() = _uiState.update { it.copy(errorRes = null, error = null) }

    fun save() {
        val state = _uiState.value
        if (state.isSaving) return

        val problem = state.validate()
        if (problem != null) {
            _uiState.update { it.copy(errorRes = problem, error = null) }
            return
        }

        val reminder = state.toReminder()
        _uiState.update { it.copy(isSaving = true, errorRes = null, error = null) }
        viewModelScope.launch {
            val result = if (state.reminderId != null) {
                reminderRepository.update(state.reminderId, reminder)
            } else {
                reminderRepository.create(state.contactId, reminder)
            }
            result.foldApiError(
                onSuccess = { _uiState.update { it.copy(isSaving = false) }; _events.value = ReminderFormEvent.Saved },
                onError = { error ->
                    _uiState.update { it.copy(isSaving = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun onSaveShown() {
        _events.value = null
    }

    private fun ReminderFormState.toFormState(reminder: Reminder): ReminderFormState = copy(
        message = reminder.message.orEmpty(),
        remindAt = reminder.remindAt.orEmpty(),
        recurrence = reminder.recurrence ?: ReminderRecurrence.ONCE,
        byMail = reminder.byMail ?: false,
    )
}
