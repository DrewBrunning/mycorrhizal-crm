package com.mycorrhizal.crm.data.session

import com.mycorrhizal.crm.network.SessionExpiryNotifier
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.launch
import java.util.concurrent.atomic.AtomicBoolean

/**
 * Glue between the network layer's 401 detection and the session store (issue
 * #678): every session-expiry signal ends the session, which flips the app to
 * the auth flow. Kept as its own class rather than inline in DI so the
 * behavior is unit-testable — the session must never survive a 401.
 *
 * Issue #722: when this install holds a device grant, the 401 first tries one
 * grant exchange ([refresher]) — a session that expired because its JWT ran
 * out is seamlessly renewed on a device whose owner already passed the local
 * biometric gate. Only when that refresh fails (grant revoked / network
 * rejected) — or when no grant is enrolled, the default — does the session
 * clear. A grant that the server has revoked therefore still ends exactly as
 * a 401 always has.
 *
 * Issue #957/#967 — two guards on the naive "every 401 launches a refresh"
 * design, both real bugs found by the same adversarial review pass:
 *  - **Re-entrancy** ([SessionManager.isClearingSession]): [clearSession]'s
 *    own authenticated teardown step (FCM deregister, session revoke) makes
 *    network calls with the bearer being invalidated right now, so it can
 *    401 too. Without this guard that 401 would attempt a grant exchange —
 *    silently logging the user back in seconds after an explicit logout.
 *  - **Single-flight** ([refreshInFlight]): with no guard, a burst of
 *    concurrent 401s — or the grant-exchange request's own 401 re-firing
 *    this same listener recursively — each launched their own refresh,
 *    producing a storm of grant-exchange requests instead of one clean
 *    outcome (only the backend's 429 used to stop it).
 */
class SessionExpiryWiring(
    private val sessionExpiryNotifier: SessionExpiryNotifier,
    private val sessionManager: SessionManager,
    private val refresher: suspend () -> Boolean = { false },
) {
    private val refreshInFlight = AtomicBoolean(false)

    fun start(scope: CoroutineScope) {
        sessionExpiryNotifier.register {
            if (sessionManager.isClearingSession()) return@register
            if (!refreshInFlight.compareAndSet(false, true)) return@register
            scope.launch {
                try {
                    val refreshed = refresher()
                    if (!refreshed) sessionManager.clearSession()
                } finally {
                    refreshInFlight.set(false)
                }
            }
        }
    }
}
