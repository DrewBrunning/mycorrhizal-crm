package com.mycorrhizal.crm.feature.imports

import androidx.annotation.StringRes
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.BundleBackupRepository
import com.mycorrhizal.crm.model.network.ImportConfirmRequest
import com.mycorrhizal.crm.model.network.MonicaConnectRequest
import com.mycorrhizal.crm.model.network.MonicaConnectResponse
import com.mycorrhizal.crm.model.network.RowImportAction
import com.mycorrhizal.crm.model.network.SourceImportPreviewResponse
import com.mycorrhizal.crm.model.network.SourceImportResult
import com.mycorrhizal.crm.model.network.SourceImportStatus
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.toApiError
import com.mycorrhizal.crm.ui.R
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

enum class MonicaImportStep { CONNECT, WORKING, REVIEW, RESULT }

data class MonicaImportUiState(
    val step: MonicaImportStep = MonicaImportStep.CONNECT,
    val baseUrl: String = "",
    val apiToken: String = "",
    /** Both fetch options default on, as on web. */
    val includeRelationships: Boolean = true,
    val includeExtras: Boolean = true,
    val isConnecting: Boolean = false,
    /** Set once the server accepted the address + token and opened a session. */
    val connected: MonicaConnectResponse? = null,
    /** A Local (this-device-only) profile has no remote-source import. */
    val unavailable: Boolean = false,
    val preview: SourceImportPreviewResponse? = null,
    /** row_index -> "skip" | "add" | "update" (merge); rows with validation errors are forced to "skip". */
    val rowActions: Map<Int, String> = emptyMap(),
    val result: SourceImportResult? = null,
    val isImporting: Boolean = false,
    @StringRes val errorRes: Int? = null,
    val error: ApiError? = null,
    /** Server-reported job failure text (not an error envelope). */
    val errorText: String? = null
) {
    val canConnect: Boolean
        get() = !isConnecting && connected == null && baseUrl.isNotBlank() && apiToken.isNotEmpty()
    val canStart: Boolean get() = connected != null && !isImporting
}

/**
 * Issues #549/#2525: "Import from Monica" over its live API. Drives the `monica`
 * import source: connect (address + API token; the server probes the instance) ->
 * fetch (background read + preview build, polled) -> preview -> confirm (background
 * import, polled), reusing the shared source-import review UI ([ImportReviewStep]).
 *
 * The token is third-party: it is sent once and cleared from state as soon as the
 * server holds it; it is never persisted or logged client-side.
 */
