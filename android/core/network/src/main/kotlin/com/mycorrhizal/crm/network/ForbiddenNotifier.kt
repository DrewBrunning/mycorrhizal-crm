package com.mycorrhizal.crm.network

import java.util.concurrent.CopyOnWriteArrayList

/**
 * Broadcasts "an authenticated API call came back 403" (issue #1826), the
 * sibling of [SessionExpiryNotifier] for 401. A 403 does not end the session,
 * but it can mean the user's role changed server-side (an admin demoted), so
 * the app-level wiring re-reads `/users/me`. Same synchronous-listener design
 * as the 401 notifier: the interceptor layer is synchronous and has no suspend
 * access to the session store, so it only signals here.
 */
class ForbiddenNotifier {

    private val listeners = CopyOnWriteArrayList<() -> Unit>()

    fun register(listener: () -> Unit) {
        listeners.add(listener)
    }

    fun onForbidden() {
        listeners.forEach { it.invoke() }
    }
}
