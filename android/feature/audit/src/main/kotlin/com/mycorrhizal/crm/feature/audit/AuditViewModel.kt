package com.mycorrhizal.crm.feature.audit

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.AuditRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.model.network.AuditEvent
import com.mycorrhizal.crm.model.network.AuditEntityTypes
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.foldApiError
import com.mycorrhizal.crm.network.toApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.Job
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

/**
 * The fetch window grows in [DEFAULT_LIMIT] steps up to [MAX_LIMIT] — the
 * backend caps `?limit=` at 500, and the API has no cursor, so "load more"
 * re-fetches with a larger window (mirrors web's useAudit).
 */
private const val DEFAULT_LIMIT = 100
private const val MAX_LIMIT = 500
private const val ENTITY_ID_DEBOUNCE_MS = 350L

data class AuditUiState(
    val events: List<AuditEvent> = emptyList(),
    val isLoading: Boolean = false,
    /** Transient list-load failure (toasted). Cleared at the start of every fetch. */
    val error: String? = null,
    /** The applied entity-type filter; null = all types. */
    val entityType: String? = null,
    /** The debounced, applied entity-id filter (empty = no filter). */
    val entityId: String = "",
    /** The fetch window. The API has no cursor, so "load more" grows this to 500. */
    val limit: Int = DEFAULT_LIMIT,
    val isUndoing: Boolean = false,
    /**
     * Resolved contact summaries keyed by Contact.VCardUID for the loaded
     * contact events — the row's entity-id cell links to the contact detail
     * when its UID resolves (web: getContactsByUid). A UID that doesn't
     * resolve (deleted contact) renders as plain text.
     */
    val contactsByUid: Map<String, ContactSummary> = emptyMap(),
) {
    /**
     * The window only ever returns up to [limit] rows, so "a full window means
     * there might be more" is the only signal available (mirrors useAudit).
     */
    val canLoadMore: Boolean
        get() = events.size >= limit && limit < MAX_LIMIT

    val hasActiveFilters: Boolean
        get() = entityType != null || entityId.isNotBlank()
}

sealed interface AuditUiEvent {
    /** Undo succeeded; the list has been refreshed. */
    data object UndoSucceeded : AuditUiEvent

    /**
     * Undo failed. [isRetentionGone] is true when the backend answered 410 —
     * the event aged past AUDIT_RETENTION_DAYS and the purge removed it, so
     * there is nothing left to undo (web's audit.undo.retentionGone). Other
     * failures carry the server's own message (400 delete/unsupported entity,
     * 404 gone).
     */
    data class UndoFailed(val isRetentionGone: Boolean, val message: String) : AuditUiEvent
}

