package com.mycorrhizal.crm

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.domain.repository.SessionState
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.stateIn
import javax.inject.Inject

/** Root application state: whether a session exists (drives login vs main tree). */
@HiltViewModel
class MainViewModel @Inject constructor(
    sessionManager: SessionManager,
) : ViewModel() {
    val session: StateFlow<SessionState> = sessionManager.observeSession()
        .stateIn(viewModelScope, SharingStarted.Eagerly, SessionState())
}
