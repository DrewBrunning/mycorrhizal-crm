package com.mycorrhizal.crm.feature.auth

import android.app.Activity
import android.content.Context
import androidx.annotation.StringRes
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.data.auth.LocalAuthOutcome
import com.mycorrhizal.crm.data.auth.LocalAuthPrompt
import com.mycorrhizal.crm.data.passkey.PasskeyAvailability
import com.mycorrhizal.crm.data.passkey.PasskeyCredentialClient
import com.mycorrhizal.crm.data.passkey.PasskeyIssue
import com.mycorrhizal.crm.data.passkey.PasskeyResult
import com.mycorrhizal.crm.data.passkey.SecondFactorPrompt
import com.mycorrhizal.crm.data.passkey.offersPasskey
import com.mycorrhizal.crm.data.passkey.secondFactorPrompt
import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.DeviceGrantRepository
import com.mycorrhizal.crm.domain.repository.LocalAuthCapabilities
import com.mycorrhizal.crm.domain.repository.PasswordlessPasskeyChallenge
import com.mycorrhizal.crm.domain.usecase.LoginUseCase
import com.mycorrhizal.crm.domain.usecase.LoginWithApiTokenUseCase
import com.mycorrhizal.crm.model.network.includesPasskey
import com.mycorrhizal.crm.model.util.Validators
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.toApiError
import com.mycorrhizal.crm.ui.R
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.Job
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class LoginUiState(
    val serverUrl: String = "",
    val mode: LoginMode = LoginMode.PASSWORD,
    val isLoading: Boolean = false,
    /** Static validation error as a string resource id, resolved in the UI. */
    @StringRes val errorRes: Int? = null,
    /** The failed request, localized in the UI by `localizedMessage()`. */
    val error: ApiError? = null,
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
    // Discussion #2355: the credentials step offers passwordless passkey sign-in
    // when the device can run Credential Manager and the server declares Android
    // passkeys. Kept separate from the 2FA-step gate because the user is not
    // known yet; the true check still runs when the button is tapped.
    val passkeyAvailable: Boolean = false,
    // Issue #722 follow-up: the login screen offers biometric re-auth from the
    // stored device grant when the active profile has one, the local device can
    // complete the OS check, and the typed URL still matches that profile.
    val biometricAvailable: Boolean = false
)

/**
 * A rejected credential (401) has no reason code yet, so it gets its own copy
 * instead of `localizedMessage()`'s "session expired"; any other failure is shown
 * as the typed [ApiError].
 */
