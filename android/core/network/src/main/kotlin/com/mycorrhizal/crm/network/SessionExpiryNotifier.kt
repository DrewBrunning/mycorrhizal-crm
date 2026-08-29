package com.mycorrhizal.crm.network

import java.util.concurrent.CopyOnWriteArrayList

/**
 * Broadcasts "an API call came back 401" to whoever owns the session (issue
 * #678). The interceptor layer is synchronous and has no suspend access to the
 * session store, so it only signals here; the app-level wiring
 * ([SessionExpiryWiring]) registers a listener that clears the session, which
 * flips the app to the auth flow.
 *
 * A synchronous listener list (rather than a [kotlinx.coroutines.flow.SharedFlow])
 * is deliberate: there is exactly one consumer, the registration happens at
 * session-manager construction (before any request can 401), and the test
 * scheduler can't drop a suspended-collector delivery. [CopyOnWriteArrayList]
 * keeps [onSessionExpired] safe to call from any OkHttp thread.
 */
class SessionExpiryNotifier {

    private val listeners = CopyOnWriteArrayList<() -> Unit>()

    /** Registers a listener to be invoked on every 401. */
    fun register(listener: () -> Unit) {
        listeners.add(listener)
    }

    fun onSessionExpired() {
        listeners.forEach { it.invoke() }
    }
}
