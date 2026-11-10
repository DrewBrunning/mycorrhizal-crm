package com.mycorrhizal.crm.feature.shares

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ShareGrantRepository
import com.mycorrhizal.crm.model.network.IncomingShareGrant
import com.mycorrhizal.crm.model.network.ShareBlock
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.foldApiError
import com.mycorrhizal.crm.network.toApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** A confirmation the user must accept before a destructive recipient action runs. */
sealed interface SharedWithMePrompt {
    val grant: IncomingShareGrant

    /** Decline a pending grant (the sender can see it was declined). */
    data class Decline(override val grant: IncomingShareGrant) : SharedWithMePrompt

    /** Decline and block the sender: ends all their grants and hides future ones. */
    data class Block(override val grant: IncomingShareGrant) : SharedWithMePrompt

    /** Stop receiving a pending or active grant. */
    data class Leave(override val grant: IncomingShareGrant) : SharedWithMePrompt
}

data class SharedWithMeUiState(
    val grants: List<IncomingShareGrant> = emptyList(),
    val blocks: List<ShareBlock> = emptyList(),
    val isLoading: Boolean = false,
    /** A failed load: the screen shows a retry, never previously loaded rows. */
    val loadError: ApiError? = null,
    /** A failed action, shown once as a snackbar. */
    val actionError: ApiError? = null,
    val prompt: SharedWithMePrompt? = null,
    val busy: Boolean = false
)

/**
 * ADR 0042 decisions 7-8 (recipient side): the "Shared with me" list with accept, decline, leave,
 * block and unblock. Nothing is accepted implicitly; destructive actions are confirmation-gated;
 * state lives only in memory and a failed reload drops the rows instead of keeping stale ones.
 */
@HiltViewModel
class SharedWithMeViewModel @Inject constructor(private val repository: ShareGrantRepository) :
    ViewModel() {

    private val _uiState = MutableStateFlow(SharedWithMeUiState())
    val uiState: StateFlow<SharedWithMeUiState> = _uiState.asStateFlow()

    init {
        load()
    }

    fun load() {
        if (_uiState.value.isLoading) return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, loadError = null) }
            val incoming = repository.listIncoming()
            val blocks = repository.blocks()
            _uiState.update {
                it.copy(
                    isLoading = false,
                    grants = incoming.getOrNull()?.shareGrants.orEmpty(),
                    blocks = blocks.getOrNull().orEmpty(),
                    loadError = (incoming.exceptionOrNull() ?: blocks.exceptionOrNull())?.toApiError()
                )
            }
        }
    }

    fun accept(grant: IncomingShareGrant) = runAction { repository.accept(grant.id) }

    fun requestDecline(grant: IncomingShareGrant) = openPrompt(SharedWithMePrompt.Decline(grant))

    fun requestBlock(grant: IncomingShareGrant) = openPrompt(SharedWithMePrompt.Block(grant))

    fun requestLeave(grant: IncomingShareGrant) = openPrompt(SharedWithMePrompt.Leave(grant))

    private fun openPrompt(prompt: SharedWithMePrompt) {
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
                is SharedWithMePrompt.Decline -> repository.decline(prompt.grant.id, block = false)
                is SharedWithMePrompt.Block -> repository.decline(prompt.grant.id, block = true)
                is SharedWithMePrompt.Leave -> repository.leave(prompt.grant.id)
            }
        }
    }

    fun unblock(block: ShareBlock) = runAction { repository.unblock(block.userId) }

    fun onActionErrorShown() {
        _uiState.update { it.copy(actionError = null) }
    }

    /** Runs one mutation, then always reloads: the row may have changed under us. */
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
