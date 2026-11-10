package com.mycorrhizal.crm.feature.shares

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ShareGrantRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
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
    private val repository: ShareGrantRepository
) : ViewModel() {

    private val _pendingCount = MutableStateFlow(0)
    val pendingCount: StateFlow<Int> = _pendingCount.asStateFlow()

    init {
        refresh()
    }

    fun refresh() {
        viewModelScope.launch {
            _pendingCount.value = repository.notifications().getOrNull()?.pendingCount ?: 0
        }
    }
}
