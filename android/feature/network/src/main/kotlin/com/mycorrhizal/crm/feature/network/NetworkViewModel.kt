package com.mycorrhizal.crm.feature.network

import androidx.annotation.StringRes
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.CircleWithMembers
import com.mycorrhizal.crm.domain.repository.GraphRepository
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.GraphChain
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

private const val DEFAULT_DEPTH = 2
private const val SEARCH_DEBOUNCE_MS = 300L

data class NetworkUiState(
    /** The starting contact's numeric id (null when started from the drawer without a self contact). */
    val fromContactId: Int? = null,
    /** The starting contact's VCardUID — the endpoint's `from` param. */
    val fromVCardUid: String = "",
    /** The starting contact's display name (resolved locally; the endpoint also echoes it). */
    val fromName: String = "",
    val depth: Int = DEFAULT_DEPTH,
    /** Live relation-filter input, applied (and sent) only via [NetworkViewModel.applyRelation]. */
    val relationInput: String = "",
    /** The relation token/synonym currently sent to the endpoint; null clears the filter. */
    val appliedRelation: String? = null,
    /** All circles (with members) for the client-side circle filter. */
    val circles: List<CircleWithMembers> = emptyList(),
    /** Selected circle id; null means "all circles" (no client-side filter). */
    val selectedCircleId: String? = null,
    /** Every chain the endpoint returned (before the circle filter). */
    val allChains: List<GraphChain> = emptyList(),
    val isLoading: Boolean = false,
    val error: String? = null,
    @StringRes val errorRes: Int? = null,
    // --- "Start from" contact picker ---
    val pickerOpen: Boolean = false,
    val contactSearchQuery: String = "",
    val contactSearchResults: List<ContactSummary> = emptyList(),
    val contactSearchLoading: Boolean = false,
) {
    /** Chains narrowed to the selected circle's members (client-side, mirroring web). */
    val filteredChains: List<GraphChain>
        get() {
            val circleId = selectedCircleId ?: return allChains
            val memberUids = circles.find { it.id == circleId }?.memberVCardUids ?: emptySet()
            return allChains.filter { it.targetVCardUid in memberUids }
        }

    /** [filteredChains] grouped by hop depth, for the section headers. */
    val groupedChains: Map<Int, List<GraphChain>>
        get() = filteredChains.groupBy { it.depth }

    /** True once a starting contact is chosen (drawer entry without a self contact prompts first). */
    val hasFrom: Boolean get() = fromVCardUid.isNotBlank()
}

