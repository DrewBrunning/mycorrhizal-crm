package com.mycorrhizal.crm.data.session

import com.mycorrhizal.crm.domain.profile.ServerProfile
import com.mycorrhizal.crm.domain.repository.SessionState
import com.mycorrhizal.crm.network.ArchivedProfileProvider
import com.mycorrhizal.crm.network.BaseUrlProvider
import com.mycorrhizal.crm.network.TokenProvider
import kotlinx.coroutines.flow.Flow

/**
 * Persists the bearer token, keyed by server-profile ID (ADR 0028 Decision 1:
 * credentials move from the single `jwt` entry to per-profile `jwt:<id>` slots
 * so switching profiles keeps each one's session isolated). Implementations
 * decide where the secrets live (EncryptedSharedPreferences in production,
 * in-memory in tests). The JWT is a credential — it must never be logged.
 *
 * [loadLegacy]/[clearLegacy] expose the pre-profiles single `jwt` slot purely
 * for the one-time in-place migration; nothing else may read them.
 */
interface TokenStorage {
    suspend fun save(profileId: String, token: String)
    suspend fun load(profileId: String): String?
    suspend fun clear(profileId: String)

    /** The pre-profiles `jwt` value, or null (already migrated / fresh install). */
    suspend fun loadLegacy(): String?

    /** Removes the pre-profiles `jwt` slot once it has been migrated. */
    suspend fun clearLegacy()
}

/**
 * The persisted profile list and which one is active. The list is ordered as
 * the user sees it; [activeProfileId] must name one of [profiles] (or be null
 * when there are none).
 */
data class ProfilesSnapshot(
    val profiles: List<ServerProfile> = emptyList(),
    val activeProfileId: String? = null,
)

/**
 * Persists non-secret session preferences: the server profiles and the active
 * profile ID (ADR 0028 Decision 1 — "the profile list and active ID live in
 * DataStore (`session_prefs`)"). Plain DataStore is fine here — none of these
 * are credentials. [save]/[loadServerUrl] are the pre-profiles server URL,
 * retained only for the one-time migration.
 */
interface SessionPrefsStorage {
    suspend fun save(serverUrl: String?)
    suspend fun loadServerUrl(): String?

    suspend fun saveProfiles(snapshot: ProfilesSnapshot)
    suspend fun loadProfiles(): ProfilesSnapshot

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
interface SessionManager : TokenProvider, BaseUrlProvider, ArchivedProfileProvider {
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

    // --- ADR 0028 Decision 1: server profiles ---------------------------------

    /** The configured profiles, in display order. */
    suspend fun profiles(): List<ServerProfile>

    /** Emits the profile list whenever it changes. */
    fun observeProfiles(): Flow<List<ServerProfile>>

    /** The active profile, or null when none is configured. */
    suspend fun activeProfile(): ServerProfile?

    /** Emits the active profile (null when none is configured). */
    fun observeActiveProfile(): Flow<ServerProfile?>

    /** The active profile's ID, or null. */
    suspend fun activeProfileId(): String?

    /**
     * Create a new Remote profile and make it active. Used by "Add server" in
     * Settings and by the login flow when the typed URL does not match any
     * existing profile. The caller must have dealt with the previous profile's
     * outbox/Room first (a brand-new profile has none of the new profile's
     * data cached, but the previous profile's cache still has to go) — the
     * Servers screen routes both through [switchProfile].
     */
    suspend fun addRemoteProfile(label: String, url: String): ServerProfile

    /**
     * ADR 0028 Decision 2 / issue #1262: create a [ServerProfileKind.Local]
     * profile (or reuse the existing one), make it active, and store the session
     * token the embedded server just minted as its credential. A Local profile
     * is always logged in — there is no login surface. The previous profile's
     * Room mirror is cleared like any other switch.
     */
    suspend fun activateLocalProfile(token: String): ServerProfile

