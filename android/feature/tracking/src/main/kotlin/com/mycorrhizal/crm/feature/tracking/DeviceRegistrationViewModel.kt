package com.mycorrhizal.crm.feature.tracking

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.data.session.SessionManager
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.launch
import javax.inject.Inject

/**
 * M5 §5a (issue #152): registers the FCM device on login and deletes it on
 * logout, reacting to the session flow's login/logout transitions. Lives for as
 * long as the app tree is composed (wired in MycorrhizalApp's MainScaffold), so
 * a session change from ANY login path (password, API token, OIDC) triggers it.
 *
 * Registration is a no-op when Firebase is unavailable (FcmAvailability), which
 * is the polling-fallback guarantee: a Firebase-less build or de-Googled device
 * simply never enrolls a device.
 */
@HiltViewModel
class DeviceRegistrationViewModel @Inject constructor(
    sessionManager: SessionManager,
    private val deviceRegistration: DeviceRegistrationManager,
) : ViewModel() {

    private var wasLoggedIn = false

    init {
        viewModelScope.launch {
            sessionManager.observeSession().collect { session ->
                when {
                    session.isLoggedIn && !wasLoggedIn -> {
                        wasLoggedIn = true
                        // Re-register on every login: the token may have
                        // rotated since the last session, and the server's
                        // (client, token) key makes this idempotent.
                        deviceRegistration.register()
                    }
                    !session.isLoggedIn && wasLoggedIn -> {
                        wasLoggedIn = false
                        deviceRegistration.delete()
                    }
                }
            }
        }
    }
}
