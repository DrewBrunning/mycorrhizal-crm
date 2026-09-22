package com.mycorrhizal.crm.feature.settings

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.model.network.ContactFieldKey
import com.mycorrhizal.crm.model.network.DEFAULT_ENABLED_CONTACT_FIELDS
import com.mycorrhizal.crm.model.network.resolveEnabledFields
import com.mycorrhizal.crm.network.ApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class ContactFieldSettingsUiState(
    val isLoading: Boolean = true,
    val enabled: Set<ContactFieldKey> = DEFAULT_ENABLED_CONTACT_FIELDS,
    /** The one row whose switch is disabled while its own PATCH is in flight. */
    val savingKey: ContactFieldKey? = null,
    val loadError: String? = null,
    val saveError: String? = null,
)

/**
 * Issue #832 (web parity with `frontend/src/components/ContactFieldSettings.tsx`):
 * toggles which extended contact fields the detail/form screens show. Unlike
 * [ImmichSettingsViewModel]/[NotificationChannelsViewModel] (one batch Save),
 * this is self-saving per toggle — flipping a switch immediately PATCHes,
 * matching web exactly: there is no separate Save button here.
 */
@HiltViewModel
class ContactFieldSettingsViewModel @Inject constructor(
    private val authRepository: AuthRepository,
) : ViewModel() {

    private val _uiState = MutableStateFlow(ContactFieldSettingsUiState())
    val uiState: StateFlow<ContactFieldSettingsUiState> = _uiState.asStateFlow()

    init {
        load()
    }

    fun load() {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, loadError = null) }
            authRepository.getEnabledContactFields()
                .onSuccess { stored ->
                    _uiState.update { it.copy(isLoading = false, enabled = resolveEnabledFields(stored)) }
                }
                .onFailure { e ->
                    _uiState.update { it.copy(isLoading = false, loadError = e.displayMessage()) }
                }
        }
    }

    /**
     * Optimistically flip [key], then PATCH the full resulting set. On
     * failure, revert the flip and surface an inline error — mirrors web's
     * `handleToggle`.
     */
    fun onToggle(key: ContactFieldKey) {
        if (_uiState.value.savingKey != null) return
        val previous = _uiState.value.enabled
        val next = if (key in previous) previous - key else previous + key
        _uiState.update { it.copy(enabled = next, savingKey = key, saveError = null) }
        viewModelScope.launch {
            authRepository.updateEnabledContactFields(next.map { it.wireKey })
                .onSuccess {
                    _uiState.update { it.copy(savingKey = null) }
                }
                .onFailure { e ->
                    _uiState.update { it.copy(enabled = previous, savingKey = null, saveError = e.displayMessage()) }
                }
        }
    }

    fun onSaveErrorShown() {
        _uiState.update { it.copy(saveError = null) }
    }

    private fun Throwable.displayMessage(): String =
        (this as? ApiError)?.displayMessage ?: message ?: "error"
}
