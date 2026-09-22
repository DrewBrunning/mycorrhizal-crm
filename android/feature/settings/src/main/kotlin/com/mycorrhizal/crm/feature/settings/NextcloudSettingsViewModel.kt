package com.mycorrhizal.crm.feature.settings

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.NextcloudRepository
import com.mycorrhizal.crm.model.network.NextcloudConfigInput
import com.mycorrhizal.crm.network.ApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class NextcloudSettingsUiState(
    val isLoading: Boolean = true,
    val isSaving: Boolean = false,
    val isTesting: Boolean = false,
    val isRemoving: Boolean = false,
    val loadError: String? = null,
    val saveError: String? = null,
    val testResult: NextcloudTestOutcome? = null,
    val baseUrl: String = "",
    /** Unlike the API-token integrations, the username is basic-auth and not write-only. */
    val username: String = "",
    val hasAppPassword: Boolean = false,
    /** Write-only: what the user typed this session. Empty on save keeps the stored password. */
    val appPassword: String = "",
)

data class NextcloudTestOutcome(val ok: Boolean, val message: String?)

/**
 * Issue #833: the Nextcloud/ownCloud (WebDAV) connection-config settings screen —
 * base URL + username + app password, save/test/remove. Shape mirrors
 * [com.mycorrhizal.crm.feature.settings.ImmichSettingsViewModel] (issue #236), minus the sync
 * toggle/status Nextcloud's config has no equivalent of, plus the extra basic-auth
 * [NextcloudSettingsUiState.username] field web's `NextcloudSettings.tsx` requires
 * up front.
 */
@HiltViewModel
class NextcloudSettingsViewModel @Inject constructor(
    private val repository: NextcloudRepository,
) : ViewModel() {

    private val _uiState = MutableStateFlow(NextcloudSettingsUiState())
    val uiState: StateFlow<NextcloudSettingsUiState> = _uiState.asStateFlow()

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
                            username = config.username.orEmpty(),
                            hasAppPassword = config.hasAppPassword,
                        )
                    }
                }
                .onFailure { e ->
                    _uiState.update { it.copy(isLoading = false, loadError = e.displayMessage()) }
                }
        }
    }

    fun onBaseUrlChange(value: String) = _uiState.update { it.copy(baseUrl = value, testResult = null) }
    fun onUsernameChange(value: String) = _uiState.update { it.copy(username = value, testResult = null) }
    fun onAppPasswordChange(value: String) = _uiState.update { it.copy(appPassword = value, testResult = null) }

    /** Save the current base URL/username/app password. An empty password keeps the stored one. */
    fun save() {
        if (_uiState.value.isSaving) return
        viewModelScope.launch {
            _uiState.update { it.copy(isSaving = true, saveError = null) }
            val s = _uiState.value
            repository.saveConfig(
                NextcloudConfigInput(baseUrl = s.baseUrl.trim(), username = s.username.trim(), appPassword = s.appPassword.trim()),
            )
                .onSuccess { config ->
                    _uiState.update {
                        it.copy(
                            isSaving = false,
                            baseUrl = config.baseUrl.orEmpty(),
                            username = config.username.orEmpty(),
                            hasAppPassword = config.hasAppPassword,
                            // The password is write-only — clear what the user typed so it
                            // never lingers in state (mirrors web and Immich).
                            appPassword = "",
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
     * stored config; a save failure IS the test failure — same ordering as
     * [com.mycorrhizal.crm.feature.settings.ImmichSettingsViewModel.test].
     */
    fun test() {
        if (_uiState.value.isTesting) return
        viewModelScope.launch {
            _uiState.update { it.copy(isTesting = true, saveError = null, testResult = null) }
            val s = _uiState.value
            val saved = repository.saveConfig(
                NextcloudConfigInput(baseUrl = s.baseUrl.trim(), username = s.username.trim(), appPassword = s.appPassword.trim()),
            )
            if (saved.isFailure) {
                _uiState.update {
                    it.copy(
                        isTesting = false,
                        testResult = NextcloudTestOutcome(ok = false, message = saved.exceptionOrNull().displayMessage()),
                    )
                }
                return@launch
            }
            val config = saved.getOrThrow()
            _uiState.update {
                it.copy(
                    baseUrl = config.baseUrl.orEmpty(),
                    username = config.username.orEmpty(),
                    hasAppPassword = config.hasAppPassword,
                    appPassword = "",
                )
            }
            repository.testConnection()
                .onSuccess { result ->
                    _uiState.update {
                        it.copy(isTesting = false, testResult = NextcloudTestOutcome(ok = result.ok, message = result.message))
                    }
                }
                .onFailure { e ->
                    _uiState.update {
                        it.copy(isTesting = false, testResult = NextcloudTestOutcome(ok = false, message = e.displayMessage()))
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
                        NextcloudSettingsUiState(isLoading = false)
                    }
                }
                .onFailure { e ->
                    _uiState.update { it.copy(isRemoving = false, saveError = e.displayMessage()) }
                }
        }
    }

    private fun Throwable.displayMessage(): String =
        (this as? ApiError)?.displayMessage ?: message ?: "error"
}

private fun Throwable?.displayMessage(): String =
    this?.let { (it as? ApiError)?.displayMessage ?: it.message } ?: "error"
