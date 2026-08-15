package com.mycorrhizal.crm.feature.shares

import androidx.annotation.StringRes
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ContactShareRepository
import com.mycorrhizal.crm.model.network.ContactShareInput
import com.mycorrhizal.crm.model.network.ShareFieldSections
import com.mycorrhizal.crm.model.network.UserDirectoryEntry
import com.mycorrhizal.crm.network.foldApiError
import com.mycorrhizal.crm.ui.R
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class ShareContactUiState(
    val isLoading: Boolean = false,
    val error: String? = null,
    @StringRes val errorRes: Int? = null,
    /** The other users on the instance, for the recipient picker. */
    val recipients: List<UserDirectoryEntry> = emptyList(),
    val selectedRecipientId: Long? = null,
    /** Selected section tokens; defaults to all non-sensitive sections. */
    val selectedSections: Set<String> = ShareFieldSections.DEFAULT_SELECTED.toSet(),
    val sensitiveRevealed: Boolean = false,
    val isSharing: Boolean = false,
    /** Set when the share was created successfully — the screen navigates back. */
    val shared: Boolean = false,
) {
    val canShare: Boolean
        get() = selectedRecipientId != null && selectedSections.isNotEmpty() && !isSharing
}

@HiltViewModel
class ShareContactViewModel @Inject constructor(
    private val contactShareRepository: ContactShareRepository,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {

    /**
     * The contact's VCard UID, passed through navigation from ContactDetailScreen
     * (which already has it loaded) — matching the web's ShareContactDialog, which
     * receives `vcardUID` as a prop rather than re-fetching the contact.
     */
    private val contactVCardUid: String = (savedStateHandle["uid"] as? String).orEmpty()

    private val _uiState = MutableStateFlow(ShareContactUiState())
    val uiState: StateFlow<ShareContactUiState> = _uiState.asStateFlow()

    init {
        load()
    }

    fun load() {
        if (_uiState.value.isLoading) return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null, errorRes = null) }
            contactShareRepository.userDirectory().foldApiError(
                onSuccess = { recipients ->
                    _uiState.update { it.copy(isLoading = false, recipients = recipients) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun selectRecipient(id: Long?) {
        _uiState.update { it.copy(selectedRecipientId = id) }
    }

    fun toggleSection(token: String, checked: Boolean) {
        _uiState.update { state ->
            val next = state.selectedSections.toMutableSet()
            if (checked) next.add(token) else next.remove(token)
            state.copy(selectedSections = next)
        }
    }

    /** Deliberate opt-in: unlocks the sensitivity-gated sections (mirrors the web's reveal step). */
    fun revealSensitive() {
        _uiState.update { it.copy(sensitiveRevealed = true) }
    }

    /**
     * Offer the contact. include_sensitive is only true when the user has
     * deliberately revealed AND selected a sensitivity-marked section — the
     * backend's foot-gun guard: an ordinary unchecked box cannot imply it.
     */
    fun share() {
        if (!_uiState.value.canShare) return
        if (contactVCardUid.isBlank()) {
            // Should be unreachable (the detail screen only offers Share for a
            // contact with a uid), but don't send an empty vcard_uid if it ever is.
            _uiState.update { it.copy(errorRes = R.string.shares_error_no_vcard_uid) }
            return
        }
        viewModelScope.launch {
            _uiState.update { it.copy(isSharing = true, error = null, errorRes = null) }
            val includeSensitive = _uiState.value.sensitiveRevealed &&
                _uiState.value.selectedSections.any { token ->
                    ShareFieldSections.ALL.any { it.token == token && it.sensitive }
                }
            contactShareRepository.create(
                ContactShareInput(
                    toUserId = _uiState.value.selectedRecipientId ?: 0L,
                    vcardUid = contactVCardUid,
                    sections = _uiState.value.selectedSections.toList(),
                    includeSensitive = includeSensitive,
                ),
            ).foldApiError(
                onSuccess = {
                    _uiState.update { it.copy(isSharing = false, shared = true) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isSharing = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null, errorRes = null) }
    }
}
