package com.mycorrhizal.crm.feature.shares

import androidx.annotation.StringRes
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ContactShareRepository
import com.mycorrhizal.crm.domain.repository.ShareGrantRepository
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

/** The entity types a [StandingShareViewModel] can share; a contact has its own screen. */
private val STANDING_SHARE_TYPES = setOf(
    ShareGrantEntityTypes.NOTE,
    ShareGrantEntityTypes.ACTIVITY,
    ShareGrantEntityTypes.CIRCLE
)

data class StandingShareUiState(
    /** One of [ShareGrantEntityTypes] NOTE, ACTIVITY or CIRCLE; picks the title and notice. */
    val entityType: String = "",
    val isLoading: Boolean = false,
    val error: ApiError? = null,
    @StringRes val errorRes: Int? = null,
    val recipients: List<UserDirectoryEntry> = emptyList(),
    /** True when the server only searches by username (inferred); see [RecipientDirectorySearch]. */
    val recipientSearchMode: Boolean = false,
    /** The text typed into the recipient field (search mode only). */
    val recipientQuery: String = "",
    /** The newest directory request for [recipientQuery] has answered. */
    val recipientSearchSettled: Boolean = false,
    /** The chosen recipient, kept even when a later search no longer lists them. */
    val selectedRecipient: UserDirectoryEntry? = null,
    /** Days until the grant expires, or null for no expiry. */
    val expiryDays: Int? = null,
    val isSharing: Boolean = false,
    /** Set when the grant was created; the screen navigates to "Shared by me". */
    val shared: Boolean = false
) {
    val selectedRecipientId: Long? get() = selectedRecipient?.id
    val canShare: Boolean get() = selectedRecipientId != null && !isSharing
}

/**
 * Issue #2518 (ADR 0042 decision 12): a standing, revocable, read-only share of a note, an
 * activity or a circle. These types have no sections, so the grant carries an empty list, and the
 * grant starts pending until the recipient accepts. The contact flow stays in
 * [ShareContactViewModel].
 */
@HiltViewModel
class StandingShareViewModel @Inject constructor(
    private val contactShareRepository: ContactShareRepository,
    private val shareGrantRepository: ShareGrantRepository,
    savedStateHandle: SavedStateHandle
) : ViewModel() {

    private val entityType: String = (savedStateHandle["entityType"] as? String).orEmpty()
    private val entityId: String = (savedStateHandle["entityId"] as? String).orEmpty()

    private val _uiState = MutableStateFlow(StandingShareUiState(entityType = entityType))
    val uiState: StateFlow<StandingShareUiState> = _uiState.asStateFlow()

    /** The clock for "N days from now"; tests replace it. */
    internal var now: () -> Instant = { Instant.now() }

    fun load() {
        if (_uiState.value.isLoading) return
        _uiState.update { it.copy(isLoading = true, error = null, errorRes = null) }
        search.load()
    }

    /** Search mode: the text typed into the recipient field (debounced, newest answer wins). */
    fun setRecipientQuery(query: String) {
        _uiState.update { state ->
            // Editing the text after a pick means the user is choosing someone else.
            val keep = state.selectedRecipient?.takeIf { it.username == query.trim() }
            state.copy(selectedRecipient = keep)
        }
        search.setQuery(query)
    }

    private val search = RecipientDirectorySearch(
        scope = viewModelScope,
        repository = contactShareRepository,
        onState = { s ->
            _uiState.update {
                it.copy(
                    isLoading = s.isLoading,
                    recipients = s.recipients,
                    recipientSearchMode = s.searchMode,
                    recipientQuery = s.query,
                    recipientSearchSettled = s.settled
                )
            }
        },
        onError = { error -> _uiState.update { it.copy(error = error) } }
    )

    init {
        load()
    }

    fun selectRecipient(id: Long?) {
        val picked = id?.let { wanted -> _uiState.value.recipients.firstOrNull { it.id == wanted } }
        _uiState.update { it.copy(selectedRecipient = picked) }
        // In search mode the field shows the picked name; that must not trigger a search.
        if (picked != null && _uiState.value.recipientSearchMode) search.showQuery(picked.username)
    }

    fun setExpiryDays(days: Int?) {
        _uiState.update { it.copy(expiryDays = days) }
    }

    fun share() {
        val state = _uiState.value
        if (!state.canShare) return
        if (entityType !in STANDING_SHARE_TYPES || entityId.isBlank()) {
            // Unreachable from the app's own entry points; never send a malformed grant.
            _uiState.update { it.copy(errorRes = R.string.shares_error_no_entity) }
            return
        }
        viewModelScope.launch {
            _uiState.update { it.copy(isSharing = true, error = null, errorRes = null) }
            shareGrantRepository.create(
                ShareGrantInput(
                    toUserId = state.selectedRecipientId ?: 0L,
                    entityType = entityType,
                    entityId = entityId,
                    sections = emptyList(),
                    expiresAt = state.expiryDays?.let {
                        now().plus(it.toLong(), ChronoUnit.DAYS).toString()
                    }
                )
            ).foldApiError(
                onSuccess = { _uiState.update { it.copy(isSharing = false, shared = true) } },
                onError = { error -> _uiState.update { it.copy(isSharing = false, error = error) } }
            )
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null, errorRes = null) }
    }
}
