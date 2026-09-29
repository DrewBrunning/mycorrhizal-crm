package com.mycorrhizal.crm.feature.settings

import androidx.annotation.StringRes
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.data.local.LocalServerHost
import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.data.session.SwitchProfileResult
import com.mycorrhizal.crm.domain.profile.ServerProfile
import com.mycorrhizal.crm.domain.repository.BundleBackupRepository
import com.mycorrhizal.crm.model.util.Validators
import com.mycorrhizal.crm.ui.R
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

/** A switch blocked on the user agreeing to discard unsynced interactions. */
data class PendingSwitch(val profileId: String, val count: Int)

data class ServersUiState(
    val profiles: List<ServerProfile> = emptyList(),
    val activeProfileId: String? = null,
    val isBusy: Boolean = false,
    @StringRes val errorRes: Int? = null,
    val pendingSwitch: PendingSwitch? = null,
)

sealed interface ServersEvent {
    /** The active profile changed; the caller can leave the screen. */
    data object Switched : ServersEvent

    /** A profile was removed; the caller can leave the screen. */
    data object Removed : ServersEvent
}

/**
 * ADR 0028 Decision 1: the Settings "Servers" surface. Lists, adds, renames,
 * switches and removes server profiles. All the state and switching logic lives
 * in [SessionManager]; this is a thin, testable adapter.
 */
@HiltViewModel
class ServersViewModel @Inject constructor(
    private val sessionManager: SessionManager,
    private val localServerHost: LocalServerHost,
    private val bundleBackupRepository: BundleBackupRepository,
) : ViewModel() {

    private val _uiState = MutableStateFlow(ServersUiState())
    val uiState: StateFlow<ServersUiState> = _uiState.asStateFlow()

    private val _events = Channel<ServersEvent>(Channel.BUFFERED)
    val events = _events.receiveAsFlow()

    init {
        viewModelScope.launch {
            combine(
                sessionManager.observeProfiles(),
                sessionManager.observeActiveProfile(),
            ) { profiles, active -> profiles to active?.id }
                .collect { (profiles, activeId) ->
                    _uiState.update { it.copy(profiles = profiles, activeProfileId = activeId) }
                }
        }
    }

    /** Switch to [profileId]. May surface [ServersUiState.pendingSwitch]. */
    fun select(profileId: String) {
        viewModelScope.launch { activate(profileId, discardPending = false) }
    }

    /** The user agreed to discard the unsynced interactions named in the dialog. */
    fun confirmDiscard() {
        val pending = _uiState.value.pendingSwitch ?: return
        viewModelScope.launch {
            _uiState.update { it.copy(pendingSwitch = null) }
            activate(pending.profileId, discardPending = true)
        }
    }

    fun dismissPendingSwitch() {
        _uiState.update { it.copy(pendingSwitch = null) }
    }

    /**
     * Add a Remote profile and make it active. A fresh profile has no token, so
     * the switch lands the app on the Auth screen to sign in for it.
     */
    fun addRemote(label: String, url: String) {
        val trimmed = url.trim().trimEnd('/')
        if (!Validators.isValidServerUrl(trimmed)) {
            _uiState.update { it.copy(errorRes = R.string.servers_add_invalid_url) }
            return
        }
        viewModelScope.launch {
            _uiState.update { it.copy(isBusy = true, errorRes = null) }
            val profile = sessionManager.addRemoteProfile(label.trim(), trimmed)
            _uiState.update { it.copy(isBusy = false) }
            activate(profile.id, discardPending = false)
        }
    }

    fun rename(profileId: String, label: String) {
        if (label.isBlank()) return
        viewModelScope.launch { sessionManager.renameProfile(profileId, label.trim()) }
    }

    /**
     * Remove a profile. A Remote profile's active session is revoked through the
     * existing teardown path by [SessionManager.removeProfile].
     */
    fun remove(profileId: String) {
        viewModelScope.launch {
            _uiState.update { it.copy(isBusy = true) }
            sessionManager.removeProfile(profileId)
            bundleBackupRepository.forget(profileId)
            _uiState.update { it.copy(isBusy = false) }
            _events.send(ServersEvent.Removed)
        }
    }

    /**
     * ADR 0028 Decision 1: a Local profile's destructive action. Unlike removing
     * a Remote profile (whose data lives on a server), this stops the embedded
     * server and deletes the only copy of the data — the store and its
     * Keystore-wrapped keys — then drops the profile. Callers gate it behind a
     * typed confirmation.
     */
    fun deleteLocalData(profileId: String) {
        viewModelScope.launch {
            _uiState.update { it.copy(isBusy = true) }
            localServerHost.deleteLocalData()
            sessionManager.removeProfile(profileId)
            bundleBackupRepository.forget(profileId)
            _uiState.update { it.copy(isBusy = false) }
            _events.send(ServersEvent.Removed)
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(errorRes = null) }
    }

    private suspend fun activate(profileId: String, discardPending: Boolean) {
        _uiState.update { it.copy(isBusy = true, errorRes = null) }
        when (val result = sessionManager.switchProfile(profileId, discardPending)) {
            SwitchProfileResult.Switched -> {
                _uiState.update { it.copy(isBusy = false) }
                _events.send(ServersEvent.Switched)
            }
            is SwitchProfileResult.NeedsConfirmation -> {
                _uiState.update {
                    it.copy(isBusy = false, pendingSwitch = PendingSwitch(profileId, result.pendingCount))
                }
            }
        }
    }
}
