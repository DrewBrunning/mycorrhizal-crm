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

data class RegisterUiState(
    val serverUrl: String = "",
    val username: String = "",
    val email: String = "",
    val password: String = "",
    val isLoading: Boolean = false,
    val error: String? = null,
    @StringRes val errorRes: Int? = null,
    // M26: the server's password-strength verdict, surfaced before submit.
    val passwordStrength: PasswordStrength? = null,
    val passwordChecked: Boolean = false,
    val checkingStrength: Boolean = false,
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
            _uiState.update { it.copy(serverUrl = sessionManager.serverUrl().orEmpty()) }
        }
    }

    fun onServerUrlChange(value: String) {
        _uiState.update { it.copy(serverUrl = value) }
        // Persist so the register/reset flows and the eventual login share it.
        viewModelScope.launch { sessionManager.setServerUrl(value.trim().trimEnd('/')) }
    }

    fun onUsernameChange(value: String) {
        _uiState.update { it.copy(username = value, error = null, errorRes = null) }
    }

    fun onEmailChange(value: String) {
        _uiState.update { it.copy(email = value, error = null, errorRes = null) }
    }

    /**
     * Debounced server-side strength check on the password field (M26's test
     * case 1: the response is surfaced BEFORE submission, and a weak password
     * blocks submit rather than failing server-side). The check only fires for
     * a non-blank password; failures are silent (the register submit still
     * carries the real validation).
     */
    fun onPasswordChange(value: String) {
        _uiState.update {
            it.copy(password = value, error = null, errorRes = null, passwordStrength = null, passwordChecked = false)
        }
        strengthJob?.cancel()
        if (value.isBlank()) return
        strengthJob = viewModelScope.launch {
            delay(STRENGTH_DEBOUNCE_MS)
            _uiState.update { it.copy(checkingStrength = true) }
            authRepository.checkPasswordStrength(value).fold(
                onSuccess = { strength ->
                    _uiState.update {
                        it.copy(passwordStrength = strength, passwordChecked = true, checkingStrength = false)
                    }
                },
                onFailure = {
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
     */
    fun submit() {
        val state = _uiState.value
        if (state.isLoading) return

        val trimmedUrl = state.serverUrl.trim().trimEnd('/')
        if (!Validators.isValidServerUrl(trimmedUrl)) {
            _uiState.update { it.copy(errorRes = R.string.login_error_valid_server_url, error = null) }
            return
        }
        val validationError = when {
            state.username.isBlank() -> R.string.register_error_username_required
            state.email.isBlank() -> R.string.register_error_email_required
            state.password.isBlank() -> R.string.register_error_password_required
            else -> null
        }
        if (validationError != null) {
            _uiState.update { it.copy(errorRes = validationError, error = null) }
            return
        }
        // A known-weak password blocks submit — the strength response is the
        // pre-submission gate (test case 1).
        if (state.passwordChecked && state.passwordStrength?.isValid == false) {
            val feedback = state.passwordStrength?.feedback
            if (feedback.isNullOrBlank()) {
                _uiState.update { it.copy(errorRes = R.string.register_error_password_weak, error = null) }
            } else {
                _uiState.update { it.copy(error = feedback, errorRes = null) }
            }
            return
        }

        _uiState.update { it.copy(isLoading = true, error = null, errorRes = null) }
        viewModelScope.launch {
            sessionManager.setServerUrl(trimmedUrl)
            val registered = authRepository.register(state.username.trim(), state.email.trim(), state.password)
            if (registered.isFailure) {
                _uiState.update {
                    it.copy(isLoading = false, error = registered.exceptionOrNull()?.toApiError()?.registerMessage())
                }
                return@launch
            }
            val login = authRepository.login(state.email.trim(), state.password)
            if (login.isFailure) {
                // Registration succeeded but the follow-up login failed (e.g.
                // server hiccup). Tell the user to sign in manually rather than
                // pretending the flow is done.
                _uiState.update { it.copy(isLoading = false, error = login.exceptionOrNull()?.toApiError()?.registerMessage()) }
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
