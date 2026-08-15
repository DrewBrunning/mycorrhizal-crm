package com.mycorrhizal.crm.feature.settings

import android.content.Context
import android.content.Intent
import androidx.annotation.StringRes
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.AppSettingsRepository
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.SessionState
import com.mycorrhizal.crm.domain.repository.TrackingSettingsRepository
import com.mycorrhizal.crm.ui.R
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
    val themePreference: String = AppSettingsRepository.THEME_SYSTEM,
    val isChangingPassword: Boolean = false,
    /** Static password validation error as a string resource id, resolved in the UI (mirrors LoginViewModel). */
    @StringRes val passwordErrorRes: Int? = null,
    /** Dynamic password-change error text (server message). */
    val passwordError: String? = null,
)

sealed interface SettingsEvent {
    data object LoggedOut : SettingsEvent

    /** A language change was persisted — the Activity must recreate to re-resolve resources. */
    data object LocaleChanged : SettingsEvent

    /** Password change succeeded; the server invalidated every JWT, so the user must re-login. */
    data object PasswordChanged : SettingsEvent
}

@HiltViewModel
class SettingsViewModel @Inject constructor(
    private val authRepository: AuthRepository,
    private val trackingSettings: TrackingSettingsRepository,
    private val appSettings: AppSettingsRepository,
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
        viewModelScope.launch {
            appSettings.themePreference().collect { pref ->
                _uiState.update { it.copy(themePreference = pref) }
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

    // --- M25: profile & channels ---

    /**
     * Persist the language server-side and locally, then request a locale
     * change. The activity recreates on [SettingsEvent.LocaleChanged] so the
     * `values-XX` resources resolve in the new language without a restart.
     */
    fun updateLanguage(language: String) {
        val current = _uiState.value.session.language
        if (language == current) return
        viewModelScope.launch {
            authRepository.updateLanguage(language).onSuccess {
                appSettings.setLanguageOverride(language)
                _events.value = SettingsEvent.LocaleChanged
            }
        }
    }

    /** Persist the date-format preference server-side; SessionState re-emits so screens update live. */
    fun updateDateFormat(dateFormat: String) {
        val current = _uiState.value.session.dateFormat
        if (dateFormat == current) return
        viewModelScope.launch {
            authRepository.updateDateFormat(dateFormat)
        }
    }

    fun setThemePreference(preference: String) {
        viewModelScope.launch {
            appSettings.setThemePreference(preference)
        }
    }

    /**
     * Change the password. [newPassword] must equal [confirmPassword]; the
     * mismatch is a UI validation error, never sent to the server. On success
     * the server invalidates every JWT, so the session is cleared and
     * [SettingsEvent.PasswordChanged] asks the user to sign in again. Passwords
     * never live in [SettingsUiState].
     */
    fun changePassword(currentPassword: String, newPassword: String, confirmPassword: String) {
        if (_uiState.value.isChangingPassword) return
        if (newPassword != confirmPassword) {
            _uiState.update { it.copy(passwordErrorRes = R.string.settings_password_mismatch, passwordError = null) }
            return
        }
        _uiState.update { it.copy(isChangingPassword = true, passwordErrorRes = null, passwordError = null) }
        viewModelScope.launch {
            authRepository.changePassword(currentPassword, newPassword)
                .onSuccess {
                    _uiState.update { it.copy(isChangingPassword = false) }
                    // Every JWT was invalidated server-side; the web re-issues a
                    // cookie, a bearer-token client cannot. Force a clean re-login.
                    authRepository.logout()
                    _events.value = SettingsEvent.PasswordChanged
                }
                .onFailure { error ->
                    _uiState.update {
                        it.copy(isChangingPassword = false, passwordError = error.displayMessage())
                    }
                }
        }
    }

    private fun Throwable.displayMessage(): String =
        (this as? com.mycorrhizal.crm.network.ApiError)?.displayMessage ?: message ?: "error"

    private fun startCallDetectionService() {
        // minSdk is 26, so startForegroundService is always the correct path
        // (the pre-O branch could never be taken — M5 §7 cleanup).
        appContext.startForegroundService(
            Intent(appContext, com.mycorrhizal.crm.feature.tracking.CallDetectionService::class.java),
        )
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

    /** Clear the one-shot event so the screen's LaunchedEffect doesn't re-fire it. */
    fun onEventShown() {
        _events.value = null
    }
}
