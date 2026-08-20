package com.mycorrhizal.crm.feature.auth

import androidx.annotation.StringRes
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.model.network.PasswordStrength
import com.mycorrhizal.crm.model.util.Validators
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.toApiError
import com.mycorrhizal.crm.ui.R
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.Job
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

/**
 * Registration state. Deliberately carries NO credentials (username/email/
 * password): the screen holds them in local `remember` state and passes them
 * up on submit, matching LoginViewModel's tested convention — a password must
 * not linger in ViewModel state. Only the server's strength VERDICT (not the
 * password) is kept, for the pre-submit gate.
 */
data class RegisterUiState(
    val serverUrl: String = "",
    val isLoading: Boolean = false,
    val error: String? = null,
    @StringRes val errorRes: Int? = null,
    // M26: the server's password-strength verdict, surfaced before submit.
    val passwordStrength: PasswordStrength? = null,
    val passwordChecked: Boolean = false,
    val checkingStrength: Boolean = false,
    // Android testing feedback: DISABLE_REGISTRATION was only ever enforced
    // by the eventual 403 on submit — this flag lets RegisterScreen show a
    // disabled notice up front instead. Best-effort: a failed fetch leaves
    // this false (the form shows, exactly as before this existed) — the
    // server's 403 is still the real enforcement either way.
    val registrationDisabled: Boolean = false,
)

sealed interface RegisterEvent {
    data object Registered : RegisterEvent
}

@HiltViewModel
class RegisterViewModel @Inject constructor(
    private val authRepository: AuthRepository,
    private val sessionManager: SessionManager,
) : ViewModel() {

    private val _uiState = MutableStateFlow(RegisterUiState())
    val uiState: StateFlow<RegisterUiState> = _uiState.asStateFlow()

    private val _events = Channel<RegisterEvent>(Channel.BUFFERED)
    val events = _events.receiveAsFlow()

    private var strengthJob: Job? = null

    init {
        // Pre-fill the server URL the login screen already captured (the
        // register flow is always reached from it).
        viewModelScope.launch {
            val serverUrl = sessionManager.serverUrl().orEmpty()
            _uiState.update { it.copy(serverUrl = serverUrl) }
            if (serverUrl.isNotBlank()) checkRegistrationAvailability()
        }
    }

    /**
     * Best-effort: checked once against the server URL captured at screen
     * entry. A user who edits the server URL on this screen before
     * submitting is not re-checked — the submit's own 403 is the backstop
     * either way, so a stale check only ever costs an extra tap, never a
     * silently-created account.
     */
    private fun checkRegistrationAvailability() {
        viewModelScope.launch {
            authRepository.getAuthConfig().onSuccess { config ->
                _uiState.update { it.copy(registrationDisabled = config.registrationDisabled) }
            }
        }
    }

    fun onServerUrlChange(value: String) {
        _uiState.update { it.copy(serverUrl = value) }
        // Persist so the register/reset flows and the eventual login share it.
        viewModelScope.launch { sessionManager.setServerUrl(value.trim().trimEnd('/')) }
    }

    /**
     * Debounced server-side strength check on the password field (M26's test
     * case 1: the response is surfaced BEFORE submission, and a weak password
     * blocks submit rather than failing server-side). [value] is transient —
     * it is used for the check and discarded, never stored in state. The
     * checking flag is set IMMEDIATELY (before the debounce) so a submit can
     * never race past an in-flight check (review-pass fix). Failures are
     * silent (the register submit still carries the real validation).
     */
    fun onPasswordChange(value: String) {
        _uiState.update {
            it.copy(
                error = null,
                errorRes = null,
                passwordStrength = null,
                passwordChecked = false,
                checkingStrength = value.isNotBlank(),
            )
        }
        strengthJob?.cancel()
        if (value.isBlank()) return
        strengthJob = viewModelScope.launch {
            delay(STRENGTH_DEBOUNCE_MS)
            authRepository.checkPasswordStrength(value).fold(
                onSuccess = { strength ->
                    _uiState.update {
                        it.copy(passwordStrength = strength, passwordChecked = true, checkingStrength = false)
                    }
                },
                onFailure = {
                    // The check is best-effort; the server is the backstop.
                    _uiState.update { it.copy(checkingStrength = false) }
                },
            )
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null, errorRes = null) }
    }

    /**
     * Creates the account and auto-logs-in (the ticket's test case 3: register
     * itself sets no session — the web redirects to /login — so Android
     * exchanges the same credentials for a JWT instead of asking twice).
     * [username], [email] and [password] come from the screen's local state.
     */
    fun submit(username: String, email: String, password: String) {
        val state = _uiState.value
        if (state.isLoading) return

        val trimmedUrl = state.serverUrl.trim().trimEnd('/')
        if (!Validators.isValidServerUrl(trimmedUrl)) {
            _uiState.update { it.copy(errorRes = R.string.login_error_valid_server_url, error = null) }
            return
        }
        val validationError = when {
            username.isBlank() -> R.string.register_error_username_required
            email.isBlank() -> R.string.register_error_email_required
            password.isBlank() -> R.string.register_error_password_required
            else -> null
        }
        if (validationError != null) {
            _uiState.update { it.copy(errorRes = validationError, error = null) }
            return
        }
        // The pre-submit strength gate (test case 1): a known-weak password
        // blocks submit, and an in-flight check also blocks (a verdict that
        // hasn't landed yet must not be raced past — review-pass fix).
        if (state.checkingStrength) {
            _uiState.update { it.copy(errorRes = R.string.register_checking_strength, error = null) }
            return
        }
        if (state.passwordChecked && state.passwordStrength?.isValid == false) {
            val feedback = state.passwordStrength?.feedback
            if (feedback.isNullOrBlank()) {
                _uiState.update { it.copy(errorRes = R.string.register_error_password_weak, error = null) }
            } else {
                _uiState.update { it.copy(error = feedback, errorRes = null) }
            }
            return
        }

        val trimmedUsername = username.trim()
        val trimmedEmail = email.trim()
        _uiState.update { it.copy(isLoading = true, error = null, errorRes = null) }
        viewModelScope.launch {
            sessionManager.setServerUrl(trimmedUrl)
            val registered = authRepository.register(trimmedUsername, trimmedEmail, password)
            if (registered.isFailure) {
                _uiState.update {
                    it.copy(isLoading = false, error = registered.exceptionOrNull()?.toApiError()?.registerMessage())
                }
                return@launch
            }
            val login = authRepository.login(trimmedEmail, password)
            if (login.isFailure) {
                // Registration succeeded but the follow-up login failed (e.g.
                // server hiccup). The account exists — say so, and point the
                // user at the sign-in screen rather than a confusing 409 on
                // retry (review-pass fix).
                _uiState.update {
                    it.copy(isLoading = false, errorRes = R.string.register_created_login_failed, error = null)
                }
                return@launch
            }
            _uiState.update { it.copy(isLoading = false) }
            _events.send(RegisterEvent.Registered)
        }
    }

    private fun ApiError.registerMessage(): String = when (this) {
        is ApiError.Client -> body
        else -> displayMessage
    }

    private companion object {
        const val STRENGTH_DEBOUNCE_MS = 300L
    }
}