@HiltViewModel
class MonicaImportViewModel @Inject constructor(
    private val apiClient: ApiClient,
    bundleBackupRepository: BundleBackupRepository
) : ViewModel() {

    private val _uiState = MutableStateFlow(MonicaImportUiState())
    val uiState: StateFlow<MonicaImportUiState> = _uiState.asStateFlow()

    /** Poll cadence; overridable so tests need not wait in real time. */
    internal var pollIntervalMillis: Long = POLL_INTERVAL_MILLIS

    /**
     * Poll budget per wait. Monica's fetch is rate limited at about one request per
     * second, so the budget is far longer than Meerkat's (an hour at the default
     * cadence); overridable so tests need not run it.
     */
    internal var maxPolls: Int = MAX_POLLS

    /**
     * True once the server accepted a confirm. From then on leaving the screen must
     * not cancel: while `importing` a cancel rolls the confirmed import back, and
     * during `importing_photos` it drops the session mid download.
     */
    private var importConfirmed = false

    /**
     * True while the confirm request is on the wire. The server may already have
     * started the import by the time the user backs out, so [cancel] must not roll
     * it back in that window ([importConfirmed] is only set once the call returns).
     */
    private var confirmInFlight = false

    init {
        viewModelScope.launch {
            bundleBackupRepository.observeStatus().collect { status ->
                _uiState.update { it.copy(unavailable = status.isLocalProfile) }
            }
        }
    }

    fun onBaseUrlChange(value: String) = editForm { it.copy(baseUrl = value) }

    fun onApiTokenChange(value: String) = editForm { it.copy(apiToken = value) }

    fun onIncludeRelationshipsChange(value: Boolean) =
        editForm { it.copy(includeRelationships = value) }

    fun onIncludeExtrasChange(value: Boolean) = editForm { it.copy(includeExtras = value) }

    /** The form is locked once a session exists, so edits after connecting are ignored. */
    private fun editForm(change: (MonicaImportUiState) -> MonicaImportUiState) =
        _uiState.update { if (it.connected == null) change(it) else it }

    fun connect() {
        val state = _uiState.value
        if (!state.canConnect) return
        val request = MonicaConnectRequest(
            baseUrl = state.baseUrl.trim(),
            apiToken = state.apiToken
        )
        _uiState.update {
            it.copy(isConnecting = true, errorRes = null, error = null, errorText = null)
        }
        viewModelScope.launch {
            // NonCancellable: leaving the screen must not abort the request mid-flight
            // and leave a session (holding the Monica token) nobody knows about.
            val result = withContext(NonCancellable) { apiClient.connectMonica(request) }
            if (!isActive) {
                // The screen was left while connecting; nobody will use this session.
                result.getOrNull()?.let { cancelSession(it.sessionId) }
                return@launch
            }
            result.fold(
                onSuccess = { connected ->
                    _uiState.update {
                        // The server holds the token now; drop it from the form state.
                        it.copy(isConnecting = false, connected = connected, apiToken = "")
                    }
                },
                onFailure = { error ->
                    _uiState.update { it.copy(isConnecting = false, error = error.toApiError()) }
                }
            )
        }
    }

    fun startImport() {
        val state = _uiState.value
        val connected = state.connected ?: return
        if (!state.canStart) return
        // Flip to WORKING synchronously so a second tap cannot launch a second fetch.
        _uiState.update {
            it.copy(
                step = MonicaImportStep.WORKING,
                isImporting = true,
                errorRes = null,
                error = null,
                errorText = null
            )
        }
        viewModelScope.launch {
            val sessionId = connected.sessionId
            apiClient.startMonicaFetch(
                sessionId,
                state.includeRelationships,
                state.includeExtras
            ).exceptionOrNull()?.let { return@launch failToConnect(it.toApiError()) }
            val status = pollUntil(sessionId, terminal = setOf(PHASE_READY)).terminal
            if (status == null || status.phase != PHASE_READY) {
                return@launch failToConnect(text = status?.error)
            }
            apiClient.getMonicaImportPreview(sessionId).fold(
                onSuccess = { preview ->
                    val actions = preview.rows.associate { row ->
                        row.rowIndex to
                            if (row.validationErrors.isNotEmpty()) "skip" else row.suggestedAction
                    }
                    _uiState.update {
                        it.copy(
                            step = MonicaImportStep.REVIEW,
                            isImporting = false,
                            preview = preview,
                            rowActions = actions
                        )
                    }
                },
                onFailure = { error -> failToConnect(error.toApiError()) }
            )
        }
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
        if (state.step != MonicaImportStep.REVIEW || state.isImporting) return
        _uiState.update {
            it.copy(
                step = MonicaImportStep.WORKING,
                isImporting = true,
                error = null,
                errorText = null
            )
        }
        viewModelScope.launch {
            val actions = _uiState.value.rowActions.map { (index, action) ->
                RowImportAction(index, action)
            }
            confirmInFlight = true
            val confirmed = try {
                apiClient.confirmMonicaImport(ImportConfirmRequest(preview.sessionId, actions))
            } finally {
                confirmInFlight = false
            }
            confirmed.exceptionOrNull()?.let { return@launch failToReview(it.toApiError()) }
            importConfirmed = true
            val polled = pollUntil(preview.sessionId, terminal = setOf(PHASE_DONE))
            val status = polled.terminal
            // The import commits before the profile-photo downloads finish, so a poll
            // that ran out (or failed) after `status.result` appeared still imported.
            val committed = polled.last?.result
            val finished = status?.takeIf { it.phase == PHASE_DONE }?.result
            if (finished != null) {
                showResult(finished)
            } else if (status == null && committed != null) {
                showResult(committed)
            } else {
                // A reported failure means the import is no longer running; with no
                // status (poll failed or timed out) it may still be, so stay confirmed.
                if (status != null) importConfirmed = false
                failToReview(text = status?.error)
            }
        }
    }

    private fun showResult(result: SourceImportResult) {
        _uiState.update {
            it.copy(step = MonicaImportStep.RESULT, isImporting = false, result = result)
        }
    }

    /**
     * Drop the server session (and its stored token). Called from [onCleared], so it
     * runs however the screen is left: the app-bar back icon, the system back button
     * or gesture, or any other pop of the destination. A no-op once the import has
     * been confirmed ([importConfirmed]) or finished.
     */
    fun cancel() {
        val state = _uiState.value
        val sessionId = state.connected?.sessionId ?: return
        if (state.step == MonicaImportStep.RESULT || importConfirmed || confirmInFlight) return
        cancelSession(sessionId)
    }

    /**
     * Fire-and-forget cancel that outlives this ViewModel: by the time [onCleared]
     * runs `viewModelScope` is already cancelled, which would abort the in-flight
     * HTTP call, so [NonCancellable] keeps it alive.
     */
    private fun cancelSession(sessionId: String) {
        viewModelScope.launch(NonCancellable) { apiClient.cancelMonicaImport(sessionId) }
    }

    override fun onCleared() {
        cancel()
        super.onCleared()
    }

    fun onErrorShown() {
        _uiState.update { it.copy(errorRes = null, error = null, errorText = null) }
    }

    /** [terminal] is null when a poll request failed or [maxPolls] ran out; [last] is the last status seen. */
    private class Polled(val terminal: SourceImportStatus?, val last: SourceImportStatus?)

    /**
     * Polls until the phase is in [terminal] (or `failed`/`cancelled`). The result
     * carries that status, or null when a poll request itself fails or [maxPolls]
     * runs out, plus the last status seen either way.
     */
    private suspend fun pollUntil(sessionId: String, terminal: Set<String>): Polled {
        var last: SourceImportStatus? = null
        repeat(maxPolls) {
            val status = apiClient.getMonicaImportStatus(sessionId).getOrNull()
                ?: return Polled(null, last)
            last = status
            if (status.phase in terminal || status.phase == PHASE_FAILED ||
                status.phase == PHASE_CANCELLED
            ) {
                return Polled(status, status)
            }
            delay(pollIntervalMillis)
        }
        return Polled(null, last)
    }

    /**
     * Back to the connect form. The form was locked and the token already cleared, so
     * the user connects again and gets a new session; the abandoned one is cancelled
     * first because it still holds the Monica token until it expires.
     */
    private fun failToConnect(error: ApiError? = null, text: String? = null) {
        val abandoned = _uiState.value.connected?.sessionId
        _uiState.update {
            it.copy(
                step = MonicaImportStep.CONNECT,
                isImporting = false,
                connected = null,
                preview = null,
                errorRes = genericFailureRes(error, text),
                error = error,
                errorText = text?.takeIf { m -> m.isNotBlank() }
            )
        }
        abandoned?.let(::cancelSession)
    }

    private fun failToReview(error: ApiError? = null, text: String? = null) {
        _uiState.update {
            it.copy(
                step = MonicaImportStep.REVIEW,
                isImporting = false,
                errorRes = genericFailureRes(error, text),
                error = error,
                errorText = text?.takeIf { m -> m.isNotBlank() }
            )
        }
    }

    @StringRes
    private fun genericFailureRes(error: ApiError?, text: String?): Int? =
        if (error == null && text.isNullOrBlank()) R.string.monica_import_error_failed else null

    companion object {
        const val POLL_INTERVAL_MILLIS = 1000L
        const val MAX_POLLS = 3600

        // Phase tokens: hardcoded mirror of backend models.SourceImportPhase* — keep in sync.
        const val PHASE_READY = "ready"
        const val PHASE_DONE = "done"
        const val PHASE_FAILED = "failed"
        const val PHASE_CANCELLED = "cancelled"
    }
}
