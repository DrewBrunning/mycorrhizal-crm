package com.mycorrhizal.crm.feature.auth

import androidx.annotation.StringRes
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.model.util.Validators
import com.mycorrhizal.crm.network.toApiError
import com.mycorrhizal.crm.ui.R
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

/** Mirrors web's ForgotPasswordDialog ResetStep = 'request' | 'confirm' | 'done'. */
enum class PasswordResetStep { REQUEST, CONFIRM, DONE }

data class ForgotPasswordUiState(
    val serverUrl: String = "",
    val step: PasswordResetStep = PasswordResetStep.REQUEST,
    val email: String = "",
    val token: String = "",
    val newPassword: String = "",
    val confirmPassword: String = "",
    val isLoading: Boolean = false,
    val error: String? = null,
    @StringRes val errorRes: Int? = null,
    /** The anti-enumeration request message (identical for known and unknown emails). */
    val requestMessage: String? = null,
)

@HiltViewModel
class ForgotPasswordViewModel @Inject constructor(
    private val authRepository: AuthRepository,
    private val sessionManager: SessionManager,
) : ViewModel() {

    private val _uiState = MutableStateFlow(ForgotPasswordUiState())
    val uiState: StateFlow<ForgotPasswordUiState> = _uiState.asStateFlow()

    init {
        viewModelScope.launch {
            _uiState.update { it.copy(serverUrl = sessionManager.serverUrl().orEmpty()) }
        }
    }

    fun onServerUrlChange(value: String) {
        _uiState.update { it.copy(serverUrl = value) }
        viewModelScope.launch { sessionManager.setServerUrl(value.trim().trimEnd('/')) }
    }

    fun onEmailChange(value: String) {
        _uiState.update { it.copy(email = value, error = null, errorRes = null) }
    }

    fun onTokenChange(value: String) {
        _uiState.update { it.copy(token = value, error = null, errorRes = null) }
    }

    fun onNewPasswordChange(value: String) {
        _uiState.update { it.copy(newPassword = value, error = null, errorRes = null) }
    }

    fun onConfirmPasswordChange(value: String) {
        _uiState.update { it.copy(confirmPassword = value, error = null, errorRes = null) }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null, errorRes = null) }
    }

    /** Step 1: POST /password-reset/request — always the same anti-enumeration message. */
    fun requestReset() {
        val state = _uiState.value
        if (state.isLoading) return
        val trimmedUrl = state.serverUrl.trim().trimEnd('/')
        if (!Validators.isValidServerUrl(trimmedUrl)) {
            _uiState.update { it.copy(errorRes = R.string.login_error_valid_server_url, error = null) }
            return
        }
        if (state.email.isBlank()) {
            _uiState.update { it.copy(errorRes = R.string.forgot_password_error_email_required, error = null) }
            return
        }
        _uiState.update { it.copy(isLoading = true, error = null, errorRes = null, requestMessage = null) }
        viewModelScope.launch {
            sessionManager.setServerUrl(trimmedUrl)
            authRepository.requestPasswordReset(state.email.trim()).fold(
                onSuccess = { message ->
                    _uiState.update {
                        it.copy(
                            isLoading = false,
                            step = PasswordResetStep.CONFIRM,
                            requestMessage = message ?: state.email,
                        )
                    }
                },
                onFailure = { e ->
                    _uiState.update { it.copy(isLoading = false, error = e.toApiError().displayMessage) }
                },
            )
        }
    }

    /** Step 2: POST /password-reset/confirm with the emailed token. */
    fun confirmReset() {
        val state = _uiState.value
        if (state.isLoading) return
        if (state.token.isBlank()) {
            _uiState.update { it.copy(errorRes = R.string.forgot_password_error_token_required, error = null) }
            return
        }
        if (state.newPassword.isBlank()) {
            _uiState.update { it.copy(errorRes = R.string.forgot_password_error_password_required, error = null) }
            return
        }
        if (state.newPassword != state.confirmPassword) {
            _uiState.update { it.copy(errorRes = R.string.settings_password_mismatch, error = null) }
            return
        }
        _uiState.update { it.copy(isLoading = true, error = null, errorRes = null) }
        viewModelScope.launch {
            authRepository.confirmPasswordReset(state.token.trim(), state.newPassword).fold(
                onSuccess = {
                    _uiState.update { it.copy(isLoading = false, step = PasswordResetStep.DONE) }
                },
                onFailure = { e ->
                    _uiState.update { it.copy(isLoading = false, error = e.toApiError().displayMessage) }
                },
            )
        }
    }

    fun onDone() {
        _uiState.update {
            it.copy(step = PasswordResetStep.REQUEST, email = "", token = "", newPassword = "", confirmPassword = "")
        }
    }
}
