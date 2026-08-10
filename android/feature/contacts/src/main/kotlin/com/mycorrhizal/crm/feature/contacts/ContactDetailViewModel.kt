package com.mycorrhizal.crm.feature.contacts

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.ReminderRepository
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class ContactDetailUiState(
    val contact: ContactRecordResponse? = null,
    val isLoading: Boolean = false,
    val error: String? = null,
)

@HiltViewModel
class ContactDetailViewModel @Inject constructor(
    private val contactRepository: ContactRepository,
    private val reminderRepository: ReminderRepository,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {

    private val contactId: Int = run {
        val raw: Any? = savedStateHandle["contactId"]
        (raw as? Int) ?: (raw as? String)?.toIntOrNull() ?: 0
    }

    private val _uiState = MutableStateFlow(ContactDetailUiState())
    val uiState: StateFlow<ContactDetailUiState> = _uiState.asStateFlow()

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
            contactRepository.getContact(contactId).foldApiError(
                onSuccess = { contact ->
                    _uiState.update { it.copy(isLoading = false, contact = contact) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    /** Complete a reminder from the timeline; reload the contact so the list refreshes. */
    fun completeReminder(id: Int) {
        viewModelScope.launch {
            reminderRepository.complete(id).foldApiError(
                onSuccess = { load() },
                onError = { error ->
                    _uiState.update { it.copy(error = error.displayMessage) }
                },
            )
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null) }
    }
}
