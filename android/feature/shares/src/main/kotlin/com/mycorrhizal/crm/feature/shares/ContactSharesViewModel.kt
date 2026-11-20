package com.mycorrhizal.crm.feature.shares

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ContactShareRepository
import com.mycorrhizal.crm.model.network.ContactShare
import com.mycorrhizal.crm.model.network.ImportPreviewResponse
import com.mycorrhizal.crm.model.network.ImportRowPreview
import com.mycorrhizal.crm.model.network.RowImportAction
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

enum class SharesTab { INCOMING, OUTGOING }

data class ContactSharesUiState(
    val incoming: List<ContactShare> = emptyList(),
    val outgoing: List<ContactShare> = emptyList(),
    /** Other party's user ID (stringified) -> username, merged from both lists. */
    val usernames: Map<String, String> = emptyMap(),
    val isLoading: Boolean = false,
    val error: ApiError? = null,
    val selectedTab: SharesTab = SharesTab.INCOMING,
    // --- Accept preview flow (M15: accept is preview-then-confirm, never a one-tap button) ---
    /** The share whose preview is currently open; null when no accept flow is active. */
    val acceptingShare: ContactShare? = null,
    val previewLoading: Boolean = false,
    val previewError: ApiError? = null,
    val preview: ImportPreviewResponse? = null,
    /** Per-row action for row 0 (a share is always exactly one contact). */
    val confirmAction: String = "add",
    val confirming: Boolean = false,
    // --- Decline confirmation gate ---
    /** The pending share awaiting the user's decline confirmation; null when no dialog is up. */
    val declinePendingId: String? = null,
    val declining: Boolean = false,
    /** Non-null while the selected tab has another page. */
    val incomingNextCursor: String? = null,
    val outgoingNextCursor: String? = null,
    val loadingMore: Boolean = false,
    val loadMoreError: ApiError? = null
) {
    val row: ImportRowPreview? get() = preview?.rows?.firstOrNull()
}

