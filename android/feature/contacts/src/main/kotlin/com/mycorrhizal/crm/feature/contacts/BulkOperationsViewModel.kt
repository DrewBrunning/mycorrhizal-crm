package com.mycorrhizal.crm.feature.contacts

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.BulkOperationRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.model.network.BulkActions
import com.mycorrhizal.crm.model.network.BulkContactOperationInput
import com.mycorrhizal.crm.model.network.BulkOperationResult
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class BulkUiState(
    val contacts: List<ContactSummary> = emptyList(),
    val selected: Set<Int> = emptySet(),
    val isLoading: Boolean = false,
    val runningAction: Boolean = false,
    val result: BulkOperationResult? = null,
    val error: String? = null,
)

@HiltViewModel
class BulkOperationsViewModel @Inject constructor(
    private val bulkOperationRepository: BulkOperationRepository,
    private val contactRepository: ContactRepository,
) : ViewModel() {

    private val _uiState = MutableStateFlow(BulkUiState())
    val uiState: StateFlow<BulkUiState> = _uiState.asStateFlow()

    init {
        load()
    }

    fun load() {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            contactRepository.listContacts().foldApiError(
                onSuccess = { page ->
                    _uiState.update { it.copy(isLoading = false, contacts = page.contacts) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun toggle(id: Int) {
        _uiState.update { state ->
            val next = state.selected.toMutableSet()
            if (!next.add(id)) next.remove(id)
            state.copy(selected = next, result = null)
        }
    }

    fun run(action: String, circleId: String? = null, tagId: String? = null) {
        val uids = selectedUids()
        if (uids.isEmpty() || _uiState.value.runningAction) return
        viewModelScope.launch {
            _uiState.update { it.copy(runningAction = true, error = null, result = null) }
            bulkOperationRepository.run(
                BulkContactOperationInput(
                    action = action,
                    vcardUids = uids,
                    circleId = circleId,
                    tagId = tagId,
                ),
            ).foldApiError(
                onSuccess = { result ->
                    _uiState.update { it.copy(runningAction = false, result = result, selected = emptySet()) }
                    load()
                },
                onError = { error ->
                    _uiState.update { it.copy(runningAction = false, error = error.displayMessage) }
                },
            )
        }
    }

    private fun selectedUids(): List<String> {
        val selectedIds = _uiState.value.selected
        return _uiState.value.contacts.filter { it.id in selectedIds }.mapNotNull { it.uid }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null) }
    }
}
