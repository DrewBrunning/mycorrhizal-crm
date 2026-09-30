package com.mycorrhizal.crm.feature.settings

import android.content.Context
import androidx.annotation.StringRes
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.data.passkey.PasskeyAvailability
import com.mycorrhizal.crm.data.passkey.PasskeyCredentialClient
import com.mycorrhizal.crm.data.passkey.PasskeyResult
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.PasskeyRepository
import com.mycorrhizal.crm.domain.repository.SecondFactorProof
import com.mycorrhizal.crm.model.network.TwoFactorSetupResponse
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.toApiError
import com.mycorrhizal.crm.ui.R
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** Which code-gated action is prompting for a verification code. */
enum class TwoFactorPrompt { DISABLE, REGENERATE }

/**
 * Two-factor (TOTP) enrollment and management state (issue #158 web parity,
 * Android #814). Mirrors web `TwoFactorSettings.tsx`:
 *  - status → enable (setup mints a secret/QR, confirm with a live code) or
 *    regenerate/disable (each gated on a live code);
 *  - recovery codes are shown plaintext exactly once ([recoveryCodes]) and
 *    the secret/URL in [setup] are transient — nothing 2FA-related is kept in
 *    the session or persisted beyond the normal bearer token.
 *
 * A passkey-only account already holds a second factor, so enabling TOTP there
 * first asks for a live proof (issue #1337): a recovery code, or an assertion
 * from one of its passkeys ([submitSetupProofWithPasskey]). The first factor
 * needs none.
 *
 * Error mapping mirrors web: a rejected code (400) maps to the localized
 * "Invalid code" text; setup's 403 (OIDC account) / 409 (already enabled) and
 * any 429 surface the server's own message.
 */
