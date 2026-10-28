package com.mycorrhizal.crm.feature.imports

import androidx.annotation.StringRes
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.BundleBackupRepository
import com.mycorrhizal.crm.model.network.ImportConfirmRequest
import com.mycorrhizal.crm.model.network.MeerkatConnectRequest
import com.mycorrhizal.crm.model.network.MeerkatConnectResponse
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
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

enum class MeerkatImportStep { CONNECT, WORKING, REVIEW, RESULT }

enum class MeerkatAuthMode { TOKEN, PASSWORD }

data class MeerkatImportUiState(
    val step: MeerkatImportStep = MeerkatImportStep.CONNECT,
    val baseUrl: String = "",
    val authMode: MeerkatAuthMode = MeerkatAuthMode.TOKEN,
    val apiToken: String = "",
    val username: String = "",
    val password: String = "",
    val isConnecting: Boolean = false,
    /** Set once the server accepted the address + credentials and opened a session. */
    val connected: MeerkatConnectResponse? = null,
    /** The explicit "try anyway" for a Meerkat version the adapter was not built against. */
    val acceptUntested: Boolean = false,
    /** A Local (this-device-only) profile has no remote-source import. */
    val unavailable: Boolean = false,
    val preview: SourceImportPreviewResponse? = null,
    /** row_index -> "skip" | "add" | "update"; rows with validation errors are forced to "skip". */
    val rowActions: Map<Int, String> = emptyMap(),
    val result: SourceImportResult? = null,
    val isImporting: Boolean = false,
    @StringRes val errorRes: Int? = null,
    val error: ApiError? = null,
    /** Server-reported job failure text (not an error envelope). */
    val errorText: String? = null
) {
    val hasCredential: Boolean
        get() = when (authMode) {
            MeerkatAuthMode.TOKEN -> apiToken.isNotEmpty()
            MeerkatAuthMode.PASSWORD -> username.isNotBlank() && password.isNotEmpty()
        }
    val canConnect: Boolean
        get() = !isConnecting && connected == null && baseUrl.isNotBlank() && hasCredential
    val canStart: Boolean
        get() = connected != null && !isImporting &&
            (connected.version.supported || acceptUntested)
}

/**
 * Issue #2013: "Import from Meerkat" over its live API. Drives the `meerkat`
 * import source — connect (address + token or username/password; the server
 * detects the version) → fetch (background read + preview build, polled) →
 * preview → confirm (background import, polled) — reusing the shared
 * source-import review UI ([ImportReviewStep]).
 *
 * The credential is third-party: it is sent once and cleared from state as soon
 * as the server holds it; it is never persisted client-side.
 */
