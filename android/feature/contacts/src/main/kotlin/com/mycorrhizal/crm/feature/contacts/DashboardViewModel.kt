package com.mycorrhizal.crm.feature.contacts

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.model.network.Birthday
import com.mycorrhizal.crm.model.network.DashboardRandomContact
import com.mycorrhizal.crm.model.network.DashboardReminder
import com.mycorrhizal.crm.model.network.OverdueCadence
import com.mycorrhizal.crm.model.network.OverdueDataDecayPolicy
import com.mycorrhizal.crm.model.network.ReachOutSuggestion
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class DashboardUiState(
    val birthdays: List<Birthday> = emptyList(),
    val upcomingReminders: List<DashboardReminder> = emptyList(),
    val randomContacts: List<DashboardRandomContact> = emptyList(),
    val overdueCadences: List<OverdueCadence> = emptyList(),
    // Issue #212: the favorites quick-access block (web #173) — the user's
    // favorite, non-archived contacts, name-ordered. Same wire shape as
    // randomContacts, so the same model serves both.
    val favorites: List<DashboardRandomContact> = emptyList(),
    // Issue #177: pending event-driven reach-out suggestions.
    val reachOutSuggestions: List<ReachOutSuggestion> = emptyList(),
    // Issue #352: contacts whose info is due for re-verification.
    val dataDecayOverdue: List<OverdueDataDecayPolicy> = emptyList(),
    val isLoading: Boolean = false,
    /** The dashboard-wide load failure; the screen replaces the widgets with an error + retry. */
    val error: String? = null,
    /**
     * Transient failure from a complete/skip action. Shown as a snackbar and
     * cleared via [DashboardViewModel.onActionErrorShown] — deliberately
     * separate from [error], because a failed action must not blank the
     * already-rendered dashboard the way a failed load does.
     */
    val actionError: String? = null,
    /** The reminder currently being completed/skipped; guards against double-taps. */
    val completingId: Int? = null,
    /** The reach-out suggestion currently being dismissed; guards against double-taps. */
    val dismissingSuggestionId: String? = null,
    /** The data-decay policy currently being verified; guards against double-taps. */
    val verifyingDataDecayId: String? = null,
    /** The signed-in user's `date_format` preference; falls back to "eu" when absent. */
    val dateFormat: String? = null,
) {
    /** True when at least one widget has data, so a refresh keeps the list on screen. */
    val hasContent: Boolean
        get() = birthdays.isNotEmpty() || upcomingReminders.isNotEmpty() || randomContacts.isNotEmpty() ||
            overdueCadences.isNotEmpty() || favorites.isNotEmpty() || reachOutSuggestions.isNotEmpty() ||
            dataDecayOverdue.isNotEmpty()
}

/**
 * M10 — the M3 dashboard composite consumer. One
 * `GET /dashboard` call populates the widgets (birthdays, upcoming
 * reminders, random "stay in touch" contacts, overdue cadences, and — since
 * issue #212 — the favorites quick-access block) — the
 * composite replaced the two legacy endpoints this ViewModel used to fan out
 * (`listUpcomingBirthdays` + `listUpcomingReminders`), and it aggregates the
 * widgets that had no call at all.
 *
 * Complete/skip is optimistic: the reminder leaves the widget immediately and
 * is restored at its original position if the call fails (M10 test case 3).
 * Skip (`?skip=true`) reschedules recurring reminders without recording
 * completion; the screen confirms before invoking it.
 */
