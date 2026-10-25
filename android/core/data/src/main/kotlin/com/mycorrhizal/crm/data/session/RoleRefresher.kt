package com.mycorrhizal.crm.data.session

import com.mycorrhizal.crm.network.ForbiddenNotifier
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.channels.BufferOverflow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch

/**
 * Issue #1826: Android fetches `/users/me` once per active profile per process,
 * so a server-side demotion left the admin UI on screen until a cold start.
 * Every authenticated 403 (signalled by [ForbiddenNotifier]) re-reads the
 * profile, applies a changed `isAdmin` through [SessionManager.setProfile], and
 * — when the user lost admin — emits on [demotions] so the app can leave an
 * admin-only screen. The 401 path ([SessionExpiryWiring]) is untouched.
 *
 * **Debounce:** the first 403 claims a [debounceMillis] window and fetches; every
 * further 403 inside that window (a screen firing several admin calls at once)
 * is dropped, so a burst costs exactly one `/users/me`. The window is claimed
 * before the fetch and not released on failure, so an offline device does not
 * hammer the endpoint either. A 403 that leaves the role unchanged (an ownership
 * 403) writes nothing and emits nothing: the UI stays as it was.
 *
 * [fetchIsAdmin] returns the server's current `is_admin`, or null when it could
 * not be read; it is a lambda (not an `AuthRepository`) so core:data's DI can
 * resolve the repository lazily — the repository's HTTP client depends on the
 * session manager that wires this class.
 */
class RoleRefresher(
    private val forbiddenNotifier: ForbiddenNotifier,
    private val sessionManager: SessionManager,
    private val fetchIsAdmin: suspend () -> Boolean?,
    private val debounceMillis: Long = DEFAULT_DEBOUNCE_MILLIS,
    // Monotonic: the debounce window measures elapsed time, and the wall
    // clock can jump (manual change, NTP sync, a time-zone travel fix), which
    // would either open a second window inside the first or never reopen one.
    private val nowMillis: () -> Long = { System.nanoTime() / NANOS_PER_MILLI }
) {
    private val _demotions = MutableSharedFlow<Unit>(
        extraBufferCapacity = 1,
        onBufferOverflow = BufferOverflow.DROP_OLDEST
    )

    /** Emits once each time a re-fetch finds the signed-in user is no longer an admin. */
    val demotions: SharedFlow<Unit> = _demotions.asSharedFlow()

    private val lock = Any()
    private var windowStart: Long? = null

    fun start(scope: CoroutineScope) {
        forbiddenNotifier.register {
            if (claimWindow()) scope.launch { refresh() }
        }
    }

    /** True for the first caller in a window; later callers inside it get false. */
    private fun claimWindow(): Boolean = synchronized(lock) {
        val now = nowMillis()
        val start = windowStart
        if (start != null && now - start < debounceMillis) return@synchronized false
        windowStart = now
        true
    }

    private suspend fun refresh() {
        try {
            val before = sessionManager.observeSession().first()
            if (!before.isLoggedIn) return
            val profileId = sessionManager.activeProfileId()
            val isAdmin = fetchIsAdmin() ?: return
            // The user may have switched profile or logged out while we fetched.
            if (sessionManager.activeProfileId() != profileId) return
            if (isAdmin == before.isAdmin) return
            sessionManager.setProfile(ProfileUpdate(isAdmin = isAdmin))
            if (before.isAdmin && !isAdmin) _demotions.tryEmit(Unit)
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            // Best effort: the 403 itself already surfaced as the screen's own error.
        }
    }

    companion object {
        /** A burst of 403s from one screen lands well inside this. */
        const val DEFAULT_DEBOUNCE_MILLIS = 30_000L
        private const val NANOS_PER_MILLI = 1_000_000L
    }
}
