package com.mycorrhizal.crm.feature.imports

import androidx.annotation.StringRes
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.model.network.ImportConfirmRequest
import com.mycorrhizal.crm.model.network.MycorrhizalBundleCounts
import com.mycorrhizal.crm.model.network.RowImportAction
import com.mycorrhizal.crm.model.network.SourceImportPreviewResponse
import com.mycorrhizal.crm.model.network.SourceImportResult
import com.mycorrhizal.crm.model.network.SourceImportStatus
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.toApiError
import com.mycorrhizal.crm.ui.R
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

enum class BundleRestoreStep { PICK, WORKING, REVIEW, RESULT }

data class BundleRestoreUiState(
    val step: BundleRestoreStep = BundleRestoreStep.PICK,
    val fileName: String? = null,
    /** Per-section tally shown after the upload validated the bundle. */
    val totals: MycorrhizalBundleCounts? = null,
    val preview: SourceImportPreviewResponse? = null,
    /** row_index -> "skip" | "add" | "update"; rows with validation errors are forced to "skip". */
    val rowActions: Map<Int, String> = emptyMap(),
    val result: SourceImportResult? = null,
    /** True while a server phase is running (mapping the bundle, or importing it). */
    val isImporting: Boolean = false,
    @StringRes val errorRes: Int? = null,
    val error: String? = null,
)

/**
 * Issue #1264 / ADR 0028 Decision 3: "Restore from bundle". Drives the
 * `mycorrhizal` import source — upload → fetch (background map + preview build,
 * polled) → preview → confirm (background import, polled) — against the active
 * profile, reusing the shared source-import review UI ([ImportReviewStep]).
 *
 * The bundle is the user's own full-fidelity export, so on the fresh profile
 * this is offered for, every row's suggested action is "add"; the review step
 * still lets the user override per row exactly like the VCF flow.
 */
