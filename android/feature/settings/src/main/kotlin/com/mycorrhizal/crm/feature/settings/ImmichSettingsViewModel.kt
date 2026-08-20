package com.mycorrhizal.crm.feature.settings

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ImmichRepository
import com.mycorrhizal.crm.model.network.ImmichConfigInput
import com.mycorrhizal.crm.network.ApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class ImmichSettingsUiState(
    val isLoading: Boolean = true,
    val isSaving: Boolean = false,
    val isTesting: Boolean = false,
    val isRemoving: Boolean = false,
    val loadError: String? = null,
    val saveError: String? = null,
    val testResult: ImmichTestOutcome? = null,
    val baseUrl: String = "",
    val hasApiKey: Boolean = false,
    /** Write-only: what the user typed this session. Empty on save keeps the stored key. */
    val apiKey: String = "",
    /** Only meaningful (and only shown) once [hasApiKey] is true — mirrors web's `ImmichSettings.tsx`. */
    val syncEnabled: Boolean = true,
    val lastSyncStatus: String? = null,
    val lastSyncError: String? = null,
)

data class ImmichTestOutcome(val ok: Boolean, val message: String?)

/**
 * Issue #236: the Immich connection-config settings screen — base URL + API key +
 * sync toggle, save/test/remove. The person-link and profile-photo flows (#219/#220)
 * already exist; this is the missing piece that lets a user actually configure the
 * connection those flows depend on. Shape mirrors [NotificationChannelsViewModel]
 * (the closest existing analog: a write-only secret field, save-then-test ordering,
 * confirm-then-remove) rather than inventing a new settings-form pattern.
 */
@HiltViewModel
class ImmichSettingsViewModel @Inject constructor(
    private val repository: ImmichRepository,
) : ViewModel() {

    private val _uiState = MutableStateFlow(ImmichSettingsUiState())
    val uiState: StateFlow<ImmichSettingsUiState> = _uiState.asStateFlow()

    init {
        load()
    }

    fun load() {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, loadError = null) }
            repository.getConfig()
                .onSuccess { config ->
                    _uiState.update {
                        it.copy(
                            isLoading = false,
                            baseUrl = config.baseUrl.orEmpty(),
                            hasApiKey = config.hasApiKey,
                            syncEnabled = config.syncEnabled,
                            lastSyncStatus = config.lastSyncStatus,
                            lastSyncError = config.lastSyncError,
                        )
                    }
                }
                .onFailure { e ->
                    _uiState.update { it.copy(isLoading = false, loadError = e.displayMessage()) }
                }
        }
    }

    fun onBaseUrlChange(value: String) = _uiState.update { it.copy(baseUrl = value, testResult = null) }
    fun onApiKeyChange(value: String) = _uiState.update { it.copy(apiKey = value, testResult = null) }
    fun onSyncEnabledChange(value: Boolean) = _uiState.update { it.copy(syncEnabled = value, testResult = null) }

    /** Save the current base URL/API key/sync toggle. An empty API key keeps the stored one. */
    fun save() {
        if (_uiState.value.isSaving) return
        viewModelScope.launch {
            _uiState.update { it.copy(isSaving = true, saveError = null) }
            val s = _uiState.value
            repository.saveConfig(
                ImmichConfigInput(baseUrl = s.baseUrl.trim(), apiKey = s.apiKey.trim(), syncEnabled = s.syncEnabled),
            )
                .onSuccess { config ->
                    _uiState.update {
                        it.copy(
                            isSaving = false,
                            baseUrl = config.baseUrl.orEmpty(),
                            hasApiKey = config.hasApiKey,
                            syncEnabled = config.syncEnabled,
                            lastSyncStatus = config.lastSyncStatus,
                            lastSyncError = config.lastSyncError,
                            // The key is write-only — clear what the user typed so it
                            // never lingers in state (mirrors web and NotificationChannels).
                            apiKey = "",
                        )
                    }
                }
                .onFailure { e ->
                    _uiState.update { it.copy(isSaving = false, saveError = e.displayMessage()) }
                }
        }
    }

    /**
     * Test the connection. Saves first so the test uses what's on screen, not stale
     * stored config; a save failure IS the test failure — same ordering and rationale
     * as [NotificationChannelsViewModel.test].
     */
    fun test() {
        if (_uiState.value.isTesting) return
        viewModelScope.launch {
            _uiState.update { it.copy(isTesting = true, saveError = null, testResult = null) }
            val s = _uiState.value
            val saved = repository.saveConfig(
                ImmichConfigInput(baseUrl = s.baseUrl.trim(), apiKey = s.apiKey.trim(), syncEnabled = s.syncEnabled),
            )
            if (saved.isFailure) {
                _uiState.update {
                    it.copy(
                        isTesting = false,
                        testResult = ImmichTestOutcome(ok = false, message = saved.exceptionOrNull().displayMessage()),
                    )
                }
                return@launch
            }
            val config = saved.getOrThrow()
            _uiState.update {
                it.copy(
                    baseUrl = config.baseUrl.orEmpty(),
                    hasApiKey = config.hasApiKey,
                    syncEnabled = config.syncEnabled,
                    apiKey = "",
                )
            }
            repository.testConnection()
                .onSuccess { result ->
                    _uiState.update {
                        it.copy(isTesting = false, testResult = ImmichTestOutcome(ok = result.ok, message = result.message))
                    }
                }
                .onFailure { e ->
                    _uiState.update {
                        it.copy(isTesting = false, testResult = ImmichTestOutcome(ok = false, message = e.displayMessage()))
                    }
                }
        }
    }

    /** Remove the stored connection entirely (the screen confirms first). */
    fun remove() {
        if (_uiState.value.isRemoving) return
        viewModelScope.launch {
            _uiState.update { it.copy(isRemoving = true, saveError = null) }
            repository.deleteConfig()
                .onSuccess {
                    _uiState.update {
                        ImmichSettingsUiState(isLoading = false)
                    }
                }
                .onFailure { e ->
                    _uiState.update { it.copy(isRemoving = false, saveError = e.displayMessage()) }
                }
        }
    }

    fun onSaveErrorShown() {
        _uiState.update { it.copy(saveError = null, loadError = null) }
    }

    private fun Throwable.displayMessage(): String =
        (this as? ApiError)?.displayMessage ?: message ?: "error"
}

private fun Throwable?.displayMessage(): String =
    this?.let { (it as? ApiError)?.displayMessage ?: it.message } ?: "error"
