package com.mycorrhizal.crm.feature.settings

import androidx.annotation.StringRes
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.data.attach.AttachFinishResult
import com.mycorrhizal.crm.data.attach.AttachPreview
import com.mycorrhizal.crm.data.attach.AttachProgress
import com.mycorrhizal.crm.data.attach.AttachSignInResult
import com.mycorrhizal.crm.data.attach.AttachToRemoteCoordinator
import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.domain.profile.ServerProfile
import com.mycorrhizal.crm.domain.profile.ServerProfileKind
import com.mycorrhizal.crm.model.network.MycorrhizalImportResult
import com.mycorrhizal.crm.model.network.RowImportAction
import com.mycorrhizal.crm.model.util.Validators
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.toApiError
import com.mycorrhizal.crm.ui.R
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

/** Where the wizard is. Every step before [Done] leaves the Local profile active and writable. */
enum class AttachStep {
    Loading,
    Blocked,
    SignIn,
    TwoFactor,
    Working,

    /** Export/upload/preview failed after sign-in; offers a resumable retry. */
    PrepareFailed,
    Review,
    Done,
}

data class AttachUiState(
    val step: AttachStep = AttachStep.Loading,
    /** Remote profiles the user can pick instead of typing a URL. */
    val remoteProfiles: List<ServerProfile> = emptyList(),
    val progress: AttachProgress? = null,
    val preview: AttachPreview? = null,
    /** Per-row add/skip/update decisions, seeded from each row's suggested action. */
    val rowActions: Map<Int, String> = emptyMap(),
    val result: MycorrhizalImportResult? = null,
    /** The remote import has been applied; only the switch + archive remain. */
    val importApplied: Boolean = false,
    /** Unsynced Local interactions the final switch would drop; non-null shows the confirm dialog. */
    val pendingDiscardCount: Int? = null,
    @StringRes val errorRes: Int? = null,
    val error: String? = null,
)

/**
 * ADR 0028 Decision 3 / issue #1265: the "Move this data to a server" wizard.
 * A thin adapter over [AttachToRemoteCoordinator], which owns the ordering and
 * the invariants (Local stays active until the final switch; the bundle stays in
 * memory; one Idempotency-Key per run). This class only maps step results onto
 * UI state, so it holds no policy of its own.
 */
