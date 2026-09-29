package com.mycorrhizal.crm.feature.settings

import android.content.Context
import androidx.annotation.StringRes
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.data.passkey.PasskeyAvailability
import com.mycorrhizal.crm.data.passkey.PasskeyCredentialClient
import com.mycorrhizal.crm.data.passkey.PasskeyResult
import com.mycorrhizal.crm.domain.repository.PasskeyRepository
import com.mycorrhizal.crm.model.network.WebAuthnCredential
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

/**
 * Passkey enrollment and management (issue #1293 / ADR 0034, web parity
 * `PasskeySettings.tsx`). Mirrors [TwoFactorViewModel]'s shape:
 *  - list → add (register begin → Credential Manager → register finish) →
 *    per-credential remove, gated on a live second-factor proof;
 *  - recovery codes minted with the account's FIRST second factor are shown
 *    exactly once ([PasskeysUiState.recoveryCodes]);
 *  - a user cancel is silent, an OIDC-provisioned account (403 at register
 *    begin) and a server that isn't associated with this app are persistent
 *    "cannot enroll" states rather than repeating errors.
 *
 * A removal proof is either a TOTP/recovery code, or an assertion from ANOTHER
 * passkey ([PasskeyRepository.beginProof] passes the removed id as `exclude_id`,
 * #1317); the second route is only offered when another passkey exists and the
 * ceremony gate is open.
 *
 * The [Context] parameters are Activity contexts for the provider UI; they are
 * used for the duration of one call and never stored.
 */
@HiltViewModel
class PasskeysViewModel @Inject constructor(
    private val passkeyRepository: PasskeyRepository,
    private val passkeyClient: PasskeyCredentialClient,
    private val passkeyAvailability: PasskeyAvailability,
) : ViewModel() {

    private val _uiState = MutableStateFlow(PasskeysUiState())
    val uiState: StateFlow<PasskeysUiState> = _uiState.asStateFlow()

    init {
        load()
    }

    fun load() {
        if (_uiState.value.busy) return
        _uiState.update { it.copy(loading = true, error = null, errorRes = null) }
        viewModelScope.launch {
            val available = passkeyAvailability.isAvailable()
            passkeyRepository.listPasskeys()
                .onSuccess { list ->
                    _uiState.update { it.copy(loading = false, available = available, passkeys = list) }
                }
                .onFailure { e ->
                    _uiState.update { it.copy(loading = false, available = available, error = e.serverText()) }
                }
        }
    }

    /** Open the "add a passkey" (name) dialog. */
    fun startAdd() {
        val state = _uiState.value
        if (state.busy || !state.canAdd) return
        _uiState.update { it.copy(adding = true, error = null, errorRes = null, messageRes = null) }
    }

    fun dismissAdd() {
        if (_uiState.value.busy) return
        _uiState.update { it.copy(adding = false, error = null, errorRes = null) }
    }

    /** Run the whole enrollment ceremony. A blank [name] lets the server choose a default label. */
    fun addPasskey(context: Context, name: String) {
        val state = _uiState.value
        if (state.busy || !state.canAdd) return
        _uiState.update { it.copy(busy = true, error = null, errorRes = null, messageRes = null) }
        viewModelScope.launch {
            val options = passkeyRepository.beginRegistration(name).getOrElse { e ->
                onAddBeginFailed(e)
                return@launch
            }
            when (val ceremony = passkeyClient.createPasskey(context, options)) {
                is PasskeyResult.Success -> finishAdd(ceremony.json)
                PasskeyResult.Cancelled -> _uiState.update { it.copy(busy = false, adding = false) }
                PasskeyResult.NotAssociated ->
                    _uiState.update {
                        it.copy(busy = false, adding = false, blockedRes = R.string.settings_passkeys_not_associated)
                    }
                PasskeyResult.NoProvider ->
                    _uiState.update {
                        it.copy(busy = false, adding = false, blockedRes = R.string.settings_passkeys_no_provider)
                    }
                PasskeyResult.AlreadyRegistered ->
                    _uiState.update {
                        it.copy(busy = false, errorRes = R.string.settings_passkeys_already_registered)
                    }
                PasskeyResult.NoMatchingPasskey, is PasskeyResult.Failed ->
                    _uiState.update { it.copy(busy = false, errorRes = R.string.settings_passkeys_add_error) }
            }
        }
    }

    private suspend fun finishAdd(attestationJson: String) {
        val registered = passkeyRepository.finishRegistration(attestationJson).getOrElse { e ->
            _uiState.update { it.copy(busy = false, error = e.serverText()) }
            return
        }
        val refreshed = passkeyRepository.listPasskeys().getOrNull()
        _uiState.update { state ->
            state.copy(
                busy = false,
                adding = false,
                passkeys = refreshed ?: state.passkeys,
                recoveryCodes = registered.recoveryCodes.takeIf { it.isNotEmpty() },
                messageRes = R.string.settings_passkeys_add_success,
            )
        }
    }

    private fun onAddBeginFailed(error: Throwable) {
        val apiError = error.toApiError()
        if (apiError is ApiError.Client && apiError.code == HTTP_FORBIDDEN) {
            // The backend's oidcUserErr: an identity-provider account cannot enroll.
            // Persistent, like the TOTP screen's server-worded 403 — but it also
            // withdraws the Add action rather than offering a button that can only fail.
            _uiState.update {
                it.copy(busy = false, adding = false, blockedText = apiError.serverText())
            }
        } else {
            _uiState.update { it.copy(busy = false, error = error.serverText()) }
        }
    }

    /** Ask to remove [credential]; opens the proof dialog. */
    fun requestRemove(credential: WebAuthnCredential) {
        if (_uiState.value.busy) return
        _uiState.update { it.copy(removing = credential, error = null, errorRes = null, messageRes = null) }
    }

    fun dismissRemove() {
        if (_uiState.value.busy) return
        _uiState.update { it.copy(removing = null, error = null, errorRes = null) }
    }

    /** Remove the credential under [PasskeysUiState.removing], proven by a TOTP / recovery [code]. */
    fun removeWithCode(code: String) {
        val target = _uiState.value.removing ?: return
        if (_uiState.value.busy || code.isBlank()) return
        _uiState.update { it.copy(busy = true, error = null, errorRes = null) }
        viewModelScope.launch {
            passkeyRepository.removeWithCode(target.id, code.trim())
                .onSuccess { afterRemove(target) }
                .onFailure { e -> _uiState.update { it.copy(busy = false).withProofError(e) } }
        }
    }

    /**
     * Remove the credential under [PasskeysUiState.removing], proven by an
     * assertion from ANOTHER passkey. No-op when [PasskeysUiState.canProveWithPasskey]
     * is false (the option is hidden then).
     */
    fun removeWithAnotherPasskey(context: Context) {
        val state = _uiState.value
        val target = state.removing ?: return
        if (state.busy || !state.canProveWithPasskey) return
        _uiState.update { it.copy(busy = true, error = null, errorRes = null) }
        viewModelScope.launch {
            val options = passkeyRepository.beginProof(target.id).getOrElse { e ->
                _uiState.update { it.copy(busy = false).withProofError(e) }
                return@launch
            }
            when (val ceremony = passkeyClient.getPasskey(context, options)) {
                is PasskeyResult.Success ->
                    passkeyRepository.removeWithAssertion(target.id, ceremony.json)
                        .onSuccess { afterRemove(target) }
                        .onFailure { e -> _uiState.update { it.copy(busy = false).withProofError(e) } }
                PasskeyResult.Cancelled -> _uiState.update { it.copy(busy = false) }
                PasskeyResult.NoMatchingPasskey ->
                    _uiState.update { it.copy(busy = false, errorRes = R.string.settings_passkeys_no_other_here) }
                PasskeyResult.NotAssociated ->
                    _uiState.update { it.copy(busy = false, errorRes = R.string.settings_passkeys_not_associated) }
                PasskeyResult.NoProvider ->
                    _uiState.update { it.copy(busy = false, errorRes = R.string.settings_passkeys_no_provider) }
                PasskeyResult.AlreadyRegistered, is PasskeyResult.Failed ->
                    _uiState.update { it.copy(busy = false, errorRes = R.string.settings_passkeys_invalid_proof) }
            }
        }
    }

    private suspend fun afterRemove(removed: WebAuthnCredential) {
        val refreshed = passkeyRepository.listPasskeys().getOrNull()
        _uiState.update { state ->
            state.copy(
                busy = false,
                removing = null,
                passkeys = refreshed ?: state.passkeys.filterNot { it.id == removed.id },
                messageRes = R.string.settings_passkeys_remove_success,
            )
        }
    }

    /** Dismiss the exactly-once recovery-codes dialog. */
    fun dismissRecoveryCodes() {
        _uiState.update { it.copy(recoveryCodes = null) }
    }

    fun onMessageShown() {
        _uiState.update { it.copy(messageRes = null) }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null, errorRes = null) }
    }

    /**
     * 400 is the backend's "proof rejected" (bad/missing code or a failed
     * assertion) → localized copy. 404 (unknown id) and 409 (no other passkey
     * remains) carry a specific server message, shown verbatim.
     */
    private fun PasskeysUiState.withProofError(error: Throwable): PasskeysUiState {
        val apiError = error.toApiError()
        return if (apiError is ApiError.Client && apiError.code == HTTP_BAD_REQUEST) {
            copy(errorRes = R.string.settings_passkeys_invalid_proof)
        } else {
            copy(error = error.serverText())
        }
    }

    private companion object {
        const val HTTP_BAD_REQUEST = 400
        const val HTTP_FORBIDDEN = 403
    }
}

