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

    private val listeners = CopyOnWriteArrayList<(String?) -> Unit>()

    /**
     * Registers a listener to be invoked on every 401. The argument is the
     * bearer token the rejected request actually carried (null when it carried
     * none, or when the signaller doesn't know) — issue #1353: a Local profile
     * must only restart its embedded server when the token the server rejected
     * is the one it is currently running with.
     */
    fun register(listener: (String?) -> Unit) {
        listeners.add(listener)
    }

    fun onSessionExpired(rejectedBearer: String? = null) {
        listeners.forEach { it.invoke(rejectedBearer) }
    }
}
