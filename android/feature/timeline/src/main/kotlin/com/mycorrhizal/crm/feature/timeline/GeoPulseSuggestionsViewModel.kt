package com.mycorrhizal.crm.feature.timeline

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.GeoPulseRepository
import com.mycorrhizal.crm.model.network.GeoPulseStaySuggestion
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.time.Instant
import java.time.LocalDate
import java.time.LocalTime
import java.time.OffsetDateTime
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import javax.inject.Inject

data class GeoPulseSuggestionsUiState(
    /** The date shown in the picker (YYYY-MM-DD) — may differ from [lookedUpDate] until "Look up". */
    val date: String,
    val isLoading: Boolean = false,
    val error: String? = null,
    /** The suggestions of the last successful lookup; null until one has run. */
    val suggestions: List<GeoPulseStaySuggestion>? = null,
    /** The date the current [suggestions] were fetched for. */
    val lookedUpDate: String? = null,
    /** IANA zone the lookup used; stay times and local dates are rendered in it. */
    val timezone: String,
)

/**
 * Issue #160 (ADR 0033), Android parity with web's `GeoPulseSuggestionsDialog.tsx`: pick a date →
 * see where GeoPulse says you were → tap a stay to open the activity form pre-filled. The list is
 * ephemeral (nothing is stored until the form is saved); the contacts are always the user's choice.
 */
@HiltViewModel
class GeoPulseSuggestionsViewModel @Inject constructor(
    private val repository: GeoPulseRepository,
) : ViewModel() {

    private val zone: ZoneId = ZoneId.systemDefault()

    private val _uiState = MutableStateFlow(
        GeoPulseSuggestionsUiState(date = LocalDate.now(zone).toString(), timezone = zone.id),
    )
    val uiState: StateFlow<GeoPulseSuggestionsUiState> = _uiState.asStateFlow()

    private var lookupJob: Job? = null

    fun onDateChange(value: String) = _uiState.update { it.copy(date = value) }

    /** Look up the picked date. A newer lookup cancels an in-flight older one (no stale overwrite). */
    fun lookup() {
        val s = _uiState.value
        if (s.date.isBlank()) return
        lookupJob?.cancel()
        lookupJob = viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            repository.getSuggestions(s.date, s.timezone).foldApiError(
                onSuccess = { resp ->
                    _uiState.update {
                        it.copy(
                            isLoading = false,
                            suggestions = resp.suggestions,
                            lookedUpDate = resp.date.ifBlank { s.date },
                        )
                    }
                },
                onError = { e ->
                    _uiState.update { it.copy(isLoading = false, error = e.displayMessage, suggestions = null) }
                },
            )
        }
    }

    /**
     * Re-run the last lookup (no-op before the first one), so a stay logged from the form is
     * shown as "Already logged" when the user comes back (the server stamps existing_activity_id).
     */
    fun refreshIfLoaded() {
        val looked = _uiState.value.lookedUpDate ?: return
        if (_uiState.value.isLoading) return
        _uiState.update { it.copy(date = looked) }
        lookup()
    }
}

private fun parseStayInstant(timestamp: String): Instant? =
    runCatching { OffsetDateTime.parse(timestamp).toInstant() }.getOrNull()

/**
 * The stay's own local calendar day as the activity form's RFC 3339 `date` — local midnight of the
 * day the stay began, in [zone] (issue #1500: never the picker's date, which may have been changed
 * after the lookup). Falls back to [fallbackDate] at local midnight if [timestamp] doesn't parse.
 */
fun stayPrefillDate(timestamp: String, zone: ZoneId, fallbackDate: String): String {
    val day = parseStayInstant(timestamp)?.atZone(zone)?.toLocalDate()
        ?: runCatching { LocalDate.parse(fallbackDate) }.getOrNull()
        ?: LocalDate.now(zone)
    return day.atTime(LocalTime.MIDNIGHT).atZone(zone).format(DateTimeFormatter.ISO_OFFSET_DATE_TIME)
}

/** The stay's start time as HH:mm in [zone] (the same zone the lookup defined the day in). */
fun formatStayTime(timestamp: String, zone: ZoneId): String =
    parseStayInstant(timestamp)?.atZone(zone)?.format(DateTimeFormatter.ofPattern("HH:mm")) ?: timestamp
