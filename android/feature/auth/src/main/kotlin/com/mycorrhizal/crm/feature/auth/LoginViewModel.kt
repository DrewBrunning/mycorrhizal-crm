package com.mycorrhizal.crm.feature.auth

import androidx.annotation.StringRes
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.domain.usecase.LoginUseCase
import com.mycorrhizal.crm.domain.usecase.LoginWithApiTokenUseCase
import com.mycorrhizal.crm.model.util.Validators
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
) : ViewModel() {

    private val _uiState = MutableStateFlow(LoginUiState())
    val uiState: StateFlow<LoginUiState> = _uiState.asStateFlow()

    private val _events = Channel<LoginEvent>(Channel.BUFFERED)
    val events = _events.receiveAsFlow()

    fun onServerUrlChange(value: String) {
        _uiState.update { it.copy(serverUrl = value) }
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

    fun onErrorShown() {
        _uiState.update { it.copy(errorRes = null, error = null) }
    }
}