@HiltViewModel
class BundleRestoreViewModel @Inject constructor(
    private val apiClient: ApiClient,
) : ViewModel() {

    private val _uiState = MutableStateFlow(BundleRestoreUiState())
    val uiState: StateFlow<BundleRestoreUiState> = _uiState.asStateFlow()

    /** Poll cadence; overridable so tests need not wait in real time. */
    internal var pollIntervalMillis: Long = POLL_INTERVAL_MILLIS

    fun onFileTooLarge() {
        _uiState.update { it.copy(errorRes = R.string.bundle_restore_error_too_large, error = null) }
    }

    fun onFilePicked(fileName: String, bytes: ByteArray) {
        if (_uiState.value.step == BundleRestoreStep.WORKING) return
        if (bytes.isEmpty()) {
            _uiState.update { it.copy(errorRes = R.string.bundle_restore_error_invalid_file, error = null) }
            return
        }
        if (bytes.size > MAX_BUNDLE_SIZE_BYTES) {
            onFileTooLarge()
            return
        }
        // Flip to WORKING synchronously, before launching: the re-entrancy guard above reads it,
        // so a second pick arriving before the coroutine first runs must already see it.
        _uiState.update {
            it.copy(
                step = BundleRestoreStep.WORKING,
                isImporting = true,
                fileName = fileName,
                errorRes = null,
                error = null,
            )
        }
        viewModelScope.launch {
            apiClient.uploadMycorrhizalBundle(bytes, fileName).fold(
                onSuccess = { upload ->
                    _uiState.update { it.copy(totals = upload.totals) }
                    prepare(upload.sessionId)
                },
                onFailure = { error -> failToPick(error.toApiError().displayMessage) },
            )
        }
    }

    private suspend fun prepare(sessionId: String) {
        val started = apiClient.startMycorrhizalFetch(sessionId)
        started.exceptionOrNull()?.let { return failToPick(it.toApiError().displayMessage) }
        val status = pollUntil(sessionId, terminal = PREPARED_PHASES)
        if (status == null || status.phase != PHASE_READY) {
            return failToPick(status?.error)
        }
        apiClient.getMycorrhizalImportPreview(sessionId).fold(
            onSuccess = { preview ->
                val actions = preview.rows.associate { row ->
                    row.rowIndex to if (row.validationErrors.isNotEmpty()) "skip" else row.suggestedAction
                }
                _uiState.update {
                    it.copy(
                        step = BundleRestoreStep.REVIEW,
                        isImporting = false,
                        preview = preview,
                        rowActions = actions,
                    )
                }
            },
            onFailure = { error -> failToPick(error.toApiError().displayMessage) },
        )
    }

    /** No-op for a row with validation errors — it stays forced to "skip". */
    fun setRowAction(rowIndex: Int, action: String) {
        val row = _uiState.value.preview?.rows?.find { it.rowIndex == rowIndex } ?: return
        if (row.validationErrors.isNotEmpty()) return
        _uiState.update { it.copy(rowActions = it.rowActions + (rowIndex to action)) }
    }

    /** Every valid row takes its suggested action; errored rows stay skipped. */
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
        val state = _uiState.value
        val preview = state.preview ?: return
        if (state.step != BundleRestoreStep.REVIEW || state.isImporting) return
        _uiState.update { it.copy(step = BundleRestoreStep.WORKING, isImporting = true, error = null) }
        viewModelScope.launch {
            val actions = _uiState.value.rowActions.map { (index, action) -> RowImportAction(index, action) }
            val started = apiClient.confirmMycorrhizalImport(ImportConfirmRequest(preview.sessionId, actions))
            started.exceptionOrNull()?.let { return@launch failToReview(it.toApiError().displayMessage) }
            val status = pollUntil(preview.sessionId, terminal = FINISHED_PHASES)
            if (status?.phase == PHASE_DONE && status.result != null) {
                _uiState.update {
                    it.copy(step = BundleRestoreStep.RESULT, isImporting = false, result = status.result)
                }
            } else {
                failToReview(status?.error)
            }
        }
    }

    /** Drop the server session when the user leaves mid-flow (best effort). */
    fun cancel() {
        val sessionId = _uiState.value.preview?.sessionId ?: return
        if (_uiState.value.step == BundleRestoreStep.RESULT) return
        viewModelScope.launch { apiClient.cancelMycorrhizalImport(sessionId) }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(errorRes = null, error = null) }
    }

    /**
     * Polls the session until its phase is in [terminal] (or `failed`/`cancelled`),
     * returning that status; null when a poll request itself fails or the
     * [MAX_POLLS] budget runs out (the caller reports a generic failure).
     */
    private suspend fun pollUntil(sessionId: String, terminal: Set<String>): SourceImportStatus? {
        repeat(MAX_POLLS) {
            val status = apiClient.getMycorrhizalImportStatus(sessionId).getOrNull() ?: return null
            if (status.phase in terminal || status.phase == PHASE_FAILED || status.phase == PHASE_CANCELLED) {
                return status
            }
            delay(pollIntervalMillis)
        }
        return null
    }

    private fun failToPick(message: String?) {
        _uiState.update {
            it.copy(
                step = BundleRestoreStep.PICK,
                isImporting = false,
                preview = null,
                totals = null,
                errorRes = if (message.isNullOrBlank()) R.string.bundle_restore_error_failed else null,
                error = message?.takeIf { m -> m.isNotBlank() },
            )
        }
    }

    private fun failToReview(message: String?) {
        _uiState.update {
            it.copy(
                step = BundleRestoreStep.REVIEW,
                isImporting = false,
                errorRes = if (message.isNullOrBlank()) R.string.bundle_restore_error_failed else null,
                error = message?.takeIf { m -> m.isNotBlank() },
            )
        }
    }

    companion object {
        /** Matches the backend's `services.MaxMycorrhizalBundleSize` (64 MiB) — avoids an upload doomed to 400. */
        const val MAX_BUNDLE_SIZE_BYTES = 64 * 1024 * 1024
        const val POLL_INTERVAL_MILLIS = 500L
        const val MAX_POLLS = 600

        // Phase tokens: hardcoded mirror of backend models.SourceImportPhase* — keep in sync.
        const val PHASE_READY = "ready"
        const val PHASE_DONE = "done"
        const val PHASE_FAILED = "failed"
        const val PHASE_CANCELLED = "cancelled"
        private val PREPARED_PHASES = setOf(PHASE_READY)
        private val FINISHED_PHASES = setOf(PHASE_DONE)
    }
}
