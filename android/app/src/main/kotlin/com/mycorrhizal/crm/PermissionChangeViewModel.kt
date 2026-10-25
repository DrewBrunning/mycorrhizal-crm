package com.mycorrhizal.crm

import androidx.lifecycle.ViewModel
import com.mycorrhizal.crm.data.session.RoleRefresher
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.flow.SharedFlow

/**
 * Issue #1826: surfaces [RoleRefresher]'s demotion events to the main nav tree
 * (which owns the NavController). Injecting it is also what creates, and so
 * starts, the refresher: `MainScaffold` instantiates it before the NavHost so
 * no screen's 403 can precede the listener.
 */
@HiltViewModel
class PermissionChangeViewModel @Inject constructor(roleRefresher: RoleRefresher) : ViewModel() {
    /** Emits when a re-fetch finds the user is no longer an admin. */
    val demotions: SharedFlow<Unit> = roleRefresher.demotions
}
