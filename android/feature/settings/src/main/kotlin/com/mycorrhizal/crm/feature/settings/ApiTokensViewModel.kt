package com.mycorrhizal.crm.feature.settings

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ApiTokenRepository
import com.mycorrhizal.crm.model.network.ApiToken
import com.mycorrhizal.crm.model.network.ApiTokenCreateResponse
import com.mycorrhizal.crm.model.network.ApiTokenInput
import com.mycorrhizal.crm.model.network.DEFAULT_API_TOKEN_EXPIRY_DAYS
import com.mycorrhizal.crm.model.network.DEFAULT_API_TOKEN_SCOPE
import com.mycorrhizal.crm.network.ApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class ApiTokensUiState(
    val tokens: List<ApiToken> = emptyList(),
    val isLoading: Boolean = false,
    val isSaving: Boolean = false,
    val isRevokingAll: Boolean = false,
    /** Id of the token whose single revoke call is in flight, if any. */
    val revokingId: Int? = null,
    /** Id of the token whose rotate call is in flight, if any. */
    val rotatingId: Int? = null,
    /** The just-created or just-rotated token whose secret is on screen (one-shot dialog). */
    val revealedToken: ApiTokenCreateResponse? = null,
    /** True when [revealedToken] came from rotate rather than create -- swaps the dialog's copy. */
    val revealedIsRotation: Boolean = false,
    /** A transient action error (load/create/revoke/revoke-all/rotate), shown and then cleared. */
    val error: String? = null,
    /** How many tokens the last revoke-all call actually revoked, shown and then cleared. */
    val revokedAllCount: Int? = null,
) {
    val activeCount: Int get() = tokens.count { it.isActive() }
}

/** A token is usable when it hasn't been revoked and hasn't passed its expiry. */
fun ApiToken.isExpired(): Boolean {
    val expiresAt = expiresAt ?: return false
    return runCatching { java.time.Instant.parse(expiresAt) <= java.time.Instant.now() }.getOrDefault(false)
}

fun ApiToken.isActive(): Boolean = revokedAt == null && !isExpired()

/**
 * API token list/create/revoke/revoke-all/rotate (issue #413's Android
 * follow-up, #573), mirroring web's SettingsPage.tsx API-token section.
 *
 * Unlike [WebhooksViewModel] (which patches its list in memory after a
 * mutation), every mutating call here re-[load]s the list afterward instead
 * of patching state locally. A revoke/rotate changes a token's status and
 * (for rotate) mints a whole new row; re-fetching keeps `revoked_at` and the
 * newly-issued row's timestamps as the server's authoritative values rather
 * than a client-fabricated guess -- the same choice web's SettingsPage.tsx
 * makes (`await fetchTokens()` after every mutation).
 */
@HiltViewModel
class ApiTokensViewModel @Inject constructor(
    private val repository: ApiTokenRepository,
) : ViewModel() {

    private val _uiState = MutableStateFlow(ApiTokensUiState())
    val uiState: StateFlow<ApiTokensUiState> = _uiState.asStateFlow()

    init {
        load()
    }

    fun load() {
        if (_uiState.value.isLoading) return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            repository.list()
                .onSuccess { list -> _uiState.update { it.copy(isLoading = false, tokens = list) } }
                .onFailure { e -> _uiState.update { it.copy(isLoading = false, error = e.displayMessage()) } }
        }
    }

    fun create(
        name: String,
        expiresInDays: Int = DEFAULT_API_TOKEN_EXPIRY_DAYS,
        scope: String = DEFAULT_API_TOKEN_SCOPE,
    ) {
        if (_uiState.value.isSaving || name.isBlank()) return
        viewModelScope.launch {
            _uiState.update { it.copy(isSaving = true, error = null) }
            repository.create(ApiTokenInput(name = name.trim(), expiresInDays = expiresInDays, scope = scope))
                .onSuccess { created ->
                    _uiState.update {
                        it.copy(isSaving = false, revealedToken = created, revealedIsRotation = false)
                    }
                    load()
                }
                .onFailure { e -> _uiState.update { it.copy(isSaving = false, error = e.displayMessage()) } }
        }
    }

    fun revoke(token: ApiToken) {
        viewModelScope.launch {
            _uiState.update { it.copy(revokingId = token.id, error = null) }
            repository.revoke(token.id)
                .onSuccess {
                    _uiState.update { it.copy(revokingId = null) }
                    load()
                }
                .onFailure { e -> _uiState.update { it.copy(revokingId = null, error = e.displayMessage()) } }
        }
    }

    fun revokeAll() {
        if (_uiState.value.isRevokingAll) return
        viewModelScope.launch {
            _uiState.update { it.copy(isRevokingAll = true, error = null, revokedAllCount = null) }
            repository.revokeAll()
                .onSuccess { result ->
                    _uiState.update { it.copy(isRevokingAll = false, revokedAllCount = result.revoked) }
                    load()
                }
                .onFailure { e -> _uiState.update { it.copy(isRevokingAll = false, error = e.displayMessage()) } }
        }
    }

    fun rotate(token: ApiToken) {
        viewModelScope.launch {
            _uiState.update { it.copy(rotatingId = token.id, error = null) }
            repository.rotate(token.id)
                .onSuccess { result ->
                    _uiState.update {
                        it.copy(rotatingId = null, revealedToken = result, revealedIsRotation = true)
                    }
                    load()
                }
                .onFailure { e -> _uiState.update { it.copy(rotatingId = null, error = e.displayMessage()) } }
        }
    }

    fun dismissRevealedToken() {
        _uiState.update { it.copy(revealedToken = null, revealedIsRotation = false) }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null) }
    }

    fun onRevokedAllCountShown() {
        _uiState.update { it.copy(revokedAllCount = null) }
    }

    private fun Throwable.displayMessage(): String =
        (this as? ApiError)?.displayMessage ?: message ?: "error"
}
