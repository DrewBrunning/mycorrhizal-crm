package com.mycorrhizal.crm.data.session

import com.mycorrhizal.crm.network.SessionExpiryNotifier
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.launch

/**
 * Glue between the network layer's 401 detection and the session store (issue
 * #678): every session-expiry signal clears the session, which flips the app
 * to the auth flow. Kept as its own class rather than inline in DI so the
 * behavior is unit-testable — the session must never survive a 401.
 */
class SessionExpiryWiring(
    private val sessionExpiryNotifier: SessionExpiryNotifier,
    private val sessionManager: SessionManager,
) {
    fun start(scope: CoroutineScope) {
        sessionExpiryNotifier.register {
            scope.launch { sessionManager.clearSession() }
        }
    }
}
