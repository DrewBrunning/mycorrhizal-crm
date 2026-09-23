package com.mycorrhizal.crm.feature.imports

import androidx.annotation.StringRes
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.model.network.ColumnMapping
import com.mycorrhizal.crm.model.network.ImportConfirmRequest
import com.mycorrhizal.crm.model.network.ImportPreviewRequest
import com.mycorrhizal.crm.model.network.ImportPreviewResponse
import com.mycorrhizal.crm.model.network.ImportResult
import com.mycorrhizal.crm.model.network.ImportUploadResponse
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

enum class CsvImportStep { PICK, MAPPING, PREVIEW, RESULT }

data class CsvImportUiState(
    val step: CsvImportStep = CsvImportStep.PICK,
    val fileName: String? = null,
    val upload: ImportUploadResponse? = null,
    /** csv_column -> contact_field ("" means "Ignore this column"), seeded from [ImportUploadResponse.suggestedMappings]. */
    val columnFields: Map<String, String> = emptyMap(),
    val preview: ImportPreviewResponse? = null,
    /** row_index -> "skip" | "add" | "update", same contract as VcfImportUiState.rowActions. */
    val rowActions: Map<Int, String> = emptyMap(),
    val result: ImportResult? = null,
    val isLoading: Boolean = false,
    @StringRes val errorRes: Int? = null,
    val error: String? = null,
)

/**
 * Issue #834: CSV-file import — pick a `.csv` file, map its columns to
 * contact fields (backend-suggested defaults, editable), review the
 * resulting preview rows (mirrors [VcfImportViewModel]'s review/confirm
 * halves via the shared [ImportReviewStep]), confirm. The one step VCF
 * doesn't need: CSV upload returns column headers + suggested mappings
 * rather than a preview directly, so a mapping round-trip
 * (`previewCsvImport`) sits between upload and review — mirrors the
 * CSV branch of web's `ImportContactsDialog.tsx`.
 */
@HiltViewModel
class CsvImportViewModel @Inject constructor(
    private val apiClient: ApiClient,
) : ViewModel() {

    private val _uiState = MutableStateFlow(CsvImportUiState())
    val uiState: StateFlow<CsvImportUiState> = _uiState.asStateFlow()

    /** Mirrors [VcfImportViewModel.onFileTooLarge] for the provider-declared-size probe. */
    fun onFileTooLarge() {
        _uiState.update { it.copy(errorRes = R.string.import_csv_error_too_large, error = null) }
    }

    /** Client-side gate matching `backend/services/import_service.go`'s `MaxCSVSize`. */
    fun onFilePicked(fileName: String, bytes: ByteArray) {
        if (_uiState.value.isLoading) return
        if (bytes.isEmpty()) {
            _uiState.update { it.copy(errorRes = R.string.import_csv_error_invalid_file, error = null) }
            return
        }
        if (bytes.size > MAX_CSV_SIZE_BYTES) {
            _uiState.update { it.copy(errorRes = R.string.import_csv_error_too_large, error = null) }
            return
        }
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, errorRes = null, error = null, fileName = fileName) }
            apiClient.uploadCsvImport(bytes, fileName).foldApiError(
                onSuccess = { upload ->
                    val fields = upload.suggestedMappings.associate { it.csvColumn to it.contactField }
                    _uiState.update {
                        it.copy(isLoading = false, step = CsvImportStep.MAPPING, upload = upload, columnFields = fields)
                    }
                },
                onError = { error -> _uiState.update { it.copy(isLoading = false, error = error.displayMessage) } },
            )
        }
    }

    fun setColumnField(csvColumn: String, contactField: String) {
        _uiState.update { it.copy(columnFields = it.columnFields + (csvColumn to contactField)) }
    }

    /** Submits the current column mapping and requests the preview. Columns mapped to "" (Ignore) are dropped. */
    fun submitMapping() {
        val upload = _uiState.value.upload ?: return
        if (_uiState.value.isLoading) return
        val mappings = upload.suggestedMappings.mapNotNull { suggested ->
            val field = _uiState.value.columnFields[suggested.csvColumn].orEmpty()
            if (field.isBlank()) null else ColumnMapping(csvColumn = suggested.csvColumn, contactField = field, group = suggested.group)
        }
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            apiClient.previewCsvImport(ImportPreviewRequest(sessionId = upload.sessionId, mappings = mappings)).foldApiError(
                onSuccess = { preview ->
                    val actions = preview.rows.associate { row ->
                        row.rowIndex to if (row.validationErrors.isNotEmpty()) "skip" else row.suggestedAction
                    }
                    _uiState.update {
                        it.copy(isLoading = false, step = CsvImportStep.PREVIEW, preview = preview, rowActions = actions)
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

    /** "Resolve all as merged": every valid row takes its suggested action. */
    fun resolveAll() {
        val preview = _uiState.value.preview ?: return
        _uiState.update { state ->
            val next = state.rowActions.toMutableMap()
            preview.rows.forEach { row ->
                if (row.validationErrors.isEmpty()) next[row.rowIndex] = row.suggestedAction
            }
            state.copy(rowActions = next)
        }
    }

    fun confirm() {
        val preview = _uiState.value.preview ?: return
        if (_uiState.value.isLoading) return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            val actions = _uiState.value.rowActions.map { (rowIndex, action) -> RowImportAction(rowIndex, action) }
            apiClient.confirmImport(ImportConfirmRequest(sessionId = preview.sessionId, actions = actions)).foldApiError(
                onSuccess = { result ->
                    _uiState.update { it.copy(isLoading = false, step = CsvImportStep.RESULT, result = result) }
                },
                onError = { error -> _uiState.update { it.copy(isLoading = false, error = error.displayMessage) } },
            )
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(errorRes = null, error = null) }
    }

    companion object {
        const val MAX_CSV_SIZE_BYTES = 20 * 1024 * 1024
    }
}
