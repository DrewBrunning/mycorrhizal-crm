package com.mycorrhizal.crm.data.session

import com.mycorrhizal.crm.domain.repository.SessionState
import com.mycorrhizal.crm.network.BaseUrlProvider
import com.mycorrhizal.crm.network.TokenProvider
import kotlinx.coroutines.flow.Flow

/**
 * Persists the bearer token. Implementations decide where the secret lives
 * (EncryptedSharedPreferences in production, in-memory in tests). The JWT is
 * a credential — it must never be logged.
 */
interface TokenStorage {
    suspend fun save(token: String)
    suspend fun load(): String?
    suspend fun clear()
}

/**
 * Persists non-secret session preferences (server URL, profile, language).
 * Plain DataStore is fine here — none of these are credentials.
 */
interface SessionPrefsStorage {
    suspend fun save(serverUrl: String?)
    suspend fun loadServerUrl(): String?
    suspend fun clear()
}

/**
 * Issue #385: wipes locally-cached user data (the Room mirror, cached images)
 * when a session ends. Runs inside [SessionManager.clearSession] so logout and
 * account-removal (invalid-token) paths both purge offline PII from the device.
 * Default no-op keeps `DefaultSessionManager` a plain JVM object for tests.
 */
interface SessionDataCleaner {
    suspend fun clear()
}

/** [SessionDataCleaner] that does nothing — the unit-test default. */
object NoopSessionDataCleaner : SessionDataCleaner {
    override suspend fun clear() = Unit
}

/**
 * Issue #957: authenticated network cleanup that must run while the session
 * is STILL valid, immediately before [SessionManager.clearSession] drops the
 * bearer token — the one place a request that needs THIS session's own
 * bearer (FCM device deregistration, the server-side session revoke) can
 * still go out authenticated. A call that runs after the token is gone (the
 * original bug) has nothing to attach and 401s, which then had a second,
 * worse effect: that 401 raced a device-grant refresh and silently logged
 * the user back in (see [SessionManager.isClearingSession]).
 *
 * core:data only knows this interface — FCM deregistration lives in
 * feature:tracking, which core:data cannot depend on, so the real
 * implementation is composed and Hilt-bound one layer up (the app module).
 *
 * Best-effort by contract: a failure here must never block the local clear
 * the caller is waiting on — see [DefaultSessionManager.clearSession].
 */
fun interface SessionTeardown {
    suspend fun beforeClear()
}

/** [SessionTeardown] that does nothing — the unit-test / no-binding default. */
object NoopSessionTeardown : SessionTeardown {
    override suspend fun beforeClear() = Unit
}

/**
 * Central session holder. Implements [TokenProvider] and [BaseUrlProvider]
 * from an in-memory cache so the synchronous OkHttp interceptors never touch
 * disk. The cache is hydrated at startup (see AppSessionManager).
 */
interface SessionManager : TokenProvider, BaseUrlProvider {
    fun observeSession(): Flow<SessionState>
    suspend fun serverUrl(): String?
    suspend fun token(): String?

    /** Persist a configured server origin before first authentication. */
    suspend fun setServerUrl(serverUrl: String)

    /** Persist a full session (server URL + bearer token + profile). */
    suspend fun setSession(serverUrl: String, token: String, state: SessionState)

    /**
     * Suspends until the persisted session has been hydrated into memory
     * (the async [DefaultSessionManager.init] at startup). Callers that must
     * read [serverUrl] or write a session before any user interaction — the
     * M5 OIDC cold-start deep link — await this first, or they'd read a null
     * URL and race the hydration write (review-pass fix).
     */
    suspend fun awaitHydrated()

    /** Merge profile details (userId, admin, language, …) into the session. */
    suspend fun setProfile(profile: SessionState)

    /**
     * Set the caller's "Me" contact pointer (T90, issue #831 Android parity)
     * in the session, replacing it outright rather than merging through
     * [setProfile] — unlike language/dateFormat, a null value here IS a
     * legitimate target state (the pointer cleared), so [setProfile]'s
     * "null means unchanged" merge semantics would make clearing it
     * impossible.
     */
    suspend fun setSelfContactVCardUid(vcardUid: String?)

    /**
     * Replace the stored bearer token in place, keeping the rest of the
     * session (server URL + profile) untouched. Used when the server re-issues
     * the session token after a token_version bump (2FA confirm/disable) so
     * the current session survives the mutation instead of 401ing on the next
     * request.
     */
    suspend fun setToken(token: String)

    /**
     * Drop the session: bearer token, cached profile and (via the
     * [SessionDataCleaner]) the local data mirror. The server URL survives by
     * default (issue #723) — it is non-credential device config, not part of
     * the session, so logout leaves it in place and the login screen can
     * pre-fill it. Pass `keepServerUrl = false` only when the URL itself must
     * go too (an explicit "forget this server" action, if one is ever added).
     */
    suspend fun clearSession(keepServerUrl: Boolean = true)

    /**
     * True while [clearSession] is actively running its authenticated
     * [SessionTeardown] step (issue #957). [SessionExpiryWiring] checks this
     * before attempting a device-grant refresh: a 401 that the teardown
     * step's own network calls trigger (the bearer they used is being
     * invalidated right now) must not race a refresh that would silently
     * resurrect the session already being torn down.
     */
    fun isClearingSession(): Boolean
}
