package com.mycorrhizal.crm.feature.contacts

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.MergeRepository
import com.mycorrhizal.crm.model.network.ContactMergeFieldConflict
import com.mycorrhizal.crm.model.network.ContactMergePreviewResponse
import com.mycorrhizal.crm.model.network.ContactMergeRequest
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class MergeUiState(
    val keepId: Long = 0,
    val mergeId: Long = 0,
    val preview: ContactMergePreviewResponse? = null,
    val isLoading: Boolean = false,
    val isCommitting: Boolean = false,
    /** field -> chosen value; covers scalar + field-value conflicts. */
    val resolutions: Map<String, String> = emptyMap(),
    val merged: Boolean = false,
    val error: String? = null,
)

@HiltViewModel
class MergeContactsViewModel @Inject constructor(
    private val mergeRepository: MergeRepository,
) : ViewModel() {

    private val _uiState = MutableStateFlow(MergeUiState())
    val uiState: StateFlow<MergeUiState> = _uiState.asStateFlow()

    fun setPair(keepId: Long, mergeId: Long) {
        _uiState.update { it.copy(keepId = keepId, mergeId = mergeId) }
    }

    fun preview() {
        val keep = _uiState.value.keepId
        val merge = _uiState.value.mergeId
        if (keep == 0L || merge == 0L) return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            mergeRepository.preview(ContactMergeRequest(keepId = keep, mergeId = merge)).foldApiError(
                onSuccess = { preview ->
                    _uiState.update { it.copy(isLoading = false, preview = preview) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun resolve(field: String, value: String) {
        _uiState.update { it.copy(resolutions = it.resolutions + (field to value)) }
    }

    fun commit() {
        val keep = _uiState.value.keepId
        val merge = _uiState.value.mergeId
        if (keep == 0L || merge == 0L || _uiState.value.isCommitting) return
        viewModelScope.launch {
            _uiState.update { it.copy(isCommitting = true, error = null) }
            mergeRepository.commit(
                ContactMergeRequest(keepId = keep, mergeId = merge, resolutions = _uiState.value.resolutions),
            ).foldApiError(
                onSuccess = {
                    _uiState.update { it.copy(isCommitting = false, merged = true) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isCommitting = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null) }
    }

    fun allConflicts(preview: ContactMergePreviewResponse): List<ContactMergeFieldConflict> =
        preview.resolution.conflicts + preview.resolution.fieldValueConflicts
}