private fun LoginUiState.failedWith(cause: Throwable?): LoginUiState {
    val apiError = cause?.toApiError() ?: ApiError.Unknown(IllegalStateException("Login failed"))
    return if (apiError is ApiError.Client && apiError.code == 401 && apiError.reason == null) {
        copy(isLoading = false, errorRes = R.string.login_error_invalid_credentials, error = null)
    } else {
        copy(isLoading = false, error = apiError)
    }
}

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
    // Issue #722 follow-up: login-screen biometric re-auth from a stored device grant.
    private val deviceGrantRepository: DeviceGrantRepository,
    private val localAuthCapabilities: LocalAuthCapabilities,
    private val localAuthPrompt: LocalAuthPrompt
) : ViewModel() {

    private val _uiState = MutableStateFlow(LoginUiState())
    val uiState: StateFlow<LoginUiState> = _uiState.asStateFlow()

    private val _events = Channel<LoginEvent>(Channel.BUFFERED)
    val events = _events.receiveAsFlow()

    private var passkeyAvailabilityJob: Job? = null
    private var biometricAvailabilityJob: Job? = null

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
                    if (current.serverUrl.isBlank()) {
                        current.copy(
                            serverUrl = storedUrl
                        )
                    } else {
                        current
                    }
                }
            }
            refreshPasskeyAvailability()
            refreshBiometricAvailability()
        }
    }

    /**
     * Discussion #2355: recompute whether the credentials step should offer
     * passwordless passkey sign-in. Debounced while the user types a server URL
     * so we do not query `/health` on every keystroke.
     */
    private fun refreshPasskeyAvailability(debounce: Boolean = false) {
        passkeyAvailabilityJob?.cancel()
        passkeyAvailabilityJob = viewModelScope.launch {
            if (debounce) delay(PASSKEY_AVAILABILITY_DEBOUNCE_MS)
            val available = passkeyAvailability.isAvailable()
            _uiState.update { it.copy(passkeyAvailable = available) }
        }
    }

    /**
     * Issue #722 follow-up: show the biometric login action only when the active
     * profile still has a stored grant for the typed server and this device can
     * complete the OS check. Debounced with the server URL for the same reason.
     */
    private fun refreshBiometricAvailability(debounce: Boolean = false) {
        biometricAvailabilityJob?.cancel()
        biometricAvailabilityJob = viewModelScope.launch {
            if (debounce) delay(PASSKEY_AVAILABILITY_DEBOUNCE_MS)
            val url = _uiState.value.serverUrl.trim().trimEnd('/')
            val available = url.isNotBlank() &&
                localAuthCapabilities.canEnableLocalAuth() &&
                deviceGrantRepository.hasStoredGrantFor(url)
            _uiState.update { it.copy(biometricAvailable = available) }
        }
    }

    fun onServerUrlChange(value: String) {
        _uiState.update { it.copy(serverUrl = value) }
        // Persist immediately (not just on submit): the register and
        // forgot-password flows are reached from this screen and read the
        // server URL from the session manager (M26).
        viewModelScope.launch { sessionManager.setServerUrl(value.trim().trimEnd('/')) }
        refreshPasskeyAvailability(debounce = true)
        refreshBiometricAvailability(debounce = true)
    }

    fun onModeChange(mode: LoginMode) {
        _uiState.update { it.copy(mode = mode, errorRes = null, error = null) }
    }

    /**
     * Authenticate with the given credentials. The values are deliberately
     * passed in rather than stored in [LoginUiState] — a password or API token
     * must not linger in ViewModel state after the attempt (security).
     */
    fun onSubmit(serverUrl: String, identifier: String, password: String, apiToken: String) {
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
                                result.methods.includesPasskey() &&
                                    passkeyAvailability.isAvailable()
                            ),
                            passkeyIssue = null,
                            errorRes = null,
                            error = null
                        )
                    }
                }

                is LoginUseCase.Result.Failure -> {
                    _uiState.update { it.failedWith(result.cause) }
                }

                is LoginWithApiTokenUseCase.Result.Success -> {
                    _uiState.update { it.copy(isLoading = false) }
                    _events.send(LoginEvent.LoggedIn)
                }

                is LoginWithApiTokenUseCase.Result.Failure -> {
                    _uiState.update { it.failedWith(result.cause) }
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
                error = null
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
                }
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
        if (current.isLoading || !current.twoFactorStep ||
            !current.twoFactorPrompt.offersPasskey
        ) {
            return
        }
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
                    onFailure = { error -> handlePasskeyFailure(error, fromBegin = false) }
                )

                PasskeyResult.Cancelled -> _uiState.update { it.copy(isLoading = false) }

                PasskeyResult.NoMatchingPasskey ->
                    _uiState.update {
                        it.copy(
                            isLoading = false,
                            errorRes = R.string.login_error_passkey_no_match
                        )
                    }

                PasskeyResult.NotAssociated -> degradePasskey(PasskeyIssue.NOT_ASSOCIATED)

                PasskeyResult.NoProvider -> degradePasskey(PasskeyIssue.NO_PROVIDER)

                PasskeyResult.AlreadyRegistered, is PasskeyResult.Failed ->
                    _uiState.update {
                        it.copy(
                            isLoading = false,
                            errorRes = R.string.login_error_passkey_failed
                        )
                    }
            }
        }
    }

    /**
     * Discussion #2355: passwordless passkey sign-in from the credentials step.
     * No username, password or 2FA challenge is involved. The server gate and
     * device support are re-checked here so a button shown for a previously-used
     * server cannot prompt against one that has Android passkeys disabled.
     *
     * Outcomes: success emits [LoginEvent.LoggedIn]; a dismissed prompt is
     * silent; no matching credential, provider/association failure and a
     * rejected assertion all stay on the credentials form with copy that points
     * at the password field; a lockout surfaces the server text.
     */
    fun onUsePasswordlessPasskey(context: Context) {
        if (_uiState.value.isLoading) return
        val trimmedUrl = _uiState.value.serverUrl.trim().trimEnd('/')
        if (!Validators.isValidServerUrl(trimmedUrl)) {
            _uiState.update {
                it.copy(errorRes = R.string.login_error_valid_server_url, error = null)
            }
            return
        }
        _uiState.update { it.copy(isLoading = true, errorRes = null, error = null) }
        viewModelScope.launch {
            sessionManager.setServerUrl(trimmedUrl)
            _events.send(LoginEvent.ServerUrlUpdated)

            if (!passkeyAvailability.isAvailable()) {
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        passkeyAvailable = false,
                        errorRes = R.string.login_error_passkey_unavailable_password
                    )
                }
                return@launch
            }

            val challenge = authRepository.beginPasswordlessPasskeyLogin().getOrElse { error ->
                handlePasswordlessFailure(error, fromBegin = true)
                return@launch
            }
            when (val ceremony = passkeyClient.getPasskey(context, challenge.optionsJson)) {
                is PasskeyResult.Success -> {
                    completePasswordlessLogin(challenge, ceremony.json)
                }

                PasskeyResult.Cancelled -> _uiState.update { it.copy(isLoading = false) }

                PasskeyResult.NoMatchingPasskey ->
                    _uiState.update {
                        it.copy(
                            isLoading = false,
                            errorRes = R.string.login_error_passkey_no_match_password
                        )
                    }

                PasskeyResult.NotAssociated, PasskeyResult.NoProvider ->
                    _uiState.update {
                        it.copy(
                            isLoading = false,
                            passkeyAvailable = false,
                            errorRes = R.string.login_error_passkey_unavailable_password
                        )
                    }

                PasskeyResult.AlreadyRegistered, is PasskeyResult.Failed ->
                    _uiState.update {
                        it.copy(
                            isLoading = false,
                            errorRes = R.string.login_error_passkey_failed_password
                        )
                    }
            }
        }
    }

    private suspend fun completePasswordlessLogin(
        challenge: PasswordlessPasskeyChallenge,
        assertionJson: String
    ) {
        authRepository.completePasswordlessPasskeyLogin(challenge, assertionJson).fold(
            onSuccess = {
                _uiState.update { it.copy(isLoading = false) }
                _events.send(LoginEvent.LoggedIn)
            },
            onFailure = { error -> handlePasswordlessFailure(error, fromBegin = false) }
        )
    }

    private fun handlePasswordlessFailure(error: Throwable, fromBegin: Boolean) {
        val apiError = error.toApiError()
        when {
            apiError is ApiError.Client && apiError.code == 429 ->
                _uiState.update { it.copy(isLoading = false, error = apiError) }

            fromBegin && apiError is ApiError.Client && apiError.code in setOf(401, 409) ->
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        passkeyAvailable = false,
                        errorRes = R.string.login_error_passkey_unavailable_password
                    )
                }

            else -> _uiState.update {
                it.copy(
                    isLoading = false,
                    errorRes = R.string.login_error_passkey_failed_password
                )
            }
        }
    }

    /**
     * Issue #722 follow-up: use the enrolled device grant as a true login-screen
     * re-auth. The OS biometric/device-credential prompt runs first; only a
     * SUCCESS reads the stored grant and exchanges it for a session. A dismissed
     * prompt is silent; revoked/gone grants hide the action and point at the
     * password/passkey alternatives.
     */
    fun onUseBiometric(context: Context) {
        if (_uiState.value.isLoading) return
        val trimmedUrl = _uiState.value.serverUrl.trim().trimEnd('/')
        if (!Validators.isValidServerUrl(trimmedUrl)) {
            _uiState.update {
                it.copy(errorRes = R.string.login_error_valid_server_url, error = null)
            }
            return
        }
        val activity = context as? Activity ?: return
        _uiState.update { it.copy(isLoading = true, errorRes = null, error = null) }
        viewModelScope.launch {
            sessionManager.setServerUrl(trimmedUrl)
            _events.send(LoginEvent.ServerUrlUpdated)

            if (!localAuthCapabilities.canEnableLocalAuth() ||
                !deviceGrantRepository.hasStoredGrantFor(trimmedUrl)
            ) {
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        biometricAvailable = false,
                        errorRes = R.string.login_error_biometric_unavailable
                    )
                }
                return@launch
            }

            val prompt = localAuthPrompt.requestUnlock(
                activity = activity,
                title = context.getString(R.string.login_biometric_prompt_title),
                subtitle = context.getString(R.string.login_biometric_prompt_subtitle),
                negativeButtonText = context.getString(R.string.login_biometric_prompt_cancel)
            )
            when (prompt) {
                LocalAuthOutcome.CANCELLED -> _uiState.update { it.copy(isLoading = false) }

                LocalAuthOutcome.NOT_AVAILABLE -> _uiState.update {
                    it.copy(
                        isLoading = false,
                        biometricAvailable = false,
                        errorRes = R.string.login_error_biometric_unavailable
                    )
                }

                LocalAuthOutcome.ERROR -> _uiState.update {
                    it.copy(
                        isLoading = false,
                        errorRes = R.string.login_error_biometric_failed
                    )
                }

                LocalAuthOutcome.SUCCESS -> completeBiometricLogin(trimmedUrl)
            }
        }
    }

    private suspend fun completeBiometricLogin(serverUrl: String) {
        val grant = deviceGrantRepository.storedGrantToken()
        if (grant.isNullOrBlank() || !deviceGrantRepository.hasStoredGrantFor(serverUrl)) {
            _uiState.update {
                it.copy(
                    isLoading = false,
                    biometricAvailable = false,
                    errorRes = R.string.login_error_biometric_unavailable
                )
            }
            return
        }
        authRepository.loginWithDeviceGrant(grant).fold(
            onSuccess = {
                _uiState.update { it.copy(isLoading = false) }
                _events.send(LoginEvent.LoggedIn)
            },
            onFailure = { error ->
                val apiError = error.toApiError()
                when {
                    apiError is ApiError.Client && apiError.code == 429 ->
                        _uiState.update { it.copy(isLoading = false, error = apiError) }

                    apiError is ApiError.Client && apiError.code in setOf(401, 403) ->
                        _uiState.update {
                            it.copy(
                                isLoading = false,
                                biometricAvailable = false,
                                errorRes = R.string.login_error_biometric_unavailable
                            )
                        }

                    else -> _uiState.update {
                        it.copy(
                            isLoading = false,
                            errorRes = R.string.login_error_biometric_failed
                        )
                    }
                }
            }
        )
    }

    private companion object {
        /** Long enough to swallow a typed URL, short enough to feel immediate. */
        const val PASSKEY_AVAILABILITY_DEBOUNCE_MS = 500L
    }

    /** Drops the step to the code-only prompt (gate closed) and remembers why, for the copy. */
    private fun degradePasskey(issue: PasskeyIssue) {
        _uiState.update {
            it.copy(
                isLoading = false,
                twoFactorPrompt = secondFactorPrompt(it.twoFactorMethods, passkeyAvailable = false),
                passkeyIssue = issue
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
                _uiState.update { it.copy(isLoading = false, error = apiError) }

            else -> _uiState.update {
                it.copy(
                    isLoading = false,
                    errorRes = R.string.login_error_passkey_failed
                )
            }
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
            passkeyIssue = _uiState.value.passkeyIssue
        )
        _uiState.value = when {
            apiError is ApiError.Client && apiError.code == 401 ->
                // The 10-minute challenge expired (or was already consumed /
                // 2FA was disabled) — a retry here can never succeed.
                base.copy(twoFactorStep = false, errorRes = R.string.login_error_two_factor_expired)

            apiError is ApiError.Client && apiError.code == 429 ->
                // Lockout: surface the server's message and stop (web parity).
                base.copy(error = apiError)

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
