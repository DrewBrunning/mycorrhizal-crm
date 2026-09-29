package com.mycorrhizal.crm.feature.auth

import android.content.Context
import androidx.annotation.StringRes
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.data.passkey.PasskeyAvailability
import com.mycorrhizal.crm.data.passkey.PasskeyCredentialClient
import com.mycorrhizal.crm.data.passkey.PasskeyResult
import com.mycorrhizal.crm.data.passkey.PasskeyIssue
import com.mycorrhizal.crm.data.passkey.SecondFactorPrompt
import com.mycorrhizal.crm.data.passkey.offersPasskey
import com.mycorrhizal.crm.data.passkey.secondFactorPrompt
import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.usecase.LoginUseCase
import com.mycorrhizal.crm.domain.usecase.LoginWithApiTokenUseCase
import com.mycorrhizal.crm.model.network.includesPasskey
import com.mycorrhizal.crm.model.util.Validators
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.toApiError
import com.mycorrhizal.crm.ui.R
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class LoginUiState(
    val serverUrl: String = "",
    val mode: LoginMode = LoginMode.PASSWORD,
    val isLoading: Boolean = false,
    /** Static validation error as a string resource id, resolved in the UI. */
    @StringRes val errorRes: Int? = null,
    /** Dynamic server error text (e.g. "Invalid credentials"). */
    val error: String? = null,
    // N8 (#814): the password step succeeded but the account has 2FA — the UI
    // swaps to a code-entry step until the TOTP/recovery code clears.
    val twoFactorStep: Boolean = false,
    // Issue #1293 (ADR 0034 Decision 4): how the 2FA step presents itself,
    // decided from the account's enrolled `methods` and the passkey gate.
    val twoFactorPrompt: SecondFactorPrompt = SecondFactorPrompt.STANDARD,
    // Kept so a wrong-code retry rebuilds the same prompt.
    val twoFactorMethods: List<String>? = null,
    // Issue #1293: set when a passkey attempt dropped the step to the degraded
    // (code-only) state, so the copy can say why. Null = no runtime failure.
    val passkeyIssue: PasskeyIssue? = null,
)

enum class LoginMode { PASSWORD, API_TOKEN }

sealed interface LoginEvent {
    data object LoggedIn : LoginEvent
    data object ServerUrlUpdated : LoginEvent
}

