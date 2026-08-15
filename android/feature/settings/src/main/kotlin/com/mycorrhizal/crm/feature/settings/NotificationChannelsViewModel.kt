package com.mycorrhizal.crm.feature.settings

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.NotificationSettingsRepository
import com.mycorrhizal.crm.model.network.NotificationConfig
import com.mycorrhizal.crm.model.network.NotificationConfigInput
import com.mycorrhizal.crm.network.ApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class NotificationChannelsUiState(
    val isLoading: Boolean = true,
    val isSaving: Boolean = false,
    val loadError: String? = null,
    val saveError: String? = null,
    /** Last test outcome keyed by channel ("ntfy"/"gotify"); null = not tested since last change. */
    val testResult: Map<String, NotificationTestOutcome> = emptyMap(),
    val testingChannel: String? = null,
    // ntfy
    val ntfyUrl: String = "",
    val ntfyTopic: String = "",
    val notifyNtfy: Boolean = false,
    // gotify
    val gotifyUrl: String = "",
    val gotifyHasToken: Boolean = false,
    val notifyGotify: Boolean = false,
    /** Write-only: what the user typed this session. Empty on save keeps the stored token. */
    val gotifyToken: String = "",
)

data class NotificationTestOutcome(
    val ok: Boolean,
    val error: String? = null,
)

@HiltViewModel
class NotificationChannelsViewModel @Inject constructor(
    private val repository: NotificationSettingsRepository,
) : ViewModel() {

    private val _uiState = MutableStateFlow(NotificationChannelsUiState())
    val uiState: StateFlow<NotificationChannelsUiState> = _uiState.asStateFlow()

    init {
        load()
    }

    fun load() {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, loadError = null) }
            repository.config()
                .onSuccess { config ->
                    _uiState.update {
                        it.copy(
                            isLoading = false,
                            ntfyUrl = config.ntfyUrl,
                            ntfyTopic = config.ntfyTopic,
                            notifyNtfy = config.notifyNtfy,
                            gotifyUrl = config.gotifyUrl,
                            gotifyHasToken = config.gotifyHasToken,
                            notifyGotify = config.notifyGotify,
                        )
                    }
                }
                .onFailure { e ->
                    _uiState.update { it.copy(isLoading = false, loadError = e.displayMessage()) }
                }
        }
    }

    fun onNtfyUrlChange(value: String) = _uiState.update { it.copy(ntfyUrl = value, testResult = emptyMap()) }
    fun onNtfyTopicChange(value: String) = _uiState.update { it.copy(ntfyTopic = value, testResult = emptyMap()) }
    fun onNotifyNtfyChange(value: Boolean) = _uiState.update { it.copy(notifyNtfy = value, testResult = emptyMap()) }
    fun onGotifyUrlChange(value: String) = _uiState.update { it.copy(gotifyUrl = value, testResult = emptyMap()) }
    fun onGotifyTokenChange(value: String) = _uiState.update { it.copy(gotifyToken = value, testResult = emptyMap()) }
    fun onNotifyGotifyChange(value: Boolean) = _uiState.update { it.copy(notifyGotify = value, testResult = emptyMap()) }

    /** Save the current ntfy/gotify config (URLs + toggles). An empty gotify token keeps the stored one. */
    fun save() {
        if (_uiState.value.isSaving) return
        viewModelScope.launch {
            _uiState.update { it.copy(isSaving = true, saveError = null) }
            val s = _uiState.value
            repository.save(
                NotificationConfigInput(
                    ntfyUrl = s.ntfyUrl.trim().ifBlank { null },
                    ntfyTopic = s.ntfyTopic.trim().ifBlank { null },
                    gotifyUrl = s.gotifyUrl.trim().ifBlank { null },
                    gotifyToken = s.gotifyToken.trim().ifBlank { null },
                    notifyNtfy = s.notifyNtfy,
                    notifyGotify = s.notifyGotify,
                ),
            )
                .onSuccess { config ->
                    _uiState.update {
                        it.copy(
                            isSaving = false,
                            ntfyUrl = config.ntfyUrl,
                            ntfyTopic = config.ntfyTopic,
                            notifyNtfy = config.notifyNtfy,
                            gotifyUrl = config.gotifyUrl,
                            gotifyHasToken = config.gotifyHasToken,
                            notifyGotify = config.notifyGotify,
                            // The token is write-only — clear what the user typed
                            // so it never lingers in state (mirrors web).
                            gotifyToken = "",
                        )
                    }
                }
                .onFailure { e ->
                    _uiState.update { it.copy(isSaving = false, saveError = e.displayMessage()) }
                }
        }
    }

    /**
     * Test a channel. The web saves before testing so the test uses what the
     * user is looking at (not stale stored config); mirror that. If the save
     * itself fails, that failure IS the test result — testing against stale
     * stored config would report a misleading success.
     */
    fun test(channel: String) {
        if (_uiState.value.testingChannel != null) return
        viewModelScope.launch {
            _uiState.update { it.copy(testingChannel = channel, saveError = null) }
            val s = _uiState.value
            val saved = repository.save(
                NotificationConfigInput(
                    ntfyUrl = s.ntfyUrl.trim().ifBlank { null },
                    ntfyTopic = s.ntfyTopic.trim().ifBlank { null },
                    gotifyUrl = s.gotifyUrl.trim().ifBlank { null },
                    gotifyToken = s.gotifyToken.trim().ifBlank { null },
                    notifyNtfy = s.notifyNtfy,
                    notifyGotify = s.notifyGotify,
                ),
            )
            if (saved.isFailure) {
                _uiState.update {
                    it.copy(
                        testingChannel = null,
                        testResult = it.testResult + (channel to NotificationTestOutcome(ok = false, error = saved.exceptionOrNull().displayMessage())),
                    )
                }
                return@launch
            }
            repository.test(channel)
                .onSuccess { result ->
                    _uiState.update {
                        it.copy(
                            testingChannel = null,
                            testResult = it.testResult + (channel to NotificationTestOutcome(ok = result.ok, error = result.error)),
                        )
                    }
                }
                .onFailure { e ->
                    _uiState.update {
                        it.copy(
                            testingChannel = null,
                            testResult = it.testResult + (channel to NotificationTestOutcome(ok = false, error = e.displayMessage())),
                        )
                    }
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