@HiltViewModel
class NetworkViewModel @Inject constructor(
    private val graphRepository: GraphRepository,
    private val contactRepository: ContactRepository,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {

    // The route carries the starting contact only on the contact-detail entry
    // ("contacts/{id}/network"); the drawer route passes nothing, in which
    // case the screen defaults to the self contact. The value is read once —
    // it is a starting point, not something that changes.
    private val initialContactId: Int? = run {
        val raw: Any? = savedStateHandle["contactId"]
        (raw as? Int) ?: (raw as? String)?.toIntOrNull()
    }?.takeIf { it > 0 }

    private val _uiState = MutableStateFlow(NetworkUiState())
    val uiState: StateFlow<NetworkUiState> = _uiState.asStateFlow()

    private var searchJob: Job? = null
    // The in-flight traversal. Cancelled before every relaunch so a rapid
    // depth/relation/from change can't let an older response land last and
    // show a network for the previous settings (review-pass fix).
    private var connectionsJob: Job? = null

    init {
        load()
    }

    fun load() {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null, errorRes = null) }
            loadCircles()
            // The contact-detail entry always has a starting contact; the
            // drawer entry defaults to the self contact. A failed initial
            // resolution (no VCardUID) must NOT fall through to the self
            // contact — it has already surfaced an error.
            val from = if (initialContactId != null) {
                resolveInitialContact(initialContactId)
            } else {
                resolveSelfContact()
            }
            _uiState.update {
                it.copy(
                    fromContactId = from?.id,
                    fromVCardUid = from?.uid.orEmpty(),
                    fromName = from?.name.orEmpty(),
                )
            }
            loadConnections()
        }
    }

    /**
     * Resolves the contact-detail entry's numeric id to its VCardUID + name.
     * A contact with no uid (or a failed fetch) leaves the screen showing an
     * error rather than firing a doomed traversal.
     */
    private suspend fun resolveInitialContact(id: Int): FromContact? {
        val record = contactRepository.getContact(id).getOrNull()
        val uid = record?.card?.uid
        if (uid.isNullOrBlank()) {
            _uiState.update {
                it.copy(isLoading = false, errorRes = R.string.network_error_no_vcard_uid, error = null)
            }
            return null
        }
        return FromContact(id = id, uid = uid, name = record.card?.displayName.orEmpty())
    }

    /** Drawer entry: default to the self contact when one is set, else prompt the picker. */
    private suspend fun resolveSelfContact(): FromContact? {
        val uid = graphRepository.selfContactVCardUid().getOrNull()
        if (uid.isNullOrBlank()) return null
        val summary = contactRepository.resolveByUid(listOf(uid)).getOrNull()?.get(uid)
        return FromContact(id = summary?.id, uid = uid, name = summary?.displayName.orEmpty())
    }

    private suspend fun loadCircles() {
        graphRepository.circlesWithMembers().foldApiError(
            onSuccess = { circles ->
                _uiState.update { it.copy(circles = circles) }
            },
            // A circle-filter load failure must not block the graph itself —
            // the filter simply shows "All circles" only.
            onError = { _ -> },
        )
    }

    fun loadConnections() {
        val uid = _uiState.value.fromVCardUid
        if (uid.isBlank()) {
            _uiState.update { it.copy(isLoading = false) }
            return
        }
        // Cancel the previous traversal: only the response for the CURRENT
        // depth/relation/from may write to the list (review-pass fix for the
        // out-of-order race where the last-to-complete, not the last-issued,
        // request won).
        connectionsJob?.cancel()
        connectionsJob = viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null, errorRes = null) }
            val depth = _uiState.value.depth
            val relation = _uiState.value.appliedRelation
            graphRepository.getConnections(from = uid, depth = depth, relation = relation).foldApiError(
                onSuccess = { response ->
                    _uiState.update {
                        it.copy(
                            isLoading = false,
                            allChains = response.chainsOrEmpty,
                            // The endpoint resolves from_name authoritatively;
                            // prefer it when the local resolve came up empty.
                            fromName = it.fromName.ifBlank { response.fromName },
                        )
                    }
                },
                onError = { error ->
                    // Clear the stale chains so a failed depth/relation change
                    // can't keep showing the previous settings' list under the
                    // new chip/input (review-pass fix).
                    _uiState.update { it.copy(isLoading = false, allChains = emptyList(), error = error.displayMessage) }
                },
            )
        }
    }

    /** Depth is 1..3 (the design's mobile range — web's panel offers 1..5). */
    fun setDepth(depth: Int) {
        if (depth == _uiState.value.depth) return
        _uiState.update { it.copy(depth = depth) }
        loadConnections()
    }

    fun onRelationInputChange(value: String) {
        _uiState.update { it.copy(relationInput = value) }
    }

    /**
     * Applies the typed relation filter verbatim — a registry token or
     * synonym is passed straight through to the endpoint, never resolved
     * on-device (the server resolves it). Clearing the field removes the
     * filter. Re-applying the already-applied value is a no-op (no refetch).
     */
    fun applyRelation() {
        val value = _uiState.value.relationInput.trim()
        val applied = value.takeIf { it.isNotBlank() }
        if (applied == _uiState.value.appliedRelation) return
        _uiState.update { it.copy(appliedRelation = applied) }
        loadConnections()
    }

    fun selectCircle(circleId: String?) {
        _uiState.update { it.copy(selectedCircleId = circleId) }
    }

    // --- "Start from" picker ---

    fun openPicker() {
        if (_uiState.value.pickerOpen) return
        _uiState.update { it.copy(pickerOpen = true, contactSearchQuery = "", contactSearchResults = emptyList()) }
    }

    fun closePicker() {
        searchJob?.cancel()
        _uiState.update {
            it.copy(pickerOpen = false, contactSearchQuery = "", contactSearchResults = emptyList(), contactSearchLoading = false)
        }
    }

    /** Debounced contact search for the picker; the current starting contact is excluded. */
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
            val currentUid = _uiState.value.fromVCardUid
            contactRepository.listContacts(search = query, limit = 25).fold(
                onSuccess = { page ->
                    _uiState.update {
                        it.copy(
                            contactSearchResults = page.contacts.filter { c -> c.uid != currentUid },
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

    /** Switches the starting contact and re-runs the traversal. */
    fun selectFrom(contact: ContactSummary) {
        val uid = contact.uid
        if (uid.isNullOrBlank()) return
        searchJob?.cancel()
        _uiState.update {
            it.copy(
                pickerOpen = false,
                fromContactId = contact.id,
                fromVCardUid = uid,
                fromName = contact.displayName,
                contactSearchQuery = "",
                contactSearchResults = emptyList(),
                contactSearchLoading = false,
                selectedCircleId = null,
            )
        }
        loadConnections()
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null, errorRes = null) }
    }

    private data class FromContact(val id: Int?, val uid: String, val name: String)
}