@HiltViewModel
class DashboardViewModel @Inject constructor(
    private val apiClient: ApiClient,
    private val authRepository: AuthRepository,
) : ViewModel() {

    private val _uiState = MutableStateFlow(DashboardUiState())
    val uiState: StateFlow<DashboardUiState> = _uiState.asStateFlow()

    /** The in-flight load, so a retry tapped twice can't fire two overlapping fetches. */
    private var loadJob: Job? = null

    init {
        load()
        viewModelScope.launch {
            authRepository.observeSession().collect { session ->
                _uiState.update { it.copy(dateFormat = session.dateFormat) }
            }
        }
    }

    fun load() {
        if (loadJob?.isActive == true) return
        loadJob = viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            apiClient.getDashboard().foldApiError(
                onSuccess = { dashboard ->
                    _uiState.update {
                        it.copy(
                            isLoading = false,
                            birthdays = dashboard.birthdays,
                            upcomingReminders = dashboard.upcomingReminders,
                            randomContacts = dashboard.randomContacts,
                            overdueCadences = dashboard.overdue,
                            favorites = dashboard.favorites,
                            reachOutSuggestions = dashboard.reachOutSuggestions,
                            dataDecayOverdue = dashboard.dataDecayOverdue,
                        )
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    /**
     * Complete (or, with [skip], skip) a reminder from the dashboard widget.
     * Optimistically removes it from [DashboardUiState.upcomingReminders] and
     * restores it at its original position if the call fails.
     */
    fun completeReminder(id: Int, skip: Boolean = false) {
        if (_uiState.value.completingId != null) return
        viewModelScope.launch {
            val index = _uiState.value.upcomingReminders.indexOfFirst { it.id == id }
            val removed = _uiState.value.upcomingReminders.getOrNull(index)
            _uiState.update { state ->
                state.copy(
                    completingId = id,
                    upcomingReminders = state.upcomingReminders.filterNot { it.id == id },
                )
            }
            apiClient.completeReminder(id, skip).foldApiError(
                onSuccess = { _uiState.update { it.copy(completingId = null) } },
                onError = { error ->
                    _uiState.update { state ->
                        if (removed == null) {
                            state.copy(completingId = null, actionError = error.displayMessage)
                        } else {
                            val restored = state.upcomingReminders.toMutableList()
                            restored.add(index.coerceIn(0, restored.size), removed)
                            state.copy(completingId = null, actionError = error.displayMessage, upcomingReminders = restored)
                        }
                    }
                },
            )
        }
    }

    /**
     * Dismisses a reach-out suggestion from the dashboard widget (issue
     * #177). Optimistically removes it and restores it at its original
     * position if the call fails — mirrors [completeReminder]'s contract.
     */
    fun dismissReachOutSuggestion(id: String) {
        if (_uiState.value.dismissingSuggestionId != null) return
        viewModelScope.launch {
            val index = _uiState.value.reachOutSuggestions.indexOfFirst { it.id == id }
            val removed = _uiState.value.reachOutSuggestions.getOrNull(index)
            _uiState.update { state ->
                state.copy(
                    dismissingSuggestionId = id,
                    reachOutSuggestions = state.reachOutSuggestions.filterNot { it.id == id },
                )
            }
            apiClient.dismissReachOutSuggestion(id).foldApiError(
                onSuccess = { _uiState.update { it.copy(dismissingSuggestionId = null) } },
                onError = { error ->
                    _uiState.update { state ->
                        if (removed == null) {
                            state.copy(dismissingSuggestionId = null, actionError = error.displayMessage)
                        } else {
                            val restored = state.reachOutSuggestions.toMutableList()
                            restored.add(index.coerceIn(0, restored.size), removed)
                            state.copy(dismissingSuggestionId = null, actionError = error.displayMessage, reachOutSuggestions = restored)
                        }
                    }
                },
            )
        }
    }

    /**
     * Confirms a contact's info is still current from the dashboard widget
     * (issue #352). Optimistically removes the policy from
     * [DashboardUiState.dataDecayOverdue] and restores it at its original
     * position if the call fails — mirrors [dismissReachOutSuggestion]'s
     * contract. Android v1 scope is view + confirm only; creating/editing a
     * policy is web-only for now (docs/adrs/0027-data-decay.md).
     */
    fun verifyDataDecay(id: String) {
        if (_uiState.value.verifyingDataDecayId != null) return
        viewModelScope.launch {
            val index = _uiState.value.dataDecayOverdue.indexOfFirst { it.policy?.id == id }
            val removed = _uiState.value.dataDecayOverdue.getOrNull(index)
            _uiState.update { state ->
                state.copy(
                    verifyingDataDecayId = id,
                    dataDecayOverdue = state.dataDecayOverdue.filterNot { it.policy?.id == id },
                )
            }
            apiClient.verifyDataDecayPolicy(id).foldApiError(
                onSuccess = { _uiState.update { it.copy(verifyingDataDecayId = null) } },
                onError = { error ->
                    _uiState.update { state ->
                        if (removed == null) {
                            state.copy(verifyingDataDecayId = null, actionError = error.displayMessage)
                        } else {
                            val restored = state.dataDecayOverdue.toMutableList()
                            restored.add(index.coerceIn(0, restored.size), removed)
                            state.copy(verifyingDataDecayId = null, actionError = error.displayMessage, dataDecayOverdue = restored)
                        }
                    }
                },
            )
        }
    }

    /** Clears the transient complete/skip error once its snackbar has been shown. */
    fun onActionErrorShown() {
        _uiState.update { it.copy(actionError = null) }
    }
}
