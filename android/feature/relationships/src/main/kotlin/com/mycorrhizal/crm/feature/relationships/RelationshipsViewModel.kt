package com.mycorrhizal.crm.feature.relationships

import androidx.annotation.StringRes
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.RelationshipEdgeRepository
import com.mycorrhizal.crm.model.network.RelationshipEdge
import com.mycorrhizal.crm.model.network.RelationshipEdgeInput
import com.mycorrhizal.crm.model.network.RelationshipEdgeStatuses
import com.mycorrhizal.crm.model.network.RelationshipEdgeTypes
import com.mycorrhizal.crm.network.foldApiError
import com.mycorrhizal.crm.ui.R
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class RelationshipsUiState(
    val contactVCardUid: String = "",
    val edges: List<RelationshipEdge> = emptyList(),
    val isLoading: Boolean = false,
    val acceptingId: String? = null,
    val deletingId: String? = null,
    val error: String? = null,
    @StringRes val errorRes: Int? = null,
)

@HiltViewModel
class RelationshipsViewModel @Inject constructor(
    private val relationshipEdgeRepository: RelationshipEdgeRepository,
    private val contactRepository: ContactRepository,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {

    private val contactId: Int = run {
        val raw: Any? = savedStateHandle["contactId"]
        (raw as? Int) ?: (raw as? String)?.toIntOrNull() ?: 0
    }

    private val _uiState = MutableStateFlow(RelationshipsUiState())
    val uiState: StateFlow<RelationshipsUiState> = _uiState.asStateFlow()

    init {
        load()
    }

    fun load() {
        if (contactId == 0) {
            _uiState.update {
                it.copy(isLoading = false, errorRes = R.string.relationships_error_missing_contact_id, error = null)
            }
            return
        }
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null, errorRes = null) }
            // The graph invariants use the contact's VCardUID, so resolve it
            // from the record before listing edges.
            val uid = contactRepository.getContact(contactId).getOrNull()?.card?.uid
            if (uid.isNullOrBlank()) {
                _uiState.update {
                    it.copy(isLoading = false, errorRes = R.string.relationships_error_no_vcard_uid, error = null)
                }
                return@launch
            }
            _uiState.update { it.copy(contactVCardUid = uid) }
            relationshipEdgeRepository.listForContact(uid).foldApiError(
                onSuccess = { edges ->
                    _uiState.update { it.copy(isLoading = false, edges = edges) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    /**
     * Create an edge from this contact's page. The viewed contact is always
     * `target_id`; [type] is the dropdown token describing the OTHER party
     * (the source) relative to this contact (matches the web's create-mode
     * convention). Source may be an existing contact's VCardUID or a thin
     * contact (name) for parties not yet in the CRM.
     */
    fun create(
        type: String,
        otherPartyVCardUid: String,
        otherPartyName: String,
    ) {
        val uid = otherPartyVCardUid.trim()
        val name = otherPartyName.trim()
        val viewedUid = _uiState.value.contactVCardUid
        if (uid.isEmpty() && name.isEmpty()) return
        if (viewedUid.isBlank()) return
        viewModelScope.launch {
            _uiState.update { it.copy(error = null) }
            val input = RelationshipEdgeInput(
                sourceId = uid.takeIf { it.isNotBlank() },
                sourceThin = if (uid.isBlank() && name.isNotBlank()) {
                    com.mycorrhizal.crm.model.network.ThinContactInput(name = name)
                } else null,
                targetId = viewedUid,
                type = type,
            )
            relationshipEdgeRepository.create(input).foldApiError(
                onSuccess = { edge ->
                    _uiState.update { it.copy(edges = it.edges + edge) }
                },
                onError = { error ->
                    _uiState.update { it.copy(error = error.displayMessage) }
                },
            )
        }
    }

    fun accept(id: String) {
        if (_uiState.value.acceptingId != null) return
        viewModelScope.launch {
            _uiState.update { it.copy(acceptingId = id) }
            relationshipEdgeRepository.accept(id).foldApiError(
                onSuccess = { edge ->
                    _uiState.update { state ->
                        state.copy(
                            acceptingId = null,
                            edges = state.edges.map { if (it.id == id) edge else it },
                        )
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(acceptingId = null, error = error.displayMessage) }
                },
            )
        }
    }

    fun delete(id: String) {
        if (_uiState.value.deletingId != null) return
        viewModelScope.launch {
            _uiState.update { it.copy(deletingId = id) }
            relationshipEdgeRepository.delete(id).foldApiError(
                onSuccess = {
                    _uiState.update { state ->
                        state.copy(deletingId = null, edges = state.edges.filterNot { it.id == id })
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(deletingId = null, error = error.displayMessage) }
                },
            )
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null, errorRes = null) }
    }
}

/**
 * Effective relation token as read from the viewed contact's perspective
 * (mirrors the web's getEffectiveType): if the viewed contact is the edge's
 * source, the other party's role is the inverse of `type`.
 */
fun effectiveType(edge: RelationshipEdge, viewedUid: String): String {
    val meta = RelationshipEdgeTypes.ALL[edge.type] ?: return RelationshipEdgeTypes.RELATED_TO
    return if (edge.sourceId == viewedUid) meta.inverse else edge.type
}

/** The other endpoint's VCardUID — whichever of source/target isn't the viewed contact. */
fun otherPartyId(edge: RelationshipEdge, viewedUid: String): String =
    if (edge.sourceId == viewedUid) edge.targetId else edge.sourceId
