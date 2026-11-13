package com.mycorrhizal.crm.feature.shares

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ShareGrantRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/**
 * The in-app pending-grant badge (ADR 0042 decision 11). It carries a count only — never a label
 * or sender — and a failed refresh (an older server, no connection) hides the badge rather than
 * showing a stale number.
 */
@HiltViewModel
class ShareGrantBadgeViewModel @Inject constructor(
    private val repository: ShareGrantRepository,
    private val shareGrantChanges: ShareGrantChanges = ShareGrantChanges()
) : ViewModel() {

    private val _pendingCount = MutableStateFlow(0)
    val pendingCount: StateFlow<Int> = _pendingCount.asStateFlow()

    private var refreshJob: Job? = null

    init {
        refresh()
        // A grant changed on a share screen (issue #2557): re-read without waiting for the
        // drawer to reopen or the route to change.
        viewModelScope.launch { shareGrantChanges.changes.collect { refresh() } }
    }

    fun refresh() {
        // A newer read supersedes an older one still in flight, so a slow response cannot
        // overwrite the fresher count.
        refreshJob?.cancel()
        refreshJob = viewModelScope.launch {
            _pendingCount.value = repository.notifications().getOrNull()?.pendingCount ?: 0
        }
    }
}