@HiltViewModel
class LoginViewModel @Inject constructor(
    private val loginUseCase: LoginUseCase,
    private val loginWithApiTokenUseCase: LoginWithApiTokenUseCase,
    private val sessionManager: SessionManager,
    // N8 (#814): the 2FA code step. It talks to the repository directly (not a
    // use case) because the HTTP status it must distinguish — 400 invalid
    // code vs 401 expired challenge vs 429 lockout — is an ApiError concept
    // that core:domain deliberately does not depend on (see RegisterViewModel
    // for the same direct-repository pattern).
    private val authRepository: AuthRepository,
    // Issue #1293 / ADR 0034: the passkey capability gate and the Credential Manager seam.
    private val passkeyAvailability: PasskeyAvailability,
    private val passkeyClient: PasskeyCredentialClient,
) : ViewModel() {

    private val _uiState = MutableStateFlow(LoginUiState())
    val uiState: StateFlow<LoginUiState> = _uiState.asStateFlow()

    private val _events = Channel<LoginEvent>(Channel.BUFFERED)
    val events = _events.receiveAsFlow()

    init {
        // Issue #723: pre-fill the server URL persisted across logout (a
        // self-hoster's origin effectively never changes, so re-entering it
        // after every logout is friction with no upside). Await the startup
        // session hydration first — on a cold start after logout the cached
        // URL is still being loaded from disk when this runs, and reading
        // before init() would return null. Never clobber a URL the user has
        // already started typing in the brief window before hydration lands.
        viewModelScope.launch {
            sessionManager.awaitHydrated()
            val storedUrl = sessionManager.serverUrl()
            if (!storedUrl.isNullOrBlank()) {
                _uiState.update { current ->
                    if (current.serverUrl.isBlank()) current.copy(serverUrl = storedUrl) else current
                }
            }
        }
    }

    fun onServerUrlChange(value: String) {
        _uiState.update { it.copy(serverUrl = value) }
        // Persist immediately (not just on submit): the register and
        // forgot-password flows are reached from this screen and read the
        // server URL from the session manager (M26).
        viewModelScope.launch { sessionManager.setServerUrl(value.trim().trimEnd('/')) }
    }

    fun onModeChange(mode: LoginMode) {
        _uiState.update { it.copy(mode = mode, errorRes = null, error = null) }
    }

    /**
     * Authenticate with the given credentials. The values are deliberately
     * passed in rather than stored in [LoginUiState] — a password or API token
     * must not linger in ViewModel state after the attempt (security).
     */
    fun onSubmit(
        serverUrl: String,
        identifier: String,
        password: String,
        apiToken: String,
    ) {
        if (_uiState.value.isLoading) return

        val trimmedUrl = serverUrl.trim().trimEnd('/')
        if (!Validators.isValidServerUrl(trimmedUrl)) {
            _uiState.update {
                it.copy(errorRes = R.string.login_error_valid_server_url, error = null)
            }
            return
        }

        // Field-level validation (blank checks, token prefix) is a UI concern —
        // these are localized here, not in the domain use cases.
        val validationError = when (_uiState.value.mode) {
            LoginMode.PASSWORD -> when {
                identifier.isBlank() -> R.string.login_error_identifier_required
                password.isBlank() -> R.string.login_error_password_required
                else -> null
            }
            LoginMode.API_TOKEN -> when {
                apiToken.isBlank() -> R.string.login_error_token_required
                !apiToken.startsWith("mycorrhizal_") -> R.string.login_error_token_prefix
                else -> null
            }
        }
        if (validationError != null) {
            _uiState.update { it.copy(errorRes = validationError, error = null) }
            return
        }

        _uiState.update { it.copy(isLoading = true, errorRes = null, error = null) }
        viewModelScope.launch {
            sessionManager.setServerUrl(trimmedUrl)
            _events.send(LoginEvent.ServerUrlUpdated)

            val result = when (_uiState.value.mode) {
                LoginMode.PASSWORD -> loginUseCase(identifier, password)
                LoginMode.API_TOKEN -> loginWithApiTokenUseCase(apiToken)
            }

            when (result) {
                is LoginUseCase.Result.Success -> {
                    _uiState.update { it.copy(isLoading = false) }
                    _events.send(LoginEvent.LoggedIn)
                }
                // N8 (#814): 2FA account — the password was correct but no
                // session exists yet. Stay on this screen's code step.
                is LoginUseCase.Result.TwoFactorRequired -> {
                    _uiState.update {
                        it.copy(
                            isLoading = false,
                            twoFactorStep = true,
                            twoFactorMethods = result.methods,
                            twoFactorPrompt = secondFactorPrompt(
                                result.methods,
                                result.methods.includesPasskey() && passkeyAvailability.isAvailable(),
                            ),
                            passkeyIssue = null,
                            errorRes = null,
                            error = null,
                        )
                    }
                }
                is LoginUseCase.Result.Failure -> {
                    _uiState.update { it.copy(isLoading = false, error = result.message) }
                }
                is LoginWithApiTokenUseCase.Result.Success -> {
                    _uiState.update { it.copy(isLoading = false) }
                    _events.send(LoginEvent.LoggedIn)
                }
                is LoginWithApiTokenUseCase.Result.Failure -> {
                    _uiState.update { it.copy(isLoading = false, error = result.message) }
                }
            }
        }
    }

    /** The user left the 2FA code step (back button) — return to the credentials form. */
    fun onBackToCredentials() {
        if (_uiState.value.isLoading) return
        _uiState.update {
            it.copy(
                twoFactorStep = false,
                twoFactorPrompt = SecondFactorPrompt.STANDARD,
                twoFactorMethods = null,
                passkeyIssue = null,
                errorRes = null,
                error = null,
            )
        }
    }

    /**
     * Step 2 of a 2FA login: exchange the TOTP/recovery code for a session.
     * The transient 2fa_pending challenge is held in the repository (memory
     * only) — the code itself is passed in and never stored in [LoginUiState].
     *
     * Error mapping mirrors web `auth.ts login2FA` + `LoginPage.tsx`:
     *  - 400 wrong code → localized "Invalid code", stay on the step.
     *  - 401 challenge consumed/expired/disabled → back to the credentials
     *    step with a "sign in again" message (a retry here would always 401).
     *  - 429 lockout → the server's lockout text verbatim; stop grinding.
     */
    fun onSubmitTwoFactorCode(code: String) {
        if (_uiState.value.isLoading || code.isBlank()) return
        _uiState.update { it.copy(isLoading = true, errorRes = null, error = null) }
        viewModelScope.launch {
            val outcome = authRepository.complete2faLogin(code.trim())
            outcome.fold(
                onSuccess = {
                    _uiState.update { it.copy(isLoading = false) }
                    _events.send(LoginEvent.LoggedIn)
                },
                onFailure = { error ->
                    handleTwoFactorFailure(error)
                },
            )
        }
    }

    /**
     * Issue #1293 / ADR 0034: the passkey alternative to [onSubmitTwoFactorCode].
     * begin → Credential Manager → finish, persisting the session through the
     * same repository path a code uses. [context] should be an Activity (the
     * provider UI launches from it); it is used for this call only and never
     * retained. The code field stays reachable in every outcome:
     *  - user cancel → silent, back to the step;
     *  - association failure / no provider → the degraded (code-only) prompt
     *    with copy saying why;
     *  - expired challenge (begin 401) → back to the credentials step;
     *  - lockout → the server's text; anything else → "passkey sign-in failed".
     */
    fun onUsePasskey(context: Context) {
        val current = _uiState.value
        if (current.isLoading || !current.twoFactorStep || !current.twoFactorPrompt.offersPasskey) return
        _uiState.update { it.copy(isLoading = true, errorRes = null, error = null) }
        viewModelScope.launch {
            val options = authRepository.beginPasskeyLogin().getOrElse { error ->
                handlePasskeyFailure(error, fromBegin = true)
                return@launch
            }
            when (val ceremony = passkeyClient.getPasskey(context, options)) {
                is PasskeyResult.Success -> authRepository.completePasskeyLogin(ceremony.json).fold(
                    onSuccess = {
                        _uiState.update { it.copy(isLoading = false) }
                        _events.send(LoginEvent.LoggedIn)
                    },
                    onFailure = { error -> handlePasskeyFailure(error, fromBegin = false) },
                )
                PasskeyResult.Cancelled -> _uiState.update { it.copy(isLoading = false) }
                PasskeyResult.NoMatchingPasskey ->
                    _uiState.update { it.copy(isLoading = false, errorRes = R.string.login_error_passkey_no_match) }
                PasskeyResult.NotAssociated -> degradePasskey(PasskeyIssue.NOT_ASSOCIATED)
                PasskeyResult.NoProvider -> degradePasskey(PasskeyIssue.NO_PROVIDER)
                PasskeyResult.AlreadyRegistered, is PasskeyResult.Failed ->
                    _uiState.update { it.copy(isLoading = false, errorRes = R.string.login_error_passkey_failed) }
            }
        }
    }

    /** Drops the step to the code-only prompt (gate closed) and remembers why, for the copy. */
    private fun degradePasskey(issue: PasskeyIssue) {
        _uiState.update {
            it.copy(
                isLoading = false,
                twoFactorPrompt = secondFactorPrompt(it.twoFactorMethods, passkeyAvailable = false),
                passkeyIssue = issue,
            )
        }
    }

    private fun handlePasskeyFailure(error: Throwable, fromBegin: Boolean) {
        val apiError = error.toApiError()
        val expired = fromBegin && apiError is ApiError.Client && apiError.code == 401
        when {
            expired -> handleTwoFactorFailure(error)
            apiError is ApiError.Client && apiError.code == 429 ->
                // Lockout: surface the server's message and stop (web parity).
                _uiState.update { it.copy(isLoading = false, error = apiError.displayMessage) }
            else -> _uiState.update { it.copy(isLoading = false, errorRes = R.string.login_error_passkey_failed) }
        }
    }

    private fun handleTwoFactorFailure(error: Throwable) {
        val apiError = error.toApiError()
        val base = LoginUiState(
            serverUrl = _uiState.value.serverUrl,
            mode = LoginMode.PASSWORD,
            isLoading = false,
            twoFactorStep = true,
            twoFactorMethods = _uiState.value.twoFactorMethods,
            twoFactorPrompt = _uiState.value.twoFactorPrompt,
            passkeyIssue = _uiState.value.passkeyIssue,
        )
        _uiState.value = when {
            apiError is ApiError.Client && apiError.code == 401 ->
                // The 10-minute challenge expired (or was already consumed /
                // 2FA was disabled) — a retry here can never succeed.
                base.copy(twoFactorStep = false, errorRes = R.string.login_error_two_factor_expired)
            apiError is ApiError.Client && apiError.code == 429 ->
                // Lockout: surface the server's message and stop (web parity).
                base.copy(error = apiError.displayMessage)
            else ->
                // 400 wrong code and any transient/network failure keep the
                // user on the step with the localized "invalid code" copy.
                base.copy(errorRes = R.string.login_error_two_factor_invalid)
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(errorRes = null, error = null) }
    }
}
