package com.mycorrhizal.crm.feature.relationships

import androidx.annotation.StringRes
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.RelationshipEdgeRepository
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.RelationshipEdge
import com.mycorrhizal.crm.model.network.RelationshipEdgeInput
import com.mycorrhizal.crm.model.network.RelationshipEdgeSensitivities
import com.mycorrhizal.crm.model.network.RelationshipEdgeStatuses
import com.mycorrhizal.crm.model.network.RelationshipEdgeTypes
import com.mycorrhizal.crm.model.network.ThinContactInput
import com.mycorrhizal.crm.network.foldApiError
import com.mycorrhizal.crm.ui.R
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class RelationshipsUiState(
    val contactVCardUid: String = "",
    val edges: List<RelationshipEdge> = emptyList(),
    /** Other-party VCardUID -> resolved summary, for name display/navigation. */
    val contactsByUid: Map<String, ContactSummary> = emptyMap(),
    val isLoading: Boolean = false,
    val acceptingId: String? = null,
    val deletingId: String? = null,
    val updatingId: String? = null,
    val contactSearchQuery: String = "",
    val contactSearchResults: List<ContactSummary> = emptyList(),
    val contactSearchLoading: Boolean = false,
    val error: String? = null,
    @StringRes val errorRes: Int? = null,
) {
    val confirmedEdges: List<RelationshipEdge>
        get() = edges.filter { it.status == RelationshipEdgeStatuses.CONFIRMED }

    val suggestedEdges: List<RelationshipEdge>
        get() = edges.filter { it.status == RelationshipEdgeStatuses.SUGGESTED }
}

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

    private var searchJob: Job? = null

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
                    resolveOtherParties(edges, uid)
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    /**
     * Best-effort name resolution for the other party of each edge. A
     * failure here (network, or a UID with no matching contact) is NOT
     * surfaced as the screen's primary error -- rows simply fall back to the
     * "unknown contact" display, degrading gracefully rather than blocking
     * or crashing the list that already loaded fine.
     */
    private fun resolveOtherParties(edges: List<RelationshipEdge>, viewedUid: String) {
        val otherUids = edges.map { otherPartyId(it, viewedUid) }.filter { it.isNotBlank() }.distinct()
        if (otherUids.isEmpty()) return
        viewModelScope.launch {
            contactRepository.resolveByUid(otherUids).onSuccess { resolved ->
                _uiState.update { it.copy(contactsByUid = it.contactsByUid + resolved) }
            }
        }
    }

    /**
     * Create an edge from this contact's page. The viewed contact is always
     * `target_id`; [type] is the dropdown token describing the OTHER party
     * (the source) relative to this contact (matches the web's create-mode
     * convention). Source may be an existing contact's VCardUID (linked
     * search) or a thin contact (name/gender/birthday) for parties not yet
     * in the CRM.
     */
    fun create(
        type: String,
        otherPartyVCardUid: String,
        otherPartyName: String,
        gender: String = "",
        birthday: String = "",
        sensitivity: String = RelationshipEdgeSensitivities.NORMAL,
        linkedContact: ContactSummary? = null,
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
                    ThinContactInput(
                        name = name,
                        gender = gender.trim().takeIf { it.isNotBlank() },
                        birthday = birthday.trim().takeIf { it.isNotBlank() },
                    )
                } else null,
                targetId = viewedUid,
                type = type,
                sensitivity = sensitivity,
            )
            relationshipEdgeRepository.create(input).foldApiError(
                onSuccess = { edge ->
                    val linkedEntry = linkedContact?.uid?.let { it to linkedContact }
                    _uiState.update {
                        val resolved = if (linkedEntry != null) it.contactsByUid + linkedEntry else it.contactsByUid
                        it.copy(edges = it.edges + edge, contactsByUid = resolved)
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(error = error.displayMessage) }
                },
            )
        }
    }

    /**
     * Update an existing edge's type/sensitivity. [type] is the dropdown
     * token describing the OTHER party (viewer-relative, same convention as
     * [create]/[effectiveType]) and is converted back to the backend's
     * source-relative `type` via [toBackendType]. The other party's identity
     * (source_id/target_id) is always resent verbatim -- see
     * RelationshipEdgeRepository.update's doc comment for why.
     */
    fun update(id: String, type: String, sensitivity: String) {
        if (_uiState.value.updatingId != null) return
        val edge = _uiState.value.edges.find { it.id == id } ?: return
        val viewedUid = _uiState.value.contactVCardUid
        val backendType = toBackendType(type, viewedIsSource = edge.sourceId == viewedUid)
        viewModelScope.launch {
            _uiState.update { it.copy(updatingId = id, error = null) }
            val input = RelationshipEdgeInput(
                sourceId = edge.sourceId,
                targetId = edge.targetId,
                type = backendType,
                sensitivity = sensitivity,
                metadata = edge.metadata,
            )
            relationshipEdgeRepository.update(id, input).foldApiError(
                onSuccess = { updated ->
                    _uiState.update { state ->
                        state.copy(
                            updatingId = null,
                            edges = state.edges.map { if (it.id == id) updated else it },
                        )
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(updatingId = null, error = error.displayMessage) }
                },
            )
        }
    }

    /**
     * Debounced contact search for the create dialog's linked-entry mode.
     * Cancel-and-relaunch, mirroring ContactListViewModel's search debounce.
     */
    fun searchContacts(query: String) {
        _uiState.update { it.copy(contactSearchQuery = query) }
        searchJob?.cancel()
        if (query.isBlank()) {
            _uiState.update { it.copy(contactSearchResults = emptyList(), contactSearchLoading = false) }
            return
        }
        searchJob = viewModelScope.launch {
            delay(SEARCH_DEBOUNCE_MS)
            _uiState.update { it.copy(contactSearchLoading = true) }
            val viewedUid = _uiState.value.contactVCardUid
            contactRepository.listContacts(search = query, limit = 25).fold(
                onSuccess = { page ->
                    _uiState.update {
                        it.copy(
                            contactSearchResults = page.contacts.filter { c -> c.uid != viewedUid },
                            contactSearchLoading = false,
                        )
                    }
                },
                onFailure = {
                    _uiState.update { it.copy(contactSearchLoading = false) }
                },
            )
        }
    }

    fun clearContactSearch() {
        searchJob?.cancel()
        _uiState.update { it.copy(contactSearchQuery = "", contactSearchResults = emptyList(), contactSearchLoading = false) }
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

    /** Deletes an edge. For a suggested edge, this is also how "reject" works server-side. */
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

    private companion object {
        const val SEARCH_DEBOUNCE_MS = 300L
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

/**
 * Converts a type-dropdown pick (always framed as "what is the OTHER party
 * to me", matching [effectiveType]) back into the backend's `type` value
 * (always "what is SourceID to TargetID"), given which side the viewed
 * contact occupies. Create mode: the viewed contact is always TargetID, so
 * this is the identity function there. Only matters in edit mode, when the
 * viewed contact may be SourceID (editing an edge originally created from
 * the OTHER party's page). Mirrors web's toBackendType.
 */
fun toBackendType(dropdownToken: String, viewedIsSource: Boolean): String {
    if (!viewedIsSource) return dropdownToken
    val meta = RelationshipEdgeTypes.ALL[dropdownToken] ?: return RelationshipEdgeTypes.RELATED_TO
    return meta.inverse
}
