package com.mycorrhizal.crm.feature.settings

import android.content.Context
import android.content.Intent
import android.os.Build
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.SessionState
import com.mycorrhizal.crm.domain.repository.TrackingSettingsRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class SettingsUiState(
    val session: SessionState = SessionState(),
    val isLoggingOut: Boolean = false,
    val callTrackingEnabled: Boolean = false,
    val smsTrackingEnabled: Boolean = false,
    val notificationsEnabled: Boolean = true,
)

sealed interface SettingsEvent {
    data object LoggedOut : SettingsEvent
}

@HiltViewModel
class SettingsViewModel @Inject constructor(
    private val authRepository: AuthRepository,
    private val trackingSettings: TrackingSettingsRepository,
    @ApplicationContext private val appContext: Context,
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
        viewModelScope.launch {
            _uiState.update {
                it.copy(
                    callTrackingEnabled = trackingSettings.callTrackingEnabled(),
                    smsTrackingEnabled = trackingSettings.smsTrackingEnabled(),
                    notificationsEnabled = trackingSettings.notificationsEnabled(),
                )
            }
        }
    }

    fun setCallTrackingEnabled(enabled: Boolean) {
        _uiState.update { it.copy(callTrackingEnabled = enabled) }
        viewModelScope.launch {
            trackingSettings.setCallTrackingEnabled(enabled)
            if (enabled) {
                startCallDetectionService()
            } else {
                stopCallDetectionService()
            }
        }
    }

    fun setSmsTrackingEnabled(enabled: Boolean) {
        _uiState.update { it.copy(smsTrackingEnabled = enabled) }
        viewModelScope.launch { trackingSettings.setSmsTrackingEnabled(enabled) }
    }

    fun setNotificationsEnabled(enabled: Boolean) {
        _uiState.update { it.copy(notificationsEnabled = enabled) }
        viewModelScope.launch { trackingSettings.setNotificationsEnabled(enabled) }
    }

    private fun startCallDetectionService() {
        val intent = Intent(appContext, com.mycorrhizal.crm.feature.tracking.CallDetectionService::class.java)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            appContext.startForegroundService(intent)
        } else {
            appContext.startService(intent)
        }
    }

    private fun stopCallDetectionService() {
        appContext.stopService(
            Intent(appContext, com.mycorrhizal.crm.feature.tracking.CallDetectionService::class.java),
        )
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
