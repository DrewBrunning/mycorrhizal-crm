package com.mycorrhizal.crm.feature.cadence

import androidx.annotation.StringRes
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.CadencePolicyRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.model.network.CadencePolicy
import com.mycorrhizal.crm.model.network.CadencePolicyInput
import com.mycorrhizal.crm.network.foldApiError
import com.mycorrhizal.crm.ui.R
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class CadenceUiState(
    val contactId: Int = 0,
    /** Resolved from the record so the policy's `entity_id` (a VCardUID) can be sent. */
    val contactVCardUid: String = "",
    /** At most one policy per contact (server rejects a second with a 409). */
    val policy: CadencePolicy? = null,
    /** The signed-in user's `date_format` preference; falls back to "eu" when absent. */
    val dateFormat: String? = null,
    val isLoading: Boolean = false,
    val isMutating: Boolean = false,
    @StringRes val errorRes: Int? = null,
    val error: String? = null,
)

@HiltViewModel
class CadenceViewModel @Inject constructor(
    private val cadencePolicyRepository: CadencePolicyRepository,
    private val contactRepository: ContactRepository,
    private val authRepository: AuthRepository,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {

    private val contactId: Int = run {
        val raw: Any? = savedStateHandle["contactId"]
        (raw as? Int) ?: (raw as? String)?.toIntOrNull() ?: 0
    }

    private val _uiState = MutableStateFlow(CadenceUiState(contactId = contactId))
    val uiState: StateFlow<CadenceUiState> = _uiState.asStateFlow()

    init {
        load()
        // The health captions render next-due/last-interaction in the user's
        // date format (mirroring M11's prep view), so keep it from the session.
        viewModelScope.launch {
            authRepository.observeSession().collect { session ->
                _uiState.update { it.copy(dateFormat = session.dateFormat) }
            }
        }
    }

    fun load() {
        if (contactId == 0) {
            _uiState.update { it.copy(isLoading = false, errorRes = R.string.cadence_error_missing_contact_id, error = null) }
            return
        }
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null, errorRes = null) }
            // The policy's entity_id is the contact's VCardUID, so resolve it
            // from the record before listing. vcardUid prefers the top-level
            // uid (the canonical field; issue #693).
            val uid = contactRepository.getContact(contactId).getOrNull()?.vcardUid
            if (uid.isNullOrBlank()) {
                _uiState.update { it.copy(isLoading = false, errorRes = R.string.cadence_error_no_vcard_uid, error = null) }
                return@launch
            }
            _uiState.update { it.copy(contactVCardUid = uid) }
            cadencePolicyRepository.listForContact(uid).foldApiError(
                onSuccess = { policies ->
                    _uiState.update { it.copy(isLoading = false, policy = policies.firstOrNull()) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    /**
     * Create a policy for this contact. [qualifyingTypes] is sent verbatim —
     * an empty selection is preserved as empty ("every default-qualifying type
     * counts"), never defaulted to a populated list.
     */
    fun create(intervalDays: Int, qualifyingTypes: List<String>) {
        val uid = _uiState.value.contactVCardUid
        if (uid.isBlank() || _uiState.value.isMutating) return
        viewModelScope.launch {
            _uiState.update { it.copy(isMutating = true, error = null) }
            cadencePolicyRepository.create(CadencePolicyInput(uid, intervalDays, qualifyingTypes)).foldApiError(
                onSuccess = { policy -> _uiState.update { it.copy(isMutating = false, policy = policy) } },
                onError = { error -> _uiState.update { it.copy(isMutating = false, error = error.displayMessage) } },
            )
        }
    }

    /** Update the existing policy's interval/qualifying types; entity_id is resent verbatim. */
    fun update(id: String, intervalDays: Int, qualifyingTypes: List<String>) {
        val entityId = _uiState.value.policy?.entityId ?: return
        if (_uiState.value.isMutating) return
        viewModelScope.launch {
            _uiState.update { it.copy(isMutating = true, error = null) }
            cadencePolicyRepository.update(id, CadencePolicyInput(entityId, intervalDays, qualifyingTypes)).foldApiError(
                onSuccess = { policy -> _uiState.update { it.copy(isMutating = false, policy = policy) } },
                onError = { error -> _uiState.update { it.copy(isMutating = false, error = error.displayMessage) } },
            )
        }
    }

    /** Delete the policy; the screen returns to the empty (no-policy) state. */
    fun delete(id: String) {
        if (_uiState.value.isMutating) return
        viewModelScope.launch {
            _uiState.update { it.copy(isMutating = true, error = null) }
            cadencePolicyRepository.delete(id).foldApiError(
                onSuccess = { _uiState.update { it.copy(isMutating = false, policy = null) } },
                onError = { error -> _uiState.update { it.copy(isMutating = false, error = error.displayMessage) } },
            )
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(errorRes = null, error = null) }
    }
}