@HiltViewModel
class MeerkatImportViewModel @Inject constructor(
    private val apiClient: ApiClient,
    bundleBackupRepository: BundleBackupRepository
) : ViewModel() {

    private val _uiState = MutableStateFlow(MeerkatImportUiState())
    val uiState: StateFlow<MeerkatImportUiState> = _uiState.asStateFlow()

    /** Poll cadence; overridable so tests need not wait in real time. */
    internal var pollIntervalMillis: Long = POLL_INTERVAL_MILLIS

    /** Poll budget per wait; overridable so tests need not run the real 10 minutes. */
    internal var maxPolls: Int = MAX_POLLS

    /**
     * True once the server accepted a confirm. From then on leaving the screen
     * must not cancel: while `importing` a cancel rolls the confirmed import
     * back, and during `importing_photos` it drops the session mid download.
     * Mirrors the web wizard, which never cancels an in-flight import on close.
     */
    private var importConfirmed = false

    init {
        viewModelScope.launch {
            bundleBackupRepository.observeStatus().collect { status ->
                _uiState.update { it.copy(unavailable = status.isLocalProfile) }
            }
        }
    }

    fun onBaseUrlChange(value: String) = editForm { it.copy(baseUrl = value) }

    fun onAuthModeChange(mode: MeerkatAuthMode) = editForm { it.copy(authMode = mode) }

    fun onApiTokenChange(value: String) = editForm { it.copy(apiToken = value) }

    fun onUsernameChange(value: String) = editForm { it.copy(username = value) }

    fun onPasswordChange(value: String) = editForm { it.copy(password = value) }

    fun onAcceptUntestedChange(value: Boolean) = _uiState.update { it.copy(acceptUntested = value) }

    /** The form is locked once a session exists, so edits after connecting are ignored. */
    private fun editForm(change: (MeerkatImportUiState) -> MeerkatImportUiState) =
        _uiState.update { if (it.connected == null) change(it) else it }

    fun connect() {
        val state = _uiState.value
        if (!state.canConnect) return
        val request = when (state.authMode) {
            MeerkatAuthMode.TOKEN ->
                MeerkatConnectRequest(baseUrl = state.baseUrl.trim(), apiToken = state.apiToken)

            MeerkatAuthMode.PASSWORD -> MeerkatConnectRequest(
                baseUrl = state.baseUrl.trim(),
                username = state.username.trim(),
                password = state.password
            )
        }
        _uiState.update {
            it.copy(
                isConnecting = true,
                errorRes = null,
                error = null,
                errorText = null
            )
        }
        viewModelScope.launch {
            apiClient.connectMeerkat(request).fold(
                onSuccess = { connected ->
                    _uiState.update {
                        // The server holds the credential now; drop it from the form state.
                        it.copy(
                            isConnecting = false,
                            connected = connected,
                            apiToken = "",
                            password = ""
                        )
                    }
                },
                onFailure = { error ->
                    _uiState.update {
                        it.copy(isConnecting = false, error = error.toApiError())
                    }
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
                step = MeerkatImportStep.WORKING,
                isImporting = true,
                errorRes = null,
                error = null,
                errorText = null
            )
        }
        viewModelScope.launch {
            val sessionId = connected.sessionId
            apiClient.startMeerkatFetch(sessionId, state.acceptUntested).exceptionOrNull()
                ?.let { return@launch failToConnect(it.toApiError()) }
            val status = pollUntil(sessionId, terminal = setOf(PHASE_READY)).terminal
            if (status == null || status.phase != PHASE_READY) {
                return@launch failToConnect(text = status?.error)
            }
            apiClient.getMeerkatImportPreview(sessionId).fold(
                onSuccess = { preview ->
                    val actions = preview.rows.associate { row ->
                        row.rowIndex to
                            if (row.validationErrors.isNotEmpty()) "skip" else row.suggestedAction
                    }
                    _uiState.update {
                        it.copy(
                            step = MeerkatImportStep.REVIEW,
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
        if (state.step != MeerkatImportStep.REVIEW || state.isImporting) return
        _uiState.update {
            it.copy(
                step = MeerkatImportStep.WORKING,
                isImporting = true,
                error = null,
                errorText = null
            )
        }
        viewModelScope.launch {
            val actions = _uiState.value.rowActions.map { (index, action) ->
                RowImportAction(index, action)
            }
            apiClient.confirmMeerkatImport(ImportConfirmRequest(preview.sessionId, actions))
                .exceptionOrNull()
                ?.let { return@launch failToReview(it.toApiError()) }
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
            it.copy(step = MeerkatImportStep.RESULT, isImporting = false, result = result)
        }
    }

    /**
     * Drop the server session (and its stored credential). Called from [onCleared],
     * so it runs however the screen is left: the app-bar back icon, the system
     * back button or gesture, or any other pop of the destination. A no-op once
     * the import has been confirmed ([importConfirmed]) or finished.
     */
    fun cancel() {
        val state = _uiState.value
        val sessionId = state.connected?.sessionId ?: return
        if (state.step == MeerkatImportStep.RESULT || importConfirmed) return
        cancelSession(sessionId)
    }

    /**
     * Fire-and-forget cancel that outlives this ViewModel. By the time
     * [onCleared] runs `viewModelScope` is already cancelled, which would abort
     * the in-flight HTTP call; [NonCancellable] keeps it alive. (The codebase
     * has no application-scoped `CoroutineScope` to inject.)
     */
    private fun cancelSession(sessionId: String) {
        viewModelScope.launch(NonCancellable) { apiClient.cancelMeerkatImport(sessionId) }
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
            val status = apiClient.getMeerkatImportStatus(sessionId).getOrNull()
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
     * Back to the connect form. The server would accept a retry of a failed
     * fetch on the same session, but the form was locked and the credential
     * already cleared, so the user connects again and gets a new session. Cancel
     * the abandoned one first: it still holds the Meerkat credential (and a
     * password login) until it expires.
     */
    private fun failToConnect(error: ApiError? = null, text: String? = null) {
        val abandoned = _uiState.value.connected?.sessionId
        _uiState.update {
            it.copy(
                step = MeerkatImportStep.CONNECT,
                isImporting = false,
                connected = null,
                acceptUntested = false,
                preview = null,
                errorRes = if (error == null &&
                    text.isNullOrBlank()
                ) {
                    R.string.meerkat_import_error_failed
                } else {
                    null
                },
                error = error,
                errorText = text?.takeIf { m -> m.isNotBlank() }
            )
        }
        abandoned?.let(::cancelSession)
    }

    private fun failToReview(error: ApiError? = null, text: String? = null) {
        _uiState.update {
            it.copy(
                step = MeerkatImportStep.REVIEW,
                isImporting = false,
                errorRes = if (error == null &&
                    text.isNullOrBlank()
                ) {
                    R.string.meerkat_import_error_failed
                } else {
                    null
                },
                error = error,
                errorText = text?.takeIf { m -> m.isNotBlank() }
            )
        }
    }

    companion object {
        const val POLL_INTERVAL_MILLIS = 500L
        const val MAX_POLLS = 1200

        // Phase tokens: hardcoded mirror of backend models.SourceImportPhase* — keep in sync.
        const val PHASE_READY = "ready"
        const val PHASE_DONE = "done"
        const val PHASE_FAILED = "failed"
        const val PHASE_CANCELLED = "cancelled"
    }
}
