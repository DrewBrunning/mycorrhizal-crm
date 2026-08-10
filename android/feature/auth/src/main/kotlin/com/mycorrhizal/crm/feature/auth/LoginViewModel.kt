package com.mycorrhizal.crm.feature.auth

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.domain.usecase.LoginUseCase
import com.mycorrhizal.crm.domain.usecase.LoginWithApiTokenUseCase
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
    val identifier: String = "",
    val password: String = "",
    val apiToken: String = "",
    val mode: LoginMode = LoginMode.PASSWORD,
    val isLoading: Boolean = false,
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

    fun onIdentifierChange(value: String) {
        _uiState.update { it.copy(identifier = value) }
    }

    fun onPasswordChange(value: String) {
        _uiState.update { it.copy(password = value) }
    }

    fun onApiTokenChange(value: String) {
        _uiState.update { it.copy(apiToken = value) }
    }

    fun onModeChange(mode: LoginMode) {
        _uiState.update { it.copy(mode = mode, error = null) }
    }

    fun onSubmit() {
        val state = _uiState.value
        if (state.isLoading) return

        val serverUrl = state.serverUrl.trim().trimEnd('/')
        if (serverUrl.isBlank()) {
            _uiState.update { it.copy(error = "Server URL is required") }
            return
        }

        _uiState.update { it.copy(isLoading = true, error = null) }
        viewModelScope.launch {
            sessionManager.setServerUrl(serverUrl)
            _events.send(LoginEvent.ServerUrlUpdated)

            val result = when (state.mode) {
                LoginMode.PASSWORD -> loginUseCase(state.identifier, state.password)
                LoginMode.API_TOKEN -> loginWithApiTokenUseCase(state.apiToken)
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
        _uiState.update { it.copy(error = null) }
    }
}