@HiltViewModel
class AuditViewModel @Inject constructor(
    private val auditRepository: AuditRepository,
    private val contactRepository: ContactRepository,
) : ViewModel() {

    private val _uiState = MutableStateFlow(AuditUiState())
    val uiState: StateFlow<AuditUiState> = _uiState.asStateFlow()

    private val _events = Channel<AuditUiEvent>(Channel.BUFFERED)
    val events = _events.receiveAsFlow()

    private var entityIdDebounceJob: Job? = null

    /** Monotonic guard against out-of-order responses when filters change mid-fetch. */
    private var requestRef = 0

    init {
        load()
    }

    /**
     * Fetch the newest [AuditUiState.limit] rows matching the current filters,
     * then resolve any contact UIDs among them to display names/IDs for the
     * linkable entity-id cells. Guards against out-of-order responses when a
     * filter change races an in-flight fetch (same pattern as useAudit).
     */
    fun load() {
        viewModelScope.launch {
            val requestId = ++requestRef
            _uiState.update { it.copy(isLoading = true, error = null) }
            val state = _uiState.value
            val result = auditRepository.list(
                entityType = state.entityType,
                entityId = state.entityId.takeIf { it.isNotBlank() },
                limit = state.limit,
            )
            if (requestRef != requestId) return@launch
            result.foldApiError(
                onSuccess = { response ->
                    _uiState.update {
                        it.copy(isLoading = false, events = response.auditEvents)
                    }
                },
                onError = { error ->
                    _uiState.update {
                        it.copy(isLoading = false, error = error.displayMessage)
                    }
                },
            )
            resolveContactUids()
        }
    }

    /** Grow the window by [DEFAULT_LIMIT] (to the 500 cap) and re-fetch. */
    fun loadMore() {
        val state = _uiState.value
        if (!state.canLoadMore || state.isLoading) return
        _uiState.update { it.copy(limit = (it.limit + DEFAULT_LIMIT).coerceAtMost(MAX_LIMIT)) }
        load()
    }

    /**
     * A changed filter restarts the window at the default limit — the API
     * only returns the newest `limit` rows, so a grown window must not mask
     * the filtered result (mirrors useAudit's applyEntityType).
     */
    fun applyEntityType(type: String?) {
        val next = type?.takeIf { it != "" }
        if (next == _uiState.value.entityType) return
        _uiState.update { it.copy(entityType = next, limit = DEFAULT_LIMIT) }
        load()
    }

    /**
     * The entity-id field filters server-side but is debounced so every
     * keystroke doesn't fire a request (web's useDebouncedValue + 350ms).
     */
    fun onEntityIdChange(raw: String) {
        entityIdDebounceJob?.cancel()
        entityIdDebounceJob = viewModelScope.launch {
            delay(ENTITY_ID_DEBOUNCE_MS)
            val trimmed = raw.trim()
            if (trimmed == _uiState.value.entityId) return@launch
            _uiState.update { it.copy(entityId = trimmed, limit = DEFAULT_LIMIT) }
            load()
        }
    }

    fun clearFilters() {
        entityIdDebounceJob?.cancel()
        if (!_uiState.value.hasActiveFilters) return
        _uiState.update { it.copy(entityType = null, entityId = "", limit = DEFAULT_LIMIT) }
        load()
    }

    /**
     * Revert an update event via POST /audit/:id/undo, then refresh so the
     * list reflects the restored state. Outcomes are emitted as [AuditUiEvent]
     * so the screen can pick the exact user-facing string (410 → retention
     * message, everything else → the server's own message).
     */
    fun undo(id: Long) {
        if (_uiState.value.isUndoing) return
        viewModelScope.launch {
            _uiState.update { it.copy(isUndoing = true) }
            val result = auditRepository.undo(id)
            _uiState.update { it.copy(isUndoing = false) }
            val outcome = result.fold(
                onSuccess = { AuditUiEvent.UndoSucceeded },
                onFailure = { raw ->
                    val error = raw.toApiError()
                    val retentionGone = error is ApiError.Client && error.code == 410
                    val message = if (retentionGone) {
                        error.body.ifBlank { "HTTP 410" }
                    } else {
                        error.displayMessage
                    }
                    AuditUiEvent.UndoFailed(retentionGone, message)
                },
            )
            _events.send(outcome)
            if (outcome is AuditUiEvent.UndoSucceeded) load()
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null) }
    }

    /**
     * Resolve the loaded events' contact UIDs to display summaries (the
     * entity-id cell's link target). A failure is a nicety — the raw UID is
     * still rendered — so it degrades silently rather than surfacing an error
     * (web: useContactsForEvents swallows its failure too).
     */
    private suspend fun resolveContactUids() {
        val uids = _uiState.value.events
            .filter { it.entityType == AuditEntityTypes.CONTACT }
            .mapNotNull { it.entityId.takeIf { uid -> uid.isNotBlank() } }
            .distinct()
        if (uids.isEmpty()) {
            _uiState.update { it.copy(contactsByUid = emptyMap()) }
            return
        }
        contactRepository.resolveByUid(uids).foldApiError(
            onSuccess = { byUid ->
                _uiState.update { it.copy(contactsByUid = byUid) }
            },
            onError = { _uiState.update { it.copy(contactsByUid = emptyMap()) } },
        )
    }
}
