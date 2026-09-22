package com.mycorrhizal.crm.feature.imports

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.model.network.ImportRun
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class ImportHistoryUiState(
    val runs: List<ImportRun> = emptyList(),
    val isLoading: Boolean = false,
    val error: String? = null,
)

/**
 * Issue #834 (web parity, issue #651): the persisted import-run history —
 * `GET /contacts/import/history` already served web's Data-settings table
 * with zero Android caller. No repository layer exists for import (same
 * precedent as [VcfImportViewModel] calling [ApiClient] directly).
 */
@HiltViewModel
class ImportHistoryViewModel @Inject constructor(
    private val apiClient: ApiClient,
) : ViewModel() {

    private val _uiState = MutableStateFlow(ImportHistoryUiState())
    val uiState: StateFlow<ImportHistoryUiState> = _uiState.asStateFlow()

    init {
        load()
    }

    fun load() {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            apiClient.getImportHistory().foldApiError(
                onSuccess = { runs -> _uiState.update { it.copy(isLoading = false, runs = runs) } },
                onError = { error -> _uiState.update { it.copy(isLoading = false, error = error.displayMessage) } },
            )
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null) }
    }
}
