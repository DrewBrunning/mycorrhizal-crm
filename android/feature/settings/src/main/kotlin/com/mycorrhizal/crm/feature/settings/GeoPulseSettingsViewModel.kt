package com.mycorrhizal.crm.feature.settings

import androidx.annotation.StringRes
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.GeoPulseRepository
import com.mycorrhizal.crm.model.network.GeoPulseConfigInput
import com.mycorrhizal.crm.network.toApiError
import com.mycorrhizal.crm.ui.R
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.net.URI
import javax.inject.Inject

/**
 * Outcome of "Test connection". [stage] is the server's `reachability|auth|ok` diagnosis; an
 * `ok:false` result is a successful diagnosis, not an application error.
 */
data class GeoPulseTestOutcome(val ok: Boolean, val stage: String?, val message: String?)

data class GeoPulseSettingsUiState(
    val isLoading: Boolean = true,
    val isSaving: Boolean = false,
    val isTesting: Boolean = false,
    val isRemoving: Boolean = false,
    val loadError: String? = null,
    val saveError: String? = null,
    @StringRes val saveErrorRes: Int? = null,
    val testResult: GeoPulseTestOutcome? = null,
    val baseUrl: String = "",
    /** The base URL as stored server-side (what the origin-change rule compares against). */
    val storedBaseUrl: String = "",
    val hasApiKey: Boolean = false,
    /** Write-only: what the user typed this session. */
    val apiKey: String = "",
) {
    /**
     * Mirrors the backend rule (issue #1501): the token is required on first connect, and when the
     * typed base URL's origin differs from the stored one — the stored token is never sent to a
     * different server. Unparseable stored URLs count as a different origin (fail closed).
     */
    val tokenRequired: Boolean
        get() = !hasApiKey || (geoPulseOriginOf(baseUrl) != null && !sameGeoPulseOrigin(storedBaseUrl, baseUrl))

    /** True when the token is required *because the origin changed* (drives the explanatory hint). */
    val tokenRequiredByOriginChange: Boolean
        get() = hasApiKey && tokenRequired
}

/** scheme://host:port (lowercased, default port filled in), or null if [raw] isn't an http(s) URL. */
internal fun geoPulseOriginOf(raw: String): String? = try {
    val uri = URI(raw.trim())
    val scheme = uri.scheme?.lowercase()
    val host = uri.host?.lowercase()
    if ((scheme != "http" && scheme != "https") || host.isNullOrEmpty()) {
        null
    } else {
        val port = if (uri.port != -1) uri.port else if (scheme == "https") 443 else 80
        "$scheme://$host:$port"
    }
} catch (_: Exception) {
    null
}

internal fun sameGeoPulseOrigin(a: String, b: String): Boolean {
    val oa = geoPulseOriginOf(a) ?: return false
    return oa == geoPulseOriginOf(b)
}

/**
 * Issue #160 (ADR 0033), Android parity with web's `GeoPulseSettings.tsx`: base URL + write-only
 * API token, save / test connection / disconnect. Shape mirrors [ImmichSettingsViewModel].
 */
@HiltViewModel
class GeoPulseSettingsViewModel @Inject constructor(
    private val repository: GeoPulseRepository,
) : ViewModel() {

    private val _uiState = MutableStateFlow(GeoPulseSettingsUiState())
    val uiState: StateFlow<GeoPulseSettingsUiState> = _uiState.asStateFlow()

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
                            storedBaseUrl = config.baseUrl.orEmpty(),
                            hasApiKey = config.hasApiKey,
                        )
                    }
                }
                .onFailure { e -> _uiState.update { it.copy(isLoading = false, loadError = e.message()) } }
        }
    }

    fun onBaseUrlChange(value: String) =
        _uiState.update { it.copy(baseUrl = value, saveError = null, saveErrorRes = null, testResult = null) }

    fun onApiKeyChange(value: String) =
        _uiState.update { it.copy(apiKey = value, saveError = null, saveErrorRes = null, testResult = null) }

    /** Save the connection. A blank token keeps the stored one only when [GeoPulseSettingsUiState.tokenRequired] is false. */
    fun save() {
        val s = _uiState.value
        if (s.isSaving) return
        val trimmed = s.baseUrl.trim()
        when {
            trimmed.isEmpty() -> return fail(R.string.geopulse_settings_base_url_required)
            geoPulseOriginOf(trimmed) == null -> return fail(R.string.geopulse_settings_base_url_invalid)
            s.tokenRequired && s.apiKey.isBlank() ->
                return fail(
                    if (s.tokenRequiredByOriginChange) {
                        R.string.geopulse_settings_api_key_required_origin_change
                    } else {
                        R.string.geopulse_settings_api_key_required
                    },
                )
        }
        viewModelScope.launch {
            _uiState.update { it.copy(isSaving = true, saveError = null, saveErrorRes = null) }
            repository.saveConfig(GeoPulseConfigInput(baseUrl = trimmed, apiKey = s.apiKey.trim()))
                .onSuccess { config ->
                    _uiState.update {
                        it.copy(
                            isSaving = false,
                            baseUrl = config.baseUrl.orEmpty(),
                            storedBaseUrl = config.baseUrl.orEmpty(),
                            hasApiKey = config.hasApiKey,
                            // Write-only: never linger in state once sent.
                            apiKey = "",
                        )
                    }
                }
                .onFailure { e -> _uiState.update { it.copy(isSaving = false, saveError = e.message()) } }
        }
    }

    private fun fail(@StringRes res: Int) = _uiState.update { it.copy(saveErrorRes = res, saveError = null) }

    /** Diagnose the *saved* connection (reachability, then token validity). */
    fun test() {
        if (_uiState.value.isTesting) return
        viewModelScope.launch {
            _uiState.update { it.copy(isTesting = true, testResult = null) }
            repository.testConnection()
                .onSuccess { r ->
                    _uiState.update {
                        it.copy(isTesting = false, testResult = GeoPulseTestOutcome(r.ok, r.stage, r.message))
                    }
                }
                .onFailure { e ->
                    _uiState.update {
                        it.copy(isTesting = false, testResult = GeoPulseTestOutcome(false, null, e.message()))
                    }
                }
        }
    }

    /** Disconnect: removes the stored connection (the screen confirms first). Logged activities are kept. */
    fun remove() {
        if (_uiState.value.isRemoving) return
        viewModelScope.launch {
            _uiState.update { it.copy(isRemoving = true, saveError = null, saveErrorRes = null) }
            repository.deleteConfig()
                .onSuccess { _uiState.value = GeoPulseSettingsUiState(isLoading = false) }
                .onFailure { e -> _uiState.update { it.copy(isRemoving = false, saveError = e.message()) } }
        }
    }

    private fun Throwable.message(): String = toApiError().displayMessage
}
