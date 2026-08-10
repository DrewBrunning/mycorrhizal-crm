package com.mycorrhizal.crm.feature.contacts

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.model.network.CRMEnvelope
import com.mycorrhizal.crm.model.network.Card
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
    savedStateHandle: SavedStateHandle,
) : ViewModel() {

    private val contactId: Int = savedStateHandle["contactId"] ?: 0

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

    fun onErrorShown() {
        _uiState.update { it.copy(error = null) }
    }
}

// Convenience accessors used by the detail screen.
val ContactRecordResponse.cardOrNull: Card? get() = card
val ContactRecordResponse.crmOrNull: CRMEnvelope? get() = crm
