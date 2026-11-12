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
    val selectedRecipientId: Long? = null,
    /** Days until the grant expires, or null for no expiry. */
    val expiryDays: Int? = null,
    val isSharing: Boolean = false,
    /** Set when the grant was created; the screen navigates to "Shared by me". */
    val shared: Boolean = false
) {
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

    fun selectRecipient(id: Long?) {
        _uiState.update { it.copy(selectedRecipientId = id) }
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
