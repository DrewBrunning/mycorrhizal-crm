package com.mycorrhizal.crm.feature.sysevents

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.SystemEventRepository
import com.mycorrhizal.crm.model.network.ErrorBucket
import com.mycorrhizal.crm.model.network.SubsystemHealth
import com.mycorrhizal.crm.model.network.SystemEvent
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

/**
 * The fetch window grows in [DEFAULT_LIMIT] steps up to [MAX_LIMIT] — the
 * backend caps `?limit=` at 500 and has no cursor, so "load more" re-fetches
 * with a larger window (mirrors web's useSystemEvents).
 */
private const val DEFAULT_LIMIT = 100
private const val MAX_LIMIT = 500
private const val CORRELATION_DEBOUNCE_MS = 350L

data class SystemEventsUiState(
    val events: List<SystemEvent> = emptyList(),
    /**
     * Per-subsystem last-known-good state (issue #427), shown above the
     * timeline. Loaded once on open and on an explicit refresh — it does not
     * re-fetch on a filter change. A load failure leaves it empty rather than
     * raising the list's loud error.
     */
    val subsystemHealth: List<SubsystemHealth> = emptyList(),
    /**
     * Operational failures over the last 24h bucketed by cause (issue #426),
     * shown above the timeline. Same load policy as [subsystemHealth]: fetched
     * on open and on explicit refresh, a failure leaves it empty.
     */
    val errorBuckets: List<ErrorBucket> = emptyList(),
    val isLoading: Boolean = false,
    /** Transient list-load failure (toasted). Cleared at the start of every fetch. */
    val error: String? = null,
    /** Applied component filter; null = all. */
    val component: String? = null,
    /** Applied severity filter; null = all. */
    val severity: String? = null,
    /** Applied event-type filter; null = all. */
    val eventType: String? = null,
    /** Applied, debounced correlation-id filter (empty = no filter). */
    val correlationId: String = "",
    /**
     * Exact-row drill-down from an error-aggregation bucket (issue #426). When
     * non-empty it is used alone — every other mutator clears it.
     */
    val eventIds: List<Long> = emptyList(),
    val limit: Int = DEFAULT_LIMIT,
) {
    val canLoadMore: Boolean
        get() = events.size >= limit && limit < MAX_LIMIT

    val hasActiveFilters: Boolean
        get() = component != null || severity != null || eventType != null ||
            correlationId.isNotBlank() || eventIds.isNotEmpty()
}