@HiltViewModel
class ContactSharesViewModel @Inject constructor(
    private val contactShareRepository: ContactShareRepository
) : ViewModel() {

    private val _uiState = MutableStateFlow(ContactSharesUiState())
    val uiState: StateFlow<ContactSharesUiState> = _uiState.asStateFlow()

    init {
        load()
    }

    fun load() {
        if (_uiState.value.isLoading) return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            val incomingResult = contactShareRepository.listIncoming()
            val outgoingResult = contactShareRepository.listOutgoing()
            val incoming = incomingResult.getOrNull()
            val outgoing = outgoingResult.getOrNull()
            val errorMessage = when {
                incoming == null && outgoing == null ->
                    (incomingResult.exceptionOrNull() ?: outgoingResult.exceptionOrNull())
                        ?.toApiError() ?: ApiError.Unknown(
                        IllegalStateException("Failed to load shares")
                    )

                incoming == null -> incomingResult.exceptionOrNull()?.toApiError()

                outgoing == null -> outgoingResult.exceptionOrNull()?.toApiError()

                else -> null
            }
            _uiState.update {
                it.copy(
                    isLoading = false,
                    incoming = incoming?.contactShares ?: emptyList(),
                    outgoing = outgoing?.contactShares ?: emptyList(),
                    usernames = mergeUsernames(incoming?.usernames, outgoing?.usernames),
                    error = errorMessage,
                    incomingNextCursor = incoming?.nextCursor?.takeIf { it.isNotBlank() },
                    outgoingNextCursor = outgoing?.nextCursor?.takeIf { it.isNotBlank() },
                    loadMoreError = null
                )
            }
        }
    }

    /** Appends the next page of the currently selected tab; the backend caps a page at 100. */
    fun loadMore() {
        if (_uiState.value.isLoading || _uiState.value.loadingMore) return
        val tab = _uiState.value.selectedTab
        val cursor =
            if (tab == SharesTab.INCOMING) {
                _uiState.value.incomingNextCursor
            } else {
                _uiState.value.outgoingNextCursor
            }
        if (cursor == null) return
        viewModelScope.launch {
            _uiState.update { it.copy(loadingMore = true, loadMoreError = null) }
            val result =
                if (tab == SharesTab.INCOMING) {
                    contactShareRepository.listIncoming(cursor)
                } else {
                    contactShareRepository.listOutgoing(cursor)
                }
            result.foldApiError(
                onSuccess = { page ->
                    _uiState.update {
                        if (tab == SharesTab.INCOMING) {
                            it.copy(
                                loadingMore = false,
                                incoming = it.incoming + page.contactShares,
                                usernames = mergeUsernames(it.usernames, page.usernames),
                                incomingNextCursor = page.nextCursor?.takeIf { next ->
                                    next.isNotBlank()
                                }
                            )
                        } else {
                            it.copy(
                                loadingMore = false,
                                outgoing = it.outgoing + page.contactShares,
                                usernames = mergeUsernames(it.usernames, page.usernames),
                                outgoingNextCursor = page.nextCursor?.takeIf { next ->
                                    next.isNotBlank()
                                }
                            )
                        }
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(loadingMore = false, loadMoreError = error) }
                }
            )
        }
    }

    fun selectTab(tab: SharesTab) {
        // A load-more failure belongs to the tab it happened on; don't carry
        // its message onto the other tab's list.
        _uiState.update { it.copy(selectedTab = tab, loadMoreError = null) }
    }

    // --- Accept flow ---

    /**
     * Open the accept flow for an incoming share: fetch the preview (does NOT
     * change the share's status — the backend's accept is preview-only) and
     * suggest the row's default action.
     */
    fun openAccept(share: ContactShare) {
        if (_uiState.value.acceptingShare != null) return
        _uiState.update {
            it.copy(
                acceptingShare = share,
                preview = null,
                previewError = null,
                previewLoading = true,
                confirmAction = "add",
                confirming = false
            )
        }
        viewModelScope.launch {
            contactShareRepository.accept(share.id).foldApiError(
                onSuccess = { preview ->
                    val row = preview.rows.firstOrNull()
                    _uiState.update {
                        it.copy(
                            previewLoading = false,
                            preview = preview,
                            confirmAction =
                                row?.suggestedAction?.takeIf { a -> a == "update" || a == "skip" }
                                    ?: "add"
                        )
                    }
                },
                onError = { error ->
                    _uiState.update {
                        it.copy(
                            previewLoading = false,
                            previewError = error
                        )
                    }
                }
            )
        }
    }

    fun setConfirmAction(action: String) {
        _uiState.update { it.copy(confirmAction = action) }
    }

    /** Finalize the accepted share with the recipient's chosen per-row action. */
    fun confirmAccept() {
        val share = _uiState.value.acceptingShare ?: return
        val preview = _uiState.value.preview ?: return
        val row = preview.rows.firstOrNull() ?: return
        if (_uiState.value.confirming) return
        viewModelScope.launch {
            _uiState.update { it.copy(confirming = true, previewError = null) }
            contactShareRepository.confirm(
                share.id,
                preview.sessionId,
                listOf(
                    RowImportAction(rowIndex = row.rowIndex, action = _uiState.value.confirmAction)
                )
            ).foldApiError(
                onSuccess = {
                    _uiState.update {
                        it.copy(
                            confirming = false,
                            acceptingShare = null,
                            preview = null,
                            confirmAction = "add"
                        )
                    }
                    load()
                },
                onError = { error ->
                    _uiState.update {
                        it.copy(
                            confirming = false,
                            previewError = error
                        )
                    }
                }
            )
        }
    }

    fun closeAccept() {
        if (_uiState.value.confirming) return
        _uiState.update {
            it.copy(
                acceptingShare = null,
                preview = null,
                previewError = null,
                previewLoading = false
            )
        }
    }

    // --- Decline flow ---

    /**
     * Request to decline a pending share. This only opens the confirmation
     * dialog — the repository is NOT called until [confirmDecline], so a
     * mis-tap never permanently declines a share.
     */
    fun requestDecline(share: ContactShare) {
        if (_uiState.value.declinePendingId != null) return
        _uiState.update { it.copy(declinePendingId = share.id) }
    }

    fun cancelDecline() {
        _uiState.update { it.copy(declinePendingId = null) }
    }

    fun confirmDecline() {
        val id = _uiState.value.declinePendingId ?: return
        if (_uiState.value.declining) return
        viewModelScope.launch {
            _uiState.update { it.copy(declining = true) }
            contactShareRepository.decline(id).foldApiError(
                onSuccess = {
                    _uiState.update { it.copy(declining = false, declinePendingId = null) }
                    load()
                },
                onError = { error ->
                    _uiState.update {
                        it.copy(
                            declining = false,
                            declinePendingId = null,
                            error = error
                        )
                    }
                }
            )
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null) }
    }

    private fun mergeUsernames(vararg maps: Map<String, String>?): Map<String, String> {
        val merged = mutableMapOf<String, String>()
        maps.forEach { m -> m?.let { merged.putAll(it) } }
        return merged
    }
}
