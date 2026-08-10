package com.mycorrhizal.crm.feature.settings

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.SessionState
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class SettingsUiState(
    val session: SessionState = SessionState(),
    val isLoggingOut: Boolean = false,
)

sealed interface SettingsEvent {
    data object LoggedOut : SettingsEvent
}

@HiltViewModel
class SettingsViewModel @Inject constructor(
    private val authRepository: AuthRepository,
) : ViewModel() {

    private val _uiState = MutableStateFlow(SettingsUiState())
    val uiState: StateFlow<SettingsUiState> = _uiState.asStateFlow()

    private val _events = MutableStateFlow<SettingsEvent?>(null)
    val events: StateFlow<SettingsEvent?> = _events

    init {
        viewModelScope.launch {
            authRepository.observeSession().collect { session ->
                _uiState.update { it.copy(session = session) }
            }
        }
    }

    fun logout() {
        if (_uiState.value.isLoggingOut) return
        _uiState.update { it.copy(isLoggingOut = true) }
        viewModelScope.launch {
            authRepository.logout()
            _events.value = SettingsEvent.LoggedOut
        }
    }

    fun onLoggedOutShown() {
        _events.value = null
    }
}
