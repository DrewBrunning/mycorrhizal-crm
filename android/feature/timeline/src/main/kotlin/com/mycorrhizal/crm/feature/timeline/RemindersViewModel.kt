package com.mycorrhizal.crm.feature.timeline

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ReminderRepository
import com.mycorrhizal.crm.model.network.Reminder
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class RemindersUiState(
    val contactId: Int = 0,
    val reminders: List<Reminder> = emptyList(),
    val isLoading: Boolean = false,
    val completingId: Int? = null,
    val error: String? = null,
)

sealed interface RemindersEvent {
    data class ShowMessage(val message: String) : RemindersEvent
    data class ShowError(val message: String) : RemindersEvent
}

@HiltViewModel
class RemindersViewModel @Inject constructor(
    private val reminderRepository: ReminderRepository,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {

    private val contactId: Int = run {
        val raw: Any? = savedStateHandle["contactId"]
        (raw as? Int) ?: (raw as? String)?.toIntOrNull() ?: 0
    }

    private val _uiState = MutableStateFlow(RemindersUiState(contactId = contactId))
    val uiState: StateFlow<RemindersUiState> = _uiState.asStateFlow()

    init {
        load()
    }

    fun load() {
        if (contactId == 0) {
            _uiState.update { it.copy(isLoading = false, error = "Missing contact id") }
            return
        }
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            reminderRepository.listForContact(contactId).foldApiError(
                onSuccess = { reminders ->
                    _uiState.update { it.copy(isLoading = false, reminders = reminders) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun complete(id: Int) {
        if (_uiState.value.completingId != null) return
        viewModelScope.launch {
            _uiState.update { it.copy(completingId = id) }
            reminderRepository.complete(id).foldApiError(
                onSuccess = { reminder ->
                    if (reminder == null) {
                        // Once reminder: soft-deleted server-side, drop it from the list.
                        _uiState.update { state ->
                            state.copy(
                                completingId = null,
                                reminders = state.reminders.filterNot { it.id == id },
                            )
                        }
                    } else {
                        // Recurring reminder: rescheduled, replace with the new occurrence.
                        _uiState.update { state ->
                            state.copy(
                                completingId = null,
                                reminders = state.reminders.map { if (it.id == id) reminder else it },
                            )
                        }
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(completingId = null, error = error.displayMessage) }
                },
            )
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null) }
    }
}
