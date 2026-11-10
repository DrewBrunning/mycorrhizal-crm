package com.mycorrhizal.crm.feature.shares

import androidx.annotation.StringRes
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ContactShareRepository
import com.mycorrhizal.crm.domain.repository.ShareGrantRepository
import com.mycorrhizal.crm.model.network.ContactShareInput
import com.mycorrhizal.crm.model.network.ShareFieldSections
import com.mycorrhizal.crm.model.network.ShareGrantEntityTypes
import com.mycorrhizal.crm.model.network.ShareGrantInput
import com.mycorrhizal.crm.model.network.UserDirectoryEntry
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.foldApiError
import com.mycorrhizal.crm.ui.R
import dagger.hilt.android.lifecycle.HiltViewModel
import java.time.Instant
import java.time.temporal.ChronoUnit
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * ADR 0042 decision 12: the Share dialog offers a one-time copy (the existing flow, unchanged) or a
 * standing, revocable, read-only grant. The default is one-time; a standing share is always an
 * explicit choice.
 */
enum class ShareMode { ONE_TIME, STANDING }

data class ShareContactUiState(
    val isLoading: Boolean = false,
    val error: ApiError? = null,
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
    val mode: ShareMode = ShareMode.ONE_TIME,
    /** Standing only: days until the grant expires, or null for no expiry. */
    val expiryDays: Int? = null,
    /** True when the completed share was a standing grant (the recipient must still accept). */
    val sharedStanding: Boolean = false
) {
    val canShare: Boolean
        get() = selectedRecipientId != null && selectedSections.isNotEmpty() && !isSharing
}

@HiltViewModel
class ShareContactViewModel @Inject constructor(
    private val contactShareRepository: ContactShareRepository,
    private val shareGrantRepository: ShareGrantRepository,
    savedStateHandle: SavedStateHandle
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
                    _uiState.update { it.copy(isLoading = false, error = error) }
                }
            )
        }
    }

    /** The clock for "N days from now"; tests replace it. */
    internal var now: () -> Instant = { Instant.now() }

    fun setMode(mode: ShareMode) {
        _uiState.update { it.copy(mode = mode) }
    }

    /** Standing only: expire the grant after [days] days, or null for no expiry. */
    fun setExpiryDays(days: Int?) {
        _uiState.update { it.copy(expiryDays = days) }
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
        if (_uiState.value.mode == ShareMode.STANDING) {
            shareStanding()
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
                    includeSensitive = includeSensitive
                )
            ).foldApiError(
                onSuccess = {
                    _uiState.update { it.copy(isSharing = false, shared = true) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isSharing = false, error = error) }
                }
            )
        }
    }

    /**
     * Issue a standing grant (ADR 0042). It starts pending: nothing is visible to the recipient
     * until they accept. There is no include-sensitive switch on this path — the server filters
     * every grant read to normal-sensitivity items.
     */
    private fun shareStanding() {
        val state = _uiState.value
        viewModelScope.launch {
            _uiState.update { it.copy(isSharing = true, error = null, errorRes = null) }
            shareGrantRepository.create(
                ShareGrantInput(
                    toUserId = state.selectedRecipientId ?: 0L,
                    entityType = ShareGrantEntityTypes.CONTACT,
                    entityId = contactVCardUid,
                    sections = state.selectedSections.toList(),
                    expiresAt = state.expiryDays?.let {
                        now().plus(it.toLong(), ChronoUnit.DAYS).toString()
                    }
                )
            ).foldApiError(
                onSuccess = {
                    _uiState.update {
                        it.copy(isSharing = false, shared = true, sharedStanding = true)
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(isSharing = false, error = error) }
                }
            )
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null, errorRes = null) }
    }
}
