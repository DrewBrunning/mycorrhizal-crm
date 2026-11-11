package com.mycorrhizal.crm.feature.imports

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.model.network.ImportRun
import com.mycorrhizal.crm.model.network.ImportRunDetail
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class ImportHistoryUiState(
    val runs: List<ImportRun> = emptyList(),
    val isLoading: Boolean = false,
    val error: ApiError? = null,
    /** Issue #2464: the itemized report of the run the user opened, once loaded. */
    val report: ImportRunDetail? = null,
    val isReportLoading: Boolean = false
) {
    /** The report view is showing (loading or loaded) instead of the run list. */
    val reportOpen: Boolean get() = isReportLoading || report != null
}

/**
 * Issue #834 (web parity, issue #651): the persisted import-run history —
 * `GET /contacts/import/history` already served web's Data-settings table
 * with zero Android caller. No repository layer exists for import (same
 * precedent as [VcfImportViewModel] calling [ApiClient] directly).
 */
@HiltViewModel
class ImportHistoryViewModel @Inject constructor(private val apiClient: ApiClient) : ViewModel() {

    private val _uiState = MutableStateFlow(ImportHistoryUiState())
    val uiState: StateFlow<ImportHistoryUiState> = _uiState.asStateFlow()

    private var reportJob: Job? = null

    init {
        load()
    }

    fun load() {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            apiClient.getImportHistory().foldApiError(
                onSuccess = { runs -> _uiState.update { it.copy(isLoading = false, runs = runs) } },
                onError = { error ->
                    _uiState.update {
                        it.copy(
                            isLoading = false,
                            error = error
                        )
                    }
                }
            )
        }
    }

    /** Issue #2464: load the stored per-row issues of one run (a 404/network error surfaces as [error]). */
    fun openReport(id: Long) {
        reportJob?.cancel()
        reportJob = viewModelScope.launch {
            _uiState.update { it.copy(isReportLoading = true, report = null, error = null) }
            apiClient.getImportRun(id).foldApiError(
                onSuccess = { run ->
                    _uiState.update { it.copy(isReportLoading = false, report = run) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isReportLoading = false, error = error) }
                }
            )
        }
    }

    fun closeReport() {
        reportJob?.cancel()
        _uiState.update { it.copy(report = null, isReportLoading = false) }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null) }
    }
}
