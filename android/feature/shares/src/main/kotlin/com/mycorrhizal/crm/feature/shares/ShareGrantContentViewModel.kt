package com.mycorrhizal.crm.feature.shares

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ShareGrantRepository
import com.mycorrhizal.crm.model.network.ErrorReasonCode
import com.mycorrhizal.crm.model.network.ShareGrantContent
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class ShareGrantContentUiState(
    val isLoading: Boolean = false,
    val content: ShareGrantContent? = null,
    /** The projected contact for a contact grant; null for the other entity types. */
    val contact: SharedContactView? = null,
    /** A failed read the user can retry. Never accompanied by content. */
    val error: ApiError? = null,
    /** The grant is gone (revoked, expired, entity deleted): the same 404 for every cause. */
    val unavailable: Boolean = false
)

/**
 * The recipient read view (ADR 0042 decision 12): one `GET /share-grants/{id}/content` per load.
 * The payload lives only in this ViewModel's memory (it is never written to a saved-state bundle,
 * Room or disk) and every load clears it first, so a failed or offline read shows a retryable
 * error and a revoked grant shows "unavailable" — never the previous content.
 */
@HiltViewModel
class ShareGrantContentViewModel @Inject constructor(
    private val repository: ShareGrantRepository,
    savedStateHandle: SavedStateHandle
) : ViewModel() {

    private val grantId: String = (savedStateHandle["grantId"] as? String).orEmpty()

    /**
     * The grant's own section list, passed from the list row. A blank value means "unknown", which
     * is treated as the narrowest reading (identity only), never as "everything".
     */
    private val grantedSections: Set<String> =
        (savedStateHandle["sections"] as? String).orEmpty()
            .split(',').map { it.trim() }.filter { it.isNotEmpty() }.toSet()

    private val _uiState = MutableStateFlow(ShareGrantContentUiState())
    val uiState: StateFlow<ShareGrantContentUiState> = _uiState.asStateFlow()

    private var dateFormatter: (Int?, Int?, Int?) -> String = ::isoDate

    init {
        load()
    }

    /** The screen supplies the user's date preference; it only affects rendering. */
    fun setDateFormatter(formatter: (Int?, Int?, Int?) -> String) {
        dateFormatter = formatter
        _uiState.value.content?.let { content ->
            _uiState.update { it.copy(contact = project(content)) }
        }
    }

    fun load() {
        if (grantId.isBlank()) {
            _uiState.value = ShareGrantContentUiState(unavailable = true)
            return
        }
        if (_uiState.value.isLoading) return
        viewModelScope.launch {
            _uiState.value = ShareGrantContentUiState(isLoading = true)
            repository.content(grantId).foldApiError(
                onSuccess = { content ->
                    _uiState.value = ShareGrantContentUiState(
                        content = content,
                        contact = project(content)
                    )
                },
                onError = { error ->
                    _uiState.value = if (error.isGrantGone()) {
                        ShareGrantContentUiState(unavailable = true)
                    } else {
                        ShareGrantContentUiState(error = error)
                    }
                }
            )
        }
    }

    private fun project(content: ShareGrantContent): SharedContactView? =
        content.contact?.let { projectSharedContact(it, grantedSections, dateFormatter) }

    private fun ApiError.isGrantGone(): Boolean = this is ApiError.Client &&
        (reason?.code == ErrorReasonCode.SHARE_GRANT_NOT_FOUND || code == NOT_FOUND)

    private companion object {
        const val NOT_FOUND = 404
    }
}
