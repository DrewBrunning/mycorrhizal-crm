package com.mycorrhizal.crm.feature.settings

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.PaperlessRepository
import com.mycorrhizal.crm.model.network.PaperlessConfigInput
import com.mycorrhizal.crm.network.ApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class PaperlessSettingsUiState(
    val isLoading: Boolean = true,
    val isSaving: Boolean = false,
    val isTesting: Boolean = false,
    val isRemoving: Boolean = false,
    val loadError: String? = null,
    val saveError: String? = null,
    val testResult: PaperlessTestOutcome? = null,
    val baseUrl: String = "",
    val hasApiToken: Boolean = false,
    /** Write-only: what the user typed this session. Empty on save keeps the stored token. */
    val apiToken: String = "",
)

data class PaperlessTestOutcome(val ok: Boolean, val message: String?)

/**
 * Issue #833: the Paperless-ngx connection-config settings screen — base URL + API
 * token, save/test/remove. Shape mirrors [com.mycorrhizal.crm.feature.settings.ImmichSettingsViewModel]
 * (issue #236), minus the sync toggle/status Paperless's config has no equivalent of.
 */
@HiltViewModel
class PaperlessSettingsViewModel @Inject constructor(
    private val repository: PaperlessRepository,
) : ViewModel() {

    private val _uiState = MutableStateFlow(PaperlessSettingsUiState())
    val uiState: StateFlow<PaperlessSettingsUiState> = _uiState.asStateFlow()

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
                            hasApiToken = config.hasApiToken,
                        )
                    }
                }
                .onFailure { e ->
                    _uiState.update { it.copy(isLoading = false, loadError = e.displayMessage()) }
                }
        }
    }

    fun onBaseUrlChange(value: String) = _uiState.update { it.copy(baseUrl = value, testResult = null) }
    fun onApiTokenChange(value: String) = _uiState.update { it.copy(apiToken = value, testResult = null) }

    /** Save the current base URL/API token. An empty API token keeps the stored one. */
    fun save() {
        if (_uiState.value.isSaving) return
        viewModelScope.launch {
            _uiState.update { it.copy(isSaving = true, saveError = null) }
            val s = _uiState.value
            repository.saveConfig(
                PaperlessConfigInput(baseUrl = s.baseUrl.trim(), apiToken = s.apiToken.trim()),
            )
                .onSuccess { config ->
                    _uiState.update {
                        it.copy(
                            isSaving = false,
                            baseUrl = config.baseUrl.orEmpty(),
                            hasApiToken = config.hasApiToken,
                            // The token is write-only — clear what the user typed so it
                            // never lingers in state (mirrors web and Immich).
                            apiToken = "",
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
                PaperlessConfigInput(baseUrl = s.baseUrl.trim(), apiToken = s.apiToken.trim()),
            )
            if (saved.isFailure) {
                _uiState.update {
                    it.copy(
                        isTesting = false,
                        testResult = PaperlessTestOutcome(ok = false, message = saved.exceptionOrNull().displayMessage()),
                    )
                }
                return@launch
            }
            val config = saved.getOrThrow()
            _uiState.update {
                it.copy(
                    baseUrl = config.baseUrl.orEmpty(),
                    hasApiToken = config.hasApiToken,
                    apiToken = "",
                )
            }
            repository.testConnection()
                .onSuccess { result ->
                    _uiState.update {
                        it.copy(isTesting = false, testResult = PaperlessTestOutcome(ok = result.ok, message = result.message))
                    }
                }
                .onFailure { e ->
                    _uiState.update {
                        it.copy(isTesting = false, testResult = PaperlessTestOutcome(ok = false, message = e.displayMessage()))
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
                        PaperlessSettingsUiState(isLoading = false)
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