@HiltViewModel
class AttachToRemoteViewModel @Inject constructor(
    private val coordinator: AttachToRemoteCoordinator,
    private val sessionManager: SessionManager,
) : ViewModel() {

    private val _uiState = MutableStateFlow(AttachUiState())
    val uiState: StateFlow<AttachUiState> = _uiState.asStateFlow()

    init {
        viewModelScope.launch {
            coordinator.begin().fold(
                onSuccess = {
                    val remotes = sessionManager.profiles().filter { it.kind is ServerProfileKind.Remote }
                    _uiState.update { it.copy(step = AttachStep.SignIn, remoteProfiles = remotes) }
                },
                onFailure = {
                    _uiState.update { it.copy(step = AttachStep.Blocked, errorRes = R.string.attach_error_not_local) }
                },
            )
        }
    }

    /**
     * Step 1: sign in to [existingProfileId] or, when null, a new profile for
     * [url]. Then straight on to export → upload → preview.
     */
    fun signIn(existingProfileId: String?, label: String, url: String, identifier: String, password: String) {
        if (_uiState.value.step != AttachStep.SignIn) return
        if (existingProfileId == null && !Validators.isValidServerUrl(url.trim().trimEnd('/'))) {
            _uiState.update { it.copy(errorRes = R.string.servers_add_invalid_url, error = null) }
            return
        }
        if (identifier.isBlank() || password.isEmpty()) {
            _uiState.update { it.copy(errorRes = R.string.attach_error_credentials, error = null) }
            return
        }
        viewModelScope.launch {
            _uiState.update { it.copy(step = AttachStep.Working, errorRes = null, error = null, progress = null) }
            coordinator.signIn(existingProfileId, label, url, identifier.trim(), password).fold(
                onSuccess = { outcome -> onSignedIn(outcome) },
                onFailure = { failure -> fail(AttachStep.SignIn, failure, badCredentialsOn401 = true) },
            )
        }
    }

    fun submitTwoFactor(code: String) {
        if (_uiState.value.step != AttachStep.TwoFactor || code.isBlank()) return
        viewModelScope.launch {
            _uiState.update { it.copy(step = AttachStep.Working, errorRes = null, error = null) }
            coordinator.completeTwoFactor(code.trim()).fold(
                onSuccess = { outcome -> onSignedIn(outcome) },
                onFailure = { failure -> fail(AttachStep.TwoFactor, failure, badCredentialsOn401 = true) },
            )
        }
    }

    /** Re-run the export/upload/preview steps after a failure; resumes where it stopped. */
    fun retryPrepare() {
        viewModelScope.launch { prepare() }
    }

    fun setRowAction(rowIndex: Int, action: String) {
        val row = _uiState.value.preview?.preview?.rows?.find { it.rowIndex == rowIndex } ?: return
        if (row.validationErrors.isNotEmpty()) return
        _uiState.update { it.copy(rowActions = it.rowActions + (rowIndex to action)) }
    }

    /** "Resolve all as merged" — every valid row takes its suggested action. */
    fun resolveAll() {
        val rows = _uiState.value.preview?.preview?.rows ?: return
        _uiState.update { state ->
            val next = state.rowActions.toMutableMap()
            rows.forEach { row -> if (row.validationErrors.isEmpty()) next[row.rowIndex] = row.suggestedAction }
            state.copy(rowActions = next)
        }
    }

    /** Step 5: apply the reviewed decisions on the remote, then switch and archive Local. */
    fun confirm() {
        if (_uiState.value.step != AttachStep.Review) return
        val actions = _uiState.value.rowActions.map { (index, action) -> RowImportAction(index, action) }
        viewModelScope.launch {
            _uiState.update { it.copy(step = AttachStep.Working, errorRes = null, error = null) }
            if (_uiState.value.importApplied) {
                // The remote import already ran (the switch was declined or
                // failed): finishing must not re-post the confirm.
                finish(discardPending = false)
                return@launch
            }
            coordinator.confirm(actions) { progress -> _uiState.update { it.copy(progress = progress) } }.fold(
                onSuccess = { result ->
                    _uiState.update { it.copy(result = result, importApplied = true) }
                    finish(discardPending = false)
                },
                onFailure = { failure -> fail(AttachStep.Review, failure) },
            )
        }
    }

    /** The user agreed to drop the unsent Local interactions named in the dialog. */
    fun confirmDiscardAndFinish() {
        _uiState.update { it.copy(pendingDiscardCount = null, step = AttachStep.Working) }
        viewModelScope.launch { finish(discardPending = true) }
    }

    fun dismissDiscard() {
        // The import is already applied, so the only remaining action is
        // finishing; the Review step's button now does exactly that.
        _uiState.update { it.copy(pendingDiscardCount = null, step = AttachStep.Review) }
    }

    /** Leave the wizard before the switch: drop the remote session and the in-memory bundle. */
    fun cancel() {
        viewModelScope.launch { coordinator.cancel() }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(errorRes = null, error = null) }
    }

    override fun onCleared() {
        // The bundle must not outlive the wizard. close() is synchronous and
        // idempotent; the remote session (if any) expires server-side.
        coordinator.close()
        super.onCleared()
    }

    private suspend fun onSignedIn(outcome: AttachSignInResult) {
        when (outcome) {
            AttachSignInResult.TwoFactorRequired -> _uiState.update { it.copy(step = AttachStep.TwoFactor) }
            AttachSignInResult.SignedIn -> prepare()
        }
    }

    private suspend fun prepare() {
        _uiState.update { it.copy(step = AttachStep.Working, errorRes = null, error = null, progress = null) }
        coordinator.prepare { progress -> _uiState.update { it.copy(progress = progress) } }.fold(
            onSuccess = { attachPreview ->
                val seeded = attachPreview.preview.rows.associate { row ->
                    row.rowIndex to if (row.validationErrors.isEmpty()) row.suggestedAction else "skip"
                }
                _uiState.update {
                    it.copy(step = AttachStep.Review, preview = attachPreview, rowActions = seeded, progress = null)
                }
            },
            onFailure = { failure -> fail(AttachStep.PrepareFailed, failure) },
        )
    }

    private suspend fun finish(discardPending: Boolean) {
        runCatching { coordinator.finish(discardPending) }.fold(
            onSuccess = { outcome ->
                when (outcome) {
                    AttachFinishResult.Done -> _uiState.update { it.copy(step = AttachStep.Done, progress = null) }
                    is AttachFinishResult.NeedsConfirmation -> _uiState.update {
                        it.copy(step = AttachStep.Review, pendingDiscardCount = outcome.pendingCount)
                    }
                }
            },
            onFailure = { failure -> fail(AttachStep.Review, failure) },
        )
    }

    /**
     * Back to [step] with an error line. Every failure path leaves Local active:
     * nothing before [finish] activates or archives anything.
     */
    private fun fail(step: AttachStep, failure: Throwable, badCredentialsOn401: Boolean = false) {
        val apiError = failure.toApiError()
        if (badCredentialsOn401 && apiError is ApiError.Client && apiError.code == HTTP_UNAUTHORIZED) {
            // ApiError's generic 401 text is "Session expired"; on a login attempt it means wrong credentials.
            _uiState.update {
                it.copy(step = step, error = null, errorRes = R.string.attach_error_bad_credentials, progress = null)
            }
            return
        }
        val message = if (apiError is ApiError.Unknown) failure.message else apiError.displayMessage
        _uiState.update {
            it.copy(
                step = step,
                error = message ?: "Something went wrong",
                errorRes = null,
                progress = null,
            )
        }
    }

    private companion object {
        const val HTTP_UNAUTHORIZED = 401
    }
}

