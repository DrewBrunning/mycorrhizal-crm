package com.mycorrhizal.crm.data.local

import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.domain.profile.ServerProfileKind

/**
 * Issue #1312: a `Local` profile has no login surface (ADR 0028), so when the
 * embedded server rejects its session (401) the only recovery is to restart the
 * embedded server — which mints a fresh session for its single user at start —
 * and adopt the new token. Issue #1340: each start revokes the previous one, so a
 * 401 can also mean the stored token predates a restart that already happened
 * (cold start, process death) — then the running server's token is adopted
 * without another restart. Clearing the session instead would strand the user
 * on the auth flow with no way back short of "Use on this device only" again.
 */
class LocalSessionReminter(
    private val localServerHost: LocalServerHost,
    private val sessionManager: SessionManager,
) {
    /**
     * Returns null when the active profile is not [ServerProfileKind.Local]
     * (the caller falls back to its normal 401 handling), true when the server
     * was restarted and the fresh token adopted, and false when the restart
     * failed (the caller then clears the session). [rejectedBearer] is the token
     * the 401'd request carried; a restart only happens when it is the running
     * server's own token (or unknown, or the server is not running).
     */
    suspend fun remint(rejectedBearer: String? = null): Boolean? {
        if (sessionManager.activeProfile()?.kind !is ServerProfileKind.Local) return null
        val running = localServerHost.sessionTokenIfRunning()
        if (running != null && running != sessionManager.token()) {
            sessionManager.activateLocalProfile(running)
            return true
        }
        // Issue #1353: the rejected request carried a token other than the one
        // the running server holds — a stale token from before the latest
        // start. The running token is already stored (above), so there is
        // nothing to fix and restarting would only revoke a healthy session.
        if (running != null && rejectedBearer != null && rejectedBearer != running) return true
        localServerHost.stop()
        val endpoint = localServerHost.ensureStarted().getOrNull() ?: return false
        sessionManager.activateLocalProfile(endpoint.sessionToken)
        return true
    }
}
