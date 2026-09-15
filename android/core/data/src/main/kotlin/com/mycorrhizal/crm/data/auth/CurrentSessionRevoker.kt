package com.mycorrhizal.crm.data.auth

import com.mycorrhizal.crm.network.ApiClient
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Issue #957 (finding #3 of the Android-logout review): revokes this
 * install's own server-side session (`DELETE /sessions/:id`, issue #866 /
 * ASVS 3.3.4) on logout, so the JWT stops being accepted immediately instead
 * of staying valid for its full absolute expiry after a client-side "logout"
 * that only ever dropped the local copy.
 *
 * The client never decodes its own JWT locally to read the `sid` claim —
 * `GET /sessions`' `current: true` row is a flow this app already has for
 * free, and one extra round trip is a non-issue on the least time-sensitive
 * path in the app. Meant to run as part of [com.mycorrhizal.crm.data.session.SessionTeardown],
 * i.e. while the session is still authenticated — see that interface's doc.
 */
@Singleton
class CurrentSessionRevoker @Inject constructor(
    private val api: ApiClient,
) {
    suspend fun revoke() {
        val sessions = api.listSessions().getOrNull() ?: return
        val current = sessions.firstOrNull { it.current } ?: return
        api.revokeSession(current.id)
    }
}
