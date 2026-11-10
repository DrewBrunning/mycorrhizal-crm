package com.mycorrhizal.crm.feature.shares

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ShareGrantRepository
import com.mycorrhizal.crm.model.network.OutgoingShareGrant
import com.mycorrhizal.crm.model.network.ShareGrantPatch
import com.mycorrhizal.crm.model.network.ShareGrantStatuses
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.foldApiError
import com.mycorrhizal.crm.network.toApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import java.time.Instant
import java.time.temporal.ChronoUnit
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** True while the grant can still be revoked or edited (pending or active). */
val OutgoingShareGrant.isLive: Boolean
    get() = status == ShareGrantStatuses.PENDING || status == ShareGrantStatuses.ACTIVE

/** What to do with a grant's expiry when editing. */
sealed interface ExpiryChoice {
    /** Leave the expiry as it is. */
    data object Keep : ExpiryChoice

    /** Remove the expiry. */
    data object None : ExpiryChoice

    /** Expire this many days from now. */
    data class Days(val days: Int) : ExpiryChoice
}

/** A confirmation the owner must accept before an owner action runs. */
sealed interface SharedByMePrompt {
    val grant: OutgoingShareGrant

    /** End the grant. Already-seen data cannot be recalled (ADR 0042 decision 10). */
    data class Revoke(override val grant: OutgoingShareGrant) : SharedByMePrompt

    /** Release sections withheld after a sensitivity change, once the owner has reviewed them. */
    data class Reconfirm(override val grant: OutgoingShareGrant) : SharedByMePrompt
}

/** The narrow-only editor: sections can only be removed, the expiry changed. */
data class ShareGrantEditState(
    val grant: OutgoingShareGrant,
    val sections: Set<String>,
    val expiry: ExpiryChoice = ExpiryChoice.Keep,
    val saving: Boolean = false
)

data class SharedByMeUiState(
    val grants: List<OutgoingShareGrant> = emptyList(),
    val isLoading: Boolean = false,
    val loadError: ApiError? = null,
    val actionError: ApiError? = null,
    val prompt: SharedByMePrompt? = null,
    val edit: ShareGrantEditState? = null,
    val busy: Boolean = false
)

/**
 * ADR 0042 decision 12 (owner side): status, expiry, revoke, reconfirm and narrow-only edit.
 * The server is the authority on widening (409) and staleness (412); the editor merely never
 * offers a wider list.
 */
@HiltViewModel
class SharedByMeViewModel @Inject constructor(private val repository: ShareGrantRepository) :
    ViewModel() {

    /** The clock for "N days from now"; tests replace it. */
    internal var now: () -> Instant = { Instant.now() }

    private val _uiState = MutableStateFlow(SharedByMeUiState())
    val uiState: StateFlow<SharedByMeUiState> = _uiState.asStateFlow()

    init {
        load()
    }

    fun load() {
        if (_uiState.value.isLoading) return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, loadError = null) }
            val result = repository.listOutgoing()
            _uiState.update {
                it.copy(
                    isLoading = false,
                    grants = result.getOrNull()?.shareGrants.orEmpty(),
                    loadError = result.exceptionOrNull()?.toApiError()
                )
            }
        }
    }

    fun requestRevoke(grant: OutgoingShareGrant) {
        if (grant.isLive) openPrompt(SharedByMePrompt.Revoke(grant))
    }

    fun requestReconfirm(grant: OutgoingShareGrant) {
        if (grant.isLive && grant.reconfirmSections.isNotEmpty()) {
            openPrompt(SharedByMePrompt.Reconfirm(grant))
        }
    }

    private fun openPrompt(prompt: SharedByMePrompt) {
        if (_uiState.value.prompt != null) return
        _uiState.update { it.copy(prompt = prompt) }
    }

    fun dismissPrompt() {
        if (_uiState.value.busy) return
        _uiState.update { it.copy(prompt = null) }
    }

    fun confirmPrompt() {
        val prompt = _uiState.value.prompt ?: return
        runAction {
            when (prompt) {
                is SharedByMePrompt.Revoke -> repository.revoke(prompt.grant.id)
                is SharedByMePrompt.Reconfirm -> repository.reconfirm(prompt.grant.id)
            }
        }
    }

    // --- Narrow-only edit ---

    fun openEdit(grant: OutgoingShareGrant) {
        if (!grant.isLive || _uiState.value.edit != null) return
        _uiState.update {
            it.copy(edit = ShareGrantEditState(grant = grant, sections = grant.sections.toSet()))
        }
    }

    fun toggleEditSection(token: String, checked: Boolean) {
        _uiState.update { state ->
            val edit = state.edit ?: return@update state
            // Narrow-only: a section the grant never had cannot be added (the server refuses too),
            // and a contact grant keeps at least one section.
            if (token !in edit.grant.sections) return@update state
            val next = if (checked) edit.sections + token else edit.sections - token
            if (next.isEmpty() && edit.grant.sections.isNotEmpty()) return@update state
            state.copy(edit = edit.copy(sections = next))
        }
    }

    fun setEditExpiry(choice: ExpiryChoice) {
        _uiState.update { state ->
            state.copy(edit = state.edit?.copy(expiry = choice))
        }
    }

    fun closeEdit() {
        if (_uiState.value.edit?.saving == true) return
        _uiState.update { it.copy(edit = null) }
    }

    fun saveEdit() {
        val edit = _uiState.value.edit ?: return
        if (edit.saving) return
        val patch = buildPatch(edit)
        if (patch == null) {
            _uiState.update { it.copy(edit = null) }
            return
        }
        viewModelScope.launch {
            _uiState.update { it.copy(edit = edit.copy(saving = true), actionError = null) }
            repository.update(edit.grant.id, patch, edit.grant.revision).foldApiError(
                onSuccess = { _uiState.update { it.copy(edit = null) } },
                onError = { error ->
                    _uiState.update { it.copy(edit = null, actionError = error) }
                }
            )
            // Success or failure, the row may differ from what the editor started with.
            load()
        }
    }

    /** The minimal PATCH body, or null when nothing changed. */
    private fun buildPatch(edit: ShareGrantEditState): ShareGrantPatch? {
        val sections = edit.grant.sections.filter { it in edit.sections }
            .takeIf { it.toSet() != edit.grant.sections.toSet() }
        val patch = when (val choice = edit.expiry) {
            ExpiryChoice.Keep -> ShareGrantPatch(sections = sections)

            ExpiryChoice.None -> ShareGrantPatch(
                clearExpiry = edit.grant.expiresAt != null,
                sections = sections
            )

            is ExpiryChoice.Days -> ShareGrantPatch(
                expiresAt = now().plus(choice.days.toLong(), ChronoUnit.DAYS).toString(),
                sections = sections
            )
        }
        return patch.takeIf { it.sections != null || it.clearExpiry || it.expiresAt != null }
    }

    fun onActionErrorShown() {
        _uiState.update { it.copy(actionError = null) }
    }

    private fun runAction(call: suspend () -> Result<Any>) {
        if (_uiState.value.busy) return
        viewModelScope.launch {
            _uiState.update { it.copy(busy = true, actionError = null) }
            call().foldApiError(
                onSuccess = { _uiState.update { it.copy(busy = false, prompt = null) } },
                onError = { error ->
                    _uiState.update { it.copy(busy = false, prompt = null, actionError = error) }
                }
            )
            load()
        }
    }
}