@HiltViewModel
class SystemEventsViewModel @Inject constructor(
    private val repository: SystemEventRepository,
) : ViewModel() {

    private val _uiState = MutableStateFlow(SystemEventsUiState())
    val uiState: StateFlow<SystemEventsUiState> = _uiState.asStateFlow()

    private var correlationDebounceJob: Job? = null

    /** Monotonic guard against out-of-order responses when filters change mid-fetch. */
    private var requestRef = 0

    init {
        load()
        refreshSubsystemHealth()
        refreshErrorAggregation()
    }

    /**
     * Reload the per-subsystem health panel (issue #427). Best-effort: a
     * failure is swallowed so the panel just stays as it was — the timeline's
     * own error is the one worth shouting about.
     */
    fun refreshSubsystemHealth() {
        viewModelScope.launch {
            repository.subsystemHealth().foldApiError(
                onSuccess = { response ->
                    _uiState.update { it.copy(subsystemHealth = response.subsystems) }
                },
                onError = { },
            )
        }
    }

    /**
     * Reload the error-aggregation panel (issue #426). Best-effort, same as
     * [refreshSubsystemHealth] — a failure leaves the panel as it was.
     */
    fun refreshErrorAggregation() {
        viewModelScope.launch {
            repository.errorAggregation().foldApiError(
                onSuccess = { response ->
                    _uiState.update { it.copy(errorBuckets = response.buckets) }
                },
                onError = { },
            )
        }
    }

    fun load() {
        viewModelScope.launch {
            val requestId = ++requestRef
            _uiState.update { it.copy(isLoading = true, error = null) }
            val state = _uiState.value
            val result = repository.list(
                component = state.component,
                severity = state.severity,
                eventType = state.eventType,
                correlationId = state.correlationId.takeIf { it.isNotBlank() },
                ids = state.eventIds.takeIf { it.isNotEmpty() },
                limit = state.limit,
            )
            if (requestRef != requestId) return@launch
            result.foldApiError(
                onSuccess = { response ->
                    _uiState.update { it.copy(isLoading = false, events = response.systemEvents) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun loadMore() {
        val state = _uiState.value
        if (!state.canLoadMore || state.isLoading) return
        _uiState.update { it.copy(limit = (it.limit + DEFAULT_LIMIT).coerceAtMost(MAX_LIMIT)) }
        load()
    }

    fun applyComponent(value: String?) {
        val next = value?.takeIf { it.isNotBlank() }
        if (next == _uiState.value.component && _uiState.value.eventIds.isEmpty()) return
        _uiState.update { it.copy(component = next, eventIds = emptyList(), limit = DEFAULT_LIMIT) }
        load()
    }

    fun applySeverity(value: String?) {
        val next = value?.takeIf { it.isNotBlank() }
        if (next == _uiState.value.severity && _uiState.value.eventIds.isEmpty()) return
        _uiState.update { it.copy(severity = next, eventIds = emptyList(), limit = DEFAULT_LIMIT) }
        load()
    }

    fun applyEventType(value: String?) {
        val next = value?.takeIf { it.isNotBlank() }
        if (next == _uiState.value.eventType && _uiState.value.eventIds.isEmpty()) return
        _uiState.update { it.copy(eventType = next, eventIds = emptyList(), limit = DEFAULT_LIMIT) }
        load()
    }

    /**
     * Show exactly the events behind one error-aggregation bucket (issue
     * #426). Drops every other filter so the bucket is shown whole, and widens
     * the window to the cap.
     */
    fun applyEventIds(ids: List<Long>) {
        if (ids.isEmpty()) return
        correlationDebounceJob?.cancel()
        _uiState.update {
            it.copy(
                component = null,
                severity = null,
                eventType = null,
                correlationId = "",
                eventIds = ids,
                limit = MAX_LIMIT,
            )
        }
        load()
    }

    /** Debounced correlation-id field (web's useDebouncedValue + 350ms). */
    fun onCorrelationIdChange(raw: String) {
        correlationDebounceJob?.cancel()
        correlationDebounceJob = viewModelScope.launch {
            delay(CORRELATION_DEBOUNCE_MS)
            val trimmed = raw.trim()
            if (trimmed == _uiState.value.correlationId && _uiState.value.eventIds.isEmpty()) {
                return@launch
            }
            _uiState.update { it.copy(correlationId = trimmed, eventIds = emptyList(), limit = DEFAULT_LIMIT) }
            load()
        }
    }

    /**
     * Show every event sharing one correlation ID — the timeline's "view
     * related" drill-down. Drops the other filters so the chain is shown
     * whole, and widens the window to the cap.
     */
    fun showRelated(correlationId: String) {
        correlationDebounceJob?.cancel()
        _uiState.update {
            it.copy(
                component = null,
                severity = null,
                eventType = null,
                correlationId = correlationId,
                eventIds = emptyList(),
                limit = MAX_LIMIT,
            )
        }
        load()
    }

    fun clearFilters() {
        correlationDebounceJob?.cancel()
        if (!_uiState.value.hasActiveFilters) return
        _uiState.update {
            it.copy(
                component = null,
                severity = null,
                eventType = null,
                correlationId = "",
                eventIds = emptyList(),
                limit = DEFAULT_LIMIT,
            )
        }
        load()
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null) }
    }
}