data class PasskeysUiState(
    val loading: Boolean = true,
    /** The ceremony gate (server capability + device support); false hides Add and the passkey-proof route. */
    val available: Boolean = false,
    val passkeys: List<WebAuthnCredential> = emptyList(),
    /** True while a network call or ceremony is in flight. */
    val busy: Boolean = false,
    /** The name dialog for a new passkey is open. */
    val adding: Boolean = false,
    /** The credential whose removal-proof dialog is open. */
    val removing: WebAuthnCredential? = null,
    /** Recovery codes minted with the account's first second factor — shown exactly once. */
    val recoveryCodes: List<String>? = null,
    /** Persistent "cannot enroll" reason (this server isn't associated / no provider on the device). */
    @StringRes val blockedRes: Int? = null,
    /** Persistent "cannot enroll" reason worded by the server (OIDC-provisioned account). */
    val blockedText: String? = null,
    /** A one-shot success line, shown then cleared. */
    @StringRes val messageRes: Int? = null,
    val error: String? = null,
    @StringRes val errorRes: Int? = null,
) {
    /** Enrollment is offered only through an open gate and for an account that is not blocked. */
    val canAdd: Boolean get() = available && blockedRes == null && blockedText == null

    /** "Verify with another passkey" needs the gate and at least one passkey besides the removed one. */
    val canProveWithPasskey: Boolean get() = available && removing != null && passkeys.size > 1
}

/** A server-worded message where the server supplies one (404/409/403/…), else the generic text. */
private fun Throwable.serverText(): String {
    val apiError = toApiError()
    return if (apiError is ApiError.Client) {
        apiError.message?.takeIf { it.isNotBlank() } ?: apiError.displayMessage
    } else {
        apiError.displayMessage
    }
}