@HiltViewModel
class TwoFactorViewModel @Inject constructor(
    private val authRepository: AuthRepository,
    private val passkeyRepository: PasskeyRepository,
    private val passkeyClient: PasskeyCredentialClient,
    private val passkeyAvailability: PasskeyAvailability,
) : ViewModel() {

    private val _uiState = MutableStateFlow(TwoFactorUiState())
    val uiState: StateFlow<TwoFactorUiState> = _uiState.asStateFlow()

    init {
        load()
    }

    fun load() {
        if (_uiState.value.busy) return
        _uiState.update { it.copy(loading = true, error = null, errorRes = null) }
        viewModelScope.launch {
            // Decides only whether Enable asks for a proof first; the server enforces
            // it either way, so a failed lookup is not fatal.
            val hasPasskey = passkeyRepository.listPasskeys().getOrNull()?.isNotEmpty() ?: false
            val available = passkeyAvailability.isAvailable()
            authRepository.getTwoFactorStatus()
                .onSuccess { status ->
                    _uiState.update {
                        it.copy(
                            loading = false,
                            enabled = status.enabled,
                            hasPasskey = hasPasskey,
                            passkeysAvailable = available,
                        )
                    }
                }
                .onFailure { e ->
                    _uiState.update { it.copy(loading = false, error = e.displayText()) }
                }
        }
    }

    /**
     * Begin enrollment: mints a pending secret the wizard shows as QR + manual
     * key. An account that already holds a passkey is asked for a proof first
     * ([TwoFactorUiState.proofPrompt]).
     */
    fun startSetup() {
        val state = _uiState.value
        if (state.busy) return
        if (state.hasPasskey) {
            _uiState.update { it.copy(proofPrompt = true, error = null, errorRes = null) }
            return
        }
        runSetup(null)
    }

    /** Setup proven by a recovery [code] (passkey-only account). */
    fun submitSetupProofCode(code: String) {
        val state = _uiState.value
        if (state.busy || !state.proofPrompt || code.isBlank()) return
        runSetup(SecondFactorProof.Code(code.trim()))
    }

    /**
     * Setup proven by an assertion from an existing passkey. No-op when
     * [TwoFactorUiState.canProveWithPasskey] is false (the option is hidden then).
     */
    fun submitSetupProofWithPasskey(context: Context) {
        val state = _uiState.value
        if (state.busy || !state.proofPrompt || !state.canProveWithPasskey) return
        proveWithPasskey(context) { runSetup(it) }
    }

    /**
     * Regenerate proven by an assertion from a passkey (issue #1354) — the proof
     * a passkey-only account can give once its recovery codes are spent.
     */
    fun submitRegenerateWithPasskey(context: Context) {
        val state = _uiState.value
        if (state.busy || state.prompt != TwoFactorPrompt.REGENERATE || !state.canProveWithPasskey) return
        proveWithPasskey(context) { runRegenerate(it) }
    }

    /** Runs the proof ceremony and hands the assertion to [onProof]; failures map to localized copy. */
    private fun proveWithPasskey(context: Context, onProof: (SecondFactorProof) -> Unit) {
        _uiState.update { it.copy(busy = true, error = null, errorRes = null) }
        viewModelScope.launch {
            val options = passkeyRepository.beginProof().getOrElse { e ->
                _uiState.update { it.copy(busy = false, error = e.displayText()) }
                return@launch
            }
            when (val ceremony = passkeyClient.getPasskey(context, options)) {
                is PasskeyResult.Success -> {
                    _uiState.update { it.copy(busy = false) }
                    onProof(SecondFactorProof.Assertion(ceremony.json))
                }
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

    private fun runRegenerate(proof: SecondFactorProof) {
        _uiState.update { it.copy(busy = true, error = null, errorRes = null) }
        viewModelScope.launch {
            authRepository.regenerateRecoveryCodes(proof)
                .onSuccess { result ->
                    _uiState.update {
                        it.copy(
                            busy = false,
                            prompt = null,
                            recoveryCodes = result.recoveryCodes.takeIf { codes -> codes.isNotEmpty() },
                        )
                    }
                }
                .onFailure { e ->
                    _uiState.update { it.copy(busy = false).withCodeError(e) }
                }
        }
    }

    fun dismissSetupProof() {
        if (_uiState.value.busy) return
        _uiState.update { it.copy(proofPrompt = false, error = null, errorRes = null) }
    }

    private fun runSetup(proof: SecondFactorProof?) {
        _uiState.update { it.copy(busy = true, error = null, errorRes = null) }
        viewModelScope.launch {
            authRepository.setupTwoFactor(proof)
                .onSuccess { setup ->
                    _uiState.update { it.copy(busy = false, setup = setup, proofPrompt = false) }
                }
                .onFailure { e ->
                    _uiState.update {
                        // A rejected proof (400) keeps the proof dialog open with localized copy.
                        if (proof != null) it.copy(busy = false).withCodeError(e) else it.copy(busy = false, error = e.displayText())
                    }
                }
        }
    }

    /** Confirm enrollment with a live TOTP code; on success 2FA is on and the recovery codes are shown once. */
    fun confirmSetup(code: String) {
        if (_uiState.value.busy || _uiState.value.setup == null || code.isBlank()) return
        _uiState.update { it.copy(busy = true, error = null, errorRes = null) }
        viewModelScope.launch {
            authRepository.confirmTwoFactor(code.trim())
                .onSuccess { result ->
                    _uiState.update {
                        it.copy(
                            busy = false,
                            setup = null,
                            enabled = true,
                            recoveryCodes = result.recoveryCodes,
                        )
                    }
                }
                .onFailure { e ->
                    _uiState.update { it.copy(busy = false).withCodeError(e) }
                }
        }
    }

    /** Close the enrollment wizard without confirming (the pending secret is dropped). */
    fun closeSetup() {
        if (_uiState.value.busy) return
        _uiState.update { it.copy(setup = null, error = null, errorRes = null) }
    }

    fun requestDisable() = requestPrompt(TwoFactorPrompt.DISABLE)

    fun requestRegenerate() = requestPrompt(TwoFactorPrompt.REGENERATE)

    private fun requestPrompt(prompt: TwoFactorPrompt) {
        if (_uiState.value.busy) return
        _uiState.update { it.copy(prompt = prompt, error = null, errorRes = null) }
    }

    fun dismissPrompt() {
        if (_uiState.value.busy) return
        _uiState.update { it.copy(prompt = null, error = null, errorRes = null) }
    }

    /**
     * Submit the live code the current [TwoFactorPrompt] asked for: disable
     * turns 2FA off; regenerate mints a fresh set of recovery codes (shown
     * once).
     */
    fun submitPromptCode(code: String) {
        val prompt = _uiState.value.prompt ?: return
        if (_uiState.value.busy || code.isBlank()) return
        if (prompt == TwoFactorPrompt.REGENERATE) {
            runRegenerate(SecondFactorProof.Code(code.trim()))
            return
        }
        _uiState.update { it.copy(busy = true, error = null, errorRes = null) }
        viewModelScope.launch {
            authRepository.disableTwoFactor(code.trim())
                .onSuccess {
                    _uiState.update { it.copy(busy = false, prompt = null, enabled = false) }
                }
                .onFailure { e ->
                    _uiState.update { it.copy(busy = false).withCodeError(e) }
                }
        }
    }

    /** Dismiss the exactly-once recovery-codes dialog. */
    fun dismissRecoveryCodes() {
        _uiState.update { it.copy(recoveryCodes = null) }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null, errorRes = null) }
    }

    private fun TwoFactorUiState.withCodeError(error: Throwable): TwoFactorUiState {
        val apiError = error.toApiError()
        return if (apiError is ApiError.Client && apiError.code == 400) {
            copy(errorRes = R.string.settings_two_factor_invalid_code)
        } else {
            copy(error = error.displayText())
        }
    }
}

data class TwoFactorUiState(
    /** null while the initial status is still loading. */
    val enabled: Boolean? = null,
    val loading: Boolean = true,
    /** True while a mutation (setup/confirm/disable/regenerate) is in flight. */
    val busy: Boolean = false,
    /** The pending setup wizard (secret + otpauth URL), transient. */
    val setup: TwoFactorSetupResponse? = null,
    /** The just-minted recovery codes — shown exactly once, then dismissed. */
    val recoveryCodes: List<String>? = null,
    /** Which code-gated action is prompting (disable/regenerate). */
    val prompt: TwoFactorPrompt? = null,
    /** The account holds a passkey, so enabling TOTP needs a live proof first (issue #1337). */
    val hasPasskey: Boolean = false,
    /** The passkey ceremony gate is open on this device (server capability + Credential Manager). */
    val passkeysAvailable: Boolean = false,
    /** The setup-proof dialog is open. */
    val proofPrompt: Boolean = false,
    /** A transient action error (server text), shown and then cleared. */
    val error: String? = null,
    /** A localized action error (rejected code), shown and then cleared. */
    @StringRes val errorRes: Int? = null,
) {
    /** "Verify with a passkey" for the setup proof: the gate is open and a passkey exists to answer. */
    val canProveWithPasskey: Boolean get() = hasPasskey && passkeysAvailable
}

private fun Throwable.displayText(): String {
    val apiError = this as? ApiError ?: return message ?: "error"
    // 403 maps to a generic permission message in ApiError.displayMessage, but
    // the OIDC-account "2FA unavailable" reason is the informative part.
    return if (apiError is ApiError.Client && apiError.code == 403) {
        apiError.message ?: apiError.displayMessage
    } else {
        apiError.displayMessage
    }
}
