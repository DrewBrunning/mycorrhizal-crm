package com.mycorrhizal.crm.feature.imports

import androidx.annotation.StringRes
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.model.network.ImportConfirmRequest
import com.mycorrhizal.crm.model.network.ImportPreviewResponse
import com.mycorrhizal.crm.model.network.ImportResult
import com.mycorrhizal.crm.model.network.RowImportAction
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.foldApiError
import com.mycorrhizal.crm.ui.R
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

enum class VcfImportStep { PICK, PREVIEW, RESULT }

data class VcfImportUiState(
    val step: VcfImportStep = VcfImportStep.PICK,
    val fileName: String? = null,
    val preview: ImportPreviewResponse? = null,
    /** row_index -> "skip" | "add" | "update", editable per row; rows with validation errors are
     *  forced to "skip" at load time (mirrors web's "Errors are disabled in the UI and always
     *  stay skip" — `ImportContactsDialog.tsx`'s `handleAcceptAll`/`handleSkipAll`). */
    val rowActions: Map<Int, String> = emptyMap(),
    val result: ImportResult? = null,
    val isLoading: Boolean = false,
    @StringRes val errorRes: Int? = null,
    val error: String? = null,
)

/**
 * M9 item 4: VCF-file import. `uploadVcfImport`/`ImportPreviewResponse`/`ImportConfirmRequest`/
 * `ImportResult`/`RowImportAction` already existed with zero UI callers; this ViewModel (and the
 * new `confirmVcfImport` client method) is their first real caller. No repository layer exists
 * for import (same precedent as `ContactListViewModel` injecting `ApiClient` directly for
 * `/search` — a composite flow with no single-entity home).
 */
@HiltViewModel
class VcfImportViewModel @Inject constructor(
    private val apiClient: ApiClient,
) : ViewModel() {

    private val _uiState = MutableStateFlow(VcfImportUiState())
    val uiState: StateFlow<VcfImportUiState> = _uiState.asStateFlow()

    /** Client-side gate matching `backend/services/import_service.go`'s `MaxVCFSize` (VCF files
     *  can embed photos, hence the higher limit than CSV) — avoids an upload doomed to fail. */
    fun onFilePicked(fileName: String, bytes: ByteArray) {
        if (_uiState.value.isLoading) return
        if (bytes.isEmpty()) {
            _uiState.update { it.copy(errorRes = R.string.import_vcf_error_invalid_file, error = null) }
            return
        }
        if (bytes.size > MAX_VCF_SIZE_BYTES) {
            _uiState.update { it.copy(errorRes = R.string.import_vcf_error_too_large, error = null) }
            return
        }
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, errorRes = null, error = null, fileName = fileName) }
            apiClient.uploadVcfImport(bytes, fileName).foldApiError(
                onSuccess = { preview ->
                    val actions = preview.rows.associate { row ->
                        row.rowIndex to if (row.validationErrors.isNotEmpty()) "skip" else row.suggestedAction
                    }
                    _uiState.update {
                        it.copy(isLoading = false, step = VcfImportStep.PREVIEW, preview = preview, rowActions = actions)
                    }
                },
                onError = { error -> _uiState.update { it.copy(isLoading = false, error = error.displayMessage) } },
            )
        }
    }

    /** No-op for a row with validation errors — it stays forced to "skip". */
    fun setRowAction(rowIndex: Int, action: String) {
        val row = _uiState.value.preview?.rows?.find { it.rowIndex == rowIndex } ?: return
        if (row.validationErrors.isNotEmpty()) return
        _uiState.update { it.copy(rowActions = it.rowActions + (rowIndex to action)) }
    }

    fun confirm() {
        val preview = _uiState.value.preview ?: return
        if (_uiState.value.isLoading) return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            val actions = _uiState.value.rowActions.map { (rowIndex, action) -> RowImportAction(rowIndex, action) }
            apiClient.confirmVcfImport(ImportConfirmRequest(sessionId = preview.sessionId, actions = actions)).foldApiError(
                onSuccess = { result ->
                    _uiState.update { it.copy(isLoading = false, step = VcfImportStep.RESULT, result = result) }
                },
                onError = { error -> _uiState.update { it.copy(isLoading = false, error = error.displayMessage) } },
            )
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(errorRes = null, error = null) }
    }

    companion object {
        const val MAX_VCF_SIZE_BYTES = 50 * 1024 * 1024
    }
}