    /** Rename a profile (the label is non-secret UI metadata). */
    suspend fun renameProfile(id: String, label: String)

    /**
     * Remove a profile and its per-profile credentials. When the profile is
     * active, its session is first revoked through the authenticated
     * [SessionTeardown] path (issue #957 wiring) so the server stops honouring
     * it. If another profile remains it becomes active; otherwise the session
     * is left logged out with no active profile.
     */
    suspend fun removeProfile(id: String)

    /** Unsent interactions in the outbox — named in the switch confirmation. */
    suspend fun pendingInteractionCount(): Int

    /**
     * Switch the active profile (ADR 0028 Decision 1):
     *  1. attempt a best-effort outbox drain;
     *  2. if unsynced interactions remain and [discardPending] is false, return
     *     [SwitchProfileResult.NeedsConfirmation] naming the count WITHOUT
     *     switching;
     *  3. otherwise clear the Room mirror ([SessionDataCleaner]), set the new
     *     profile active, and load its token — the session flow re-emits, which
     *     re-runs the compatibility gate for the new server.
     */
    suspend fun switchProfile(id: String, discardPending: Boolean = false): SwitchProfileResult

    // --- ADR 0028 Decision 3 / issue #1265: attach-to-remote migration --------

    /**
     * Persist [token] as [id]'s credential WITHOUT activating the profile. The
     * attach wizard logs into a Remote profile while the Local one stays active;
     * the token must survive until the final switch, and an interrupted wizard
     * leaves it stored (a re-run simply logs in again and overwrites it).
     */
    suspend fun saveProfileToken(id: String, token: String)

    /**
     * Mark a profile a read-only archive (or clear the mark). Only a Local
     * profile can be archived. Takes effect on the very next request: the
     * network layer's write-blocking interceptor reads [isActiveProfileArchived].
     */
    suspend fun setProfileArchived(id: String, archived: Boolean)
}

/**
 * The outcome of [SessionManager.switchProfile]. [NeedsConfirmation] carries the
 * number of unsynced interactions the user must agree to discard before the
 * switch (and the cache wipe) proceeds.
 */
sealed interface SwitchProfileResult {
    data object Switched : SwitchProfileResult
    data class NeedsConfirmation(val pendingCount: Int) : SwitchProfileResult
}

/**
 * Attempts to flush the pending_interactions outbox before a profile switch.
 * core:data cannot reach the WorkManager worker that normally drains it (it
 * lives in feature:tracking), so the real implementation is composed one layer
 * up; the default is a no-op, which leaves the unsynced rows in place and makes
 * the caller show the discard confirmation — the "or show a confirmation" half
 * of ADR 0028 Decision 1.
 */
fun interface OutboxDrainer {
    /** Returns true when the outbox is empty after the attempt. */
    suspend fun drain(): Boolean
}

/** [OutboxDrainer] that cannot drain — the unit-test / no-binding default. */
object NoopOutboxDrainer : OutboxDrainer {
    override suspend fun drain(): Boolean = false
}

/**
 * The per-profile credentials that are not the session JWT: the device-grant
 * token (issue #722) and the OIDC pending request (issue #965). [DefaultSessionManager]
 * needs to move the legacy single-profile slots onto the migrated profile and
 * drop them when a profile is removed; the stores themselves live in this same
 * package, but the seam keeps the manager a plain JVM object for tests.
 */
interface ProfileSecretStorage {
    /** Moves the pre-profiles device grant / OIDC pending slots onto [profileId]. */
    suspend fun migrateLegacy(profileId: String)

    /** Drops every per-profile secret for [profileId]. */
    suspend fun clear(profileId: String)
}

/** [ProfileSecretStorage] that does nothing — the unit-test / no-binding default. */
object NoopProfileSecretStorage : ProfileSecretStorage {
    override suspend fun migrateLegacy(profileId: String) = Unit
    override suspend fun clear(profileId: String) = Unit
}
