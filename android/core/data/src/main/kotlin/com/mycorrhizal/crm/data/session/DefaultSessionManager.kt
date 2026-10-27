package com.mycorrhizal.crm.data.session

import com.mycorrhizal.crm.domain.profile.LOCAL_PROFILE_LABEL
import com.mycorrhizal.crm.domain.profile.ServerProfile
import com.mycorrhizal.crm.domain.profile.ServerProfileKind
import com.mycorrhizal.crm.domain.profile.defaultProfileLabel
import com.mycorrhizal.crm.domain.repository.PendingInteractionRepository
import com.mycorrhizal.crm.domain.repository.SessionState
import com.mycorrhizal.crm.network.LOCAL_SERVER_SENTINEL_URL
import java.util.UUID
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicLong
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

/**
 * In-memory-backed [SessionManager] used by unit tests and as the base for
 * the production Android implementation. Holds the active profile's JWT and
 * server URL in memory; persistence is delegated to the injected [TokenStorage]
 * and [SessionPrefsStorage] so the class itself is a plain JVM object.
 *
 * ADR 0028 Decision 1: the server URL and its credential are no longer a single
 * global pair — they are one entry in a profile list, one of which is active at
 * a time. A legacy install (single `server_url` + `jwt`) is migrated in place on
 * [init] into one Remote profile, with no re-login.
 */
class DefaultSessionManager(
    private val tokenStorage: TokenStorage,
    private val prefsStorage: SessionPrefsStorage,
    private val localDataCleaner: SessionDataCleaner = NoopSessionDataCleaner,
    private val sessionTeardown: SessionTeardown = NoopSessionTeardown,
    // The pending_interactions outbox — counted for the switch confirmation and
    // drained (best-effort) before a switch. Optional so the many JVM tests
    // that only exercise session state need not build a Room-backed repo.
    private val pendingInteractions: PendingInteractionRepository? = null,
    private val outboxDrainer: OutboxDrainer = NoopOutboxDrainer,
    // Per-profile non-JWT secrets (device grant, OIDC pending) — migrated off
    // the legacy slots and dropped when a profile is removed. Default no-op.
    private val profileSecretStorage: ProfileSecretStorage = NoopProfileSecretStorage,
    // Issue #2065: held across every identity transition so the outbox flusher
    // cannot run between a switch's drain and its wipe.
    private val transitionGate: SessionTransitionGate = SessionTransitionGate(),
    // Deterministic injection points for tests.
    private val newProfileId: () -> String = { UUID.randomUUID().toString() },
    private val labelForUrl: (String) -> String = ::defaultProfileLabel
) : SessionManager {

    // Issue #2050: BaseUrlInterceptor and AuthInterceptor read these fields at
    // different points in the OkHttp chain, from different threads. Volatile
    // gives each read current visibility; the identity transitions below swap
    // them in an order that fails closed (no bearer) rather than pairing a new
    // origin with a previous profile's token.
    @Volatile
    private var cachedToken: String? = null

    @Volatile
    private var cachedProfiles: List<ServerProfile> = emptyList()

    @Volatile
    private var cachedActiveProfileId: String? = null

    private val sessionState = MutableStateFlow(SessionState())
    private val profilesState = MutableStateFlow<List<ServerProfile>>(emptyList())
    private val activeProfileState = MutableStateFlow<ServerProfile?>(null)
    private val hydrated = kotlinx.coroutines.CompletableDeferred<Unit>()
    private val clearingSession = AtomicBoolean(false)

    // Issue #2051: every transition that establishes, replaces or ends the
    // active session advances this epoch. A grant refresh or Local re-mint
    // captures the epoch when its 401 arrives and may only write while the
    // epoch is unchanged, so a refresh that outlives logout cannot resurrect
    // the session.
    private val sessionEpoch = AtomicLong(0L)

    // Serializes session-identity transitions and guarded token writes, so a
    // refresh's check-then-write cannot interleave with clearSession's wipe.
    private val stateMutex = Mutex()

    // Issue #2065: transitions take the outbox gate first, then stateMutex (the
    // flusher only ever takes the gate, so the order cannot deadlock).
    private suspend fun <T> transition(block: suspend () -> T): T =
        transitionGate.mutex.withLock { stateMutex.withLock { block() } }

    /** Load the profile list, migrate a legacy install if needed, surface initial state. */
    suspend fun init() {
        stateMutex.withLock {
            val snapshot = prefsStorage.loadProfiles()
            cachedProfiles = snapshot.profiles
            cachedActiveProfileId =
                snapshot.activeProfileId?.takeIf { id -> cachedProfiles.any { it.id == id } }

            // ADR 0028 Decision 1: a pre-profiles install (server_url + jwt in the
            // legacy slots) becomes one active Remote profile, no re-login.
            if (cachedProfiles.isEmpty()) {
                migrateLegacyInstall()
            }

            cachedToken = cachedActiveProfileId?.let { tokenStorage.load(it) }
            refreshProfiles()
            refreshState()
            hydrated.complete(Unit)
        }
    }

    /**
     * Moves a legacy `server_url` + `jwt` (+ device grant, OIDC pending) onto a
     * freshly-minted Remote profile. No-op on a fresh install or when there is
     * no legacy server URL to anchor a profile to.
     */
    private suspend fun migrateLegacyInstall() {
        val legacyUrl = prefsStorage.loadServerUrl()?.trim()?.takeIf { it.isNotBlank() } ?: return

        val id = newProfileId()
        val profile = ServerProfile(
            id = id,
            kind = ServerProfileKind.Remote(legacyUrl),
            label = labelForUrl(legacyUrl)
        )
        cachedProfiles = listOf(profile)
        cachedActiveProfileId = id
        persistProfiles()
        // The legacy key has been folded into the profile list — drop it so a
        // later boot cannot re-migrate a stale copy.
        prefsStorage.save(null)

        tokenStorage.loadLegacy()?.let { tokenStorage.save(id, it) }
        tokenStorage.clearLegacy()
        profileSecretStorage.migrateLegacy(id)
    }

    override suspend fun awaitHydrated() {
        hydrated.await()
    }

    override fun bearerToken(): String? = cachedToken

    override fun baseUrl(): String = profileBaseUrl(activeProfileSync()).orEmpty()

    override fun observeSession(): Flow<SessionState> = sessionState

    /** Raw server URL flow for callers that need the current value. */
    fun serverUrlFlow(): Flow<String?> = sessionState.map { it.serverUrl }

    override suspend fun serverUrl(): String? = profileBaseUrl(activeProfile())

    override suspend fun token(): String? = cachedToken

    override suspend fun profiles(): List<ServerProfile> = cachedProfiles

    override fun observeProfiles(): Flow<List<ServerProfile>> = profilesState

    override suspend fun activeProfile(): ServerProfile? =
        cachedProfiles.find { it.id == cachedActiveProfileId }

    override fun observeActiveProfile(): Flow<ServerProfile?> = activeProfileState

    override suspend fun activeProfileId(): String? = cachedActiveProfileId

    override suspend fun setServerUrl(serverUrl: String) {
        stateMutex.withLock {
            val trimmed = serverUrl.trim().trimEnd('/')
            val active = activeProfile()
            if (active == null) {
                // No profile yet: the first URL typed on the Auth screen creates
                // one (the login that follows persists its token onto it).
                val profile = ServerProfile(
                    id = newProfileId(),
                    kind = ServerProfileKind.Remote(trimmed),
                    label = labelForUrl(trimmed)
                )
                cachedProfiles = cachedProfiles + profile
                cachedActiveProfileId = profile.id
                persistProfiles()
                refreshProfiles()
                refreshState()
                return@withLock
            }
            // An existing active profile: re-point it in place. The label is the
            // user's (possibly renamed) metadata — editing the URL must not
            // silently rewrite it. The one exception (issue #1769): a label that
            // still equals the auto-derived label of the previous URL was never
            // renamed, so it follows the URL (the Auth screens call this on every
            // keystroke, so the first character would otherwise stick as the label).
            if (active.remoteUrl != trimmed) {
                val previousUrl = active.remoteUrl
                val labelIsDerived = previousUrl != null && active.label == labelForUrl(previousUrl)
                cachedProfiles = cachedProfiles.map { profile ->
                    if (profile.id == active.id) {
                        profile.copy(
                            kind = ServerProfileKind.Remote(trimmed),
                            label = if (labelIsDerived) labelForUrl(trimmed) else profile.label
                        )
                    } else {
                        profile
                    }
                }
                persistProfiles()
                refreshProfiles()
                refreshState()
            }
        }
    }

    override suspend fun setSession(serverUrl: String, token: String, state: SessionState) {
        stateMutex.withLock {
            // A full session implies an active profile; if the Auth screen somehow
            // skipped setServerUrl (or a Local profile was active), anchor one now.
            val activeId = cachedActiveProfileId ?: run {
                val profile = ServerProfile(
                    id = newProfileId(),
                    kind = ServerProfileKind.Remote(serverUrl.trim().trimEnd('/')),
                    label = labelForUrl(serverUrl.trim().trimEnd('/'))
                )
                cachedProfiles = cachedProfiles + profile
                cachedActiveProfileId = profile.id
                profile.id
            }

            // Issue #2051: an interactive login is a new session identity, so a
            // refresh that started before it cannot overwrite its token.
            sessionEpoch.incrementAndGet()
            cachedToken = token
            tokenStorage.save(activeId, token)
            persistProfiles()
            refreshProfiles()
            sessionState.value = state.copy(
                serverUrl = profileBaseUrl(activeProfileSync()),
                isLoggedIn = true
            )
        }
    }

    override suspend fun setProfile(profile: ProfileUpdate) {
        val current = sessionState.value
        sessionState.value = current.copy(
            userId = profile.userId ?: current.userId,
            username = profile.username ?: current.username,
            // Issue #1767: null means "unchanged" like every other field. A
            // partial update (language, date format, enabled fields) must not
            // clear the admin flag; only a fetched user states it explicitly.
            isAdmin = profile.isAdmin ?: current.isAdmin,
            language = profile.language ?: current.language,
            dateFormat = profile.dateFormat ?: current.dateFormat,
            enabledContactFields = profile.enabledContactFields ?: current.enabledContactFields
        )
    }

    override suspend fun setSelfContactVCardUid(vcardUid: String?) {
        sessionState.value = sessionState.value.copy(selfContactVCardUid = vcardUid)
    }

    override suspend fun setToken(token: String) {
        stateMutex.withLock {
            // A re-issued token belongs to the current session. If logout (or a
            // switch) already ended it, writing here would resurrect the ended
            // session — refuse and let the caller keep its logged-out state.
            if (cachedToken == null) return@withLock
            cachedToken = token
            cachedActiveProfileId?.let { tokenStorage.save(it, token) }
        }
    }

    override fun sessionGeneration(): Long = sessionEpoch.get()

    override suspend fun setTokenIfCurrent(token: String, expectedGeneration: Long): Boolean =
        stateMutex.withLock {
            if (sessionEpoch.get() != expectedGeneration || cachedToken == null) {
                return@withLock false
            }
            val activeId = cachedActiveProfileId ?: return@withLock false
            cachedToken = token
            tokenStorage.save(activeId, token)
            true
        }

    override suspend fun clearSession(keepServerUrl: Boolean) {
        transition {
            // Issue #2051: keep the clearing guard up for the whole teardown and
            // wipe, not just the teardown call. A 401 that arrives after the
            // teardown step but before the token is dropped must not start a
            // refresh that writes a credential into a session being ended.
            clearingSession.set(true)
            try {
                // Issue #957: run the authenticated teardown step (FCM
                // deregistration, server-side session revoke) BEFORE the bearer
                // below is dropped — both need to reach the server as this session,
                // and any request after this point goes out with no Authorization
                // header. Skipped entirely when there is no session to tear down
                // (already logged out, or a redundant/racing clearSession call), so
                // a repeat call makes no network request. Best-effort: a teardown
                // failure must never block the local clear below.
                if (cachedToken != null) {
                    runCatching { sessionTeardown.beforeClear() }
                }

                // Advance the epoch and drop the token under the same lock that
                // guards a refresh's check-then-write: a device-grant refresh or
                // Local re-mint that captured the old epoch is rejected instead of
                // persisting a fresh credential after this logout.
                sessionEpoch.incrementAndGet()
                cachedToken = null
                cachedActiveProfileId?.let { tokenStorage.clear(it) }
                // Issue #723: logout keeps the server profile (issue #723 / ADR 0028):
                // it is non-credential device config, so the login screen can pre-fill
                // it. `keepServerUrl = false` means "forget this server", which now
                // removes the active profile and its per-profile secrets entirely.
                if (!keepServerUrl) {
                    val removedId = cachedActiveProfileId
                    cachedProfiles = cachedProfiles.filterNot { it.id == removedId }
                    cachedActiveProfileId = cachedProfiles.firstOrNull()?.id
                    if (removedId != null) profileSecretStorage.clear(removedId)
                    persistProfiles()
                    refreshProfiles()
                }
                // Issue #385: purge the offline PII mirror + image cache on logout /
                // account removal so a dropped session leaves no contact data on disk.
                localDataCleaner.clear()
                // Issue #2065: forgetting the server promotes another profile; load
                // its credential rather than leaving it active but token-less.
                if (!keepServerUrl) {
                    cachedToken = cachedActiveProfileId?.let { tokenStorage.load(it) }
                }
                sessionState.value = SessionState(
                    serverUrl = profileBaseUrl(activeProfileSync()),
                    isLoggedIn = !cachedToken.isNullOrBlank()
                )
                if (cachedActiveProfileId == null) {
                    // No profile left: don't keep a stale active pointer in the flow.
                    activeProfileState.value = null
                }
            } finally {
                clearingSession.set(false)
            }
        }
    }

    override fun isClearingSession(): Boolean = clearingSession.get()

    // --- ADR 0028 Decision 1: server profiles ---------------------------------

    override suspend fun addRemoteProfile(label: String, url: String): ServerProfile {
        val trimmed = url.trim().trimEnd('/')
        val profile = ServerProfile(
            id = newProfileId(),
            kind = ServerProfileKind.Remote(trimmed),
            label = label.ifBlank { labelForUrl(trimmed) }
        )
        cachedProfiles = cachedProfiles + profile
        persistProfiles()
        refreshProfiles()
        return profile
    }

    override suspend fun activateLocalProfile(token: String): ServerProfile =
        transition { activateLocalProfileLocked(token) }

    override suspend fun activateLocalProfileIfCurrent(
        token: String,
        expectedGeneration: Long
    ): ServerProfile? = transition {
        if (sessionEpoch.get() != expectedGeneration || cachedToken == null) {
            return@transition null
        }
        activateLocalProfileLocked(token)
    }

    private suspend fun activateLocalProfileLocked(token: String): ServerProfile {
        val existing = cachedProfiles.find { it.kind is ServerProfileKind.Local }
        val profile = existing ?: ServerProfile(
            id = newProfileId(),
            kind = ServerProfileKind.Local,
            label = LOCAL_PROFILE_LABEL
        )
        if (existing == null) {
            cachedProfiles = cachedProfiles + profile
        }
        // Issue #2050: clear the previous profile's bearer before the origin can
        // change, so a request racing the swap sees no token rather than the
        // previous profile's credential against the new origin.
        cachedToken = null
        if (cachedActiveProfileId != profile.id) {
            // The Room mirror belongs to the profile being left (a cache), just
            // as on any switch; the Local server's own files are untouched.
            if (cachedActiveProfileId != null) localDataCleaner.clear()
            cachedActiveProfileId = profile.id
        }
        // Issue #2051: a Local activation is a new session identity.
        sessionEpoch.incrementAndGet()
        cachedToken = token
        tokenStorage.save(profile.id, token)
        persistProfiles()
        refreshProfiles()
        // serverUrl is the sentinel so the root's compatibility/capability check
        // fetches /health over the local socket, exactly as a Remote profile
        // fetches its server's.
        sessionState.value = SessionState(serverUrl = LOCAL_SERVER_SENTINEL_URL, isLoggedIn = true)
        return profile
    }

    override suspend fun renameProfile(id: String, label: String) {
        if (label.isBlank()) return
        val current = cachedProfiles.find { it.id == id } ?: return
        cachedProfiles = cachedProfiles.map {
            if (it.id == id) {
                it.copy(
                    label = label.trim()
                )
            } else {
                it
            }
        }
        persistProfiles()
        refreshProfiles()
        // Keep the active-profile flow in sync when the active one was renamed.
        if (current.id == cachedActiveProfileId) {
            activeProfileState.value = cachedProfiles.find { it.id == id }
        }
    }

    override suspend fun removeProfile(id: String) {
        transition {
            if (cachedProfiles.none { it.id == id }) return@transition
            val wasActive = id == cachedActiveProfileId

            // Revoke the session while it is still the active bearer (the teardown
            // step's requests read bearerToken()). Only possible for the active
            // profile — a non-active profile's token is dropped without a server
            // round-trip (there is no bearer to authenticate one).
            if (wasActive && cachedToken != null) {
                clearingSession.set(true)
                try {
                    runCatching { sessionTeardown.beforeClear() }
                } finally {
                    clearingSession.set(false)
                }
            }

            tokenStorage.clear(id)
            profileSecretStorage.clear(id)
            cachedProfiles = cachedProfiles.filterNot { it.id == id }
            if (wasActive) {
                // Issue #2051: ending the active session advances the epoch so a
                // refresh in flight for it cannot write into the next profile.
                sessionEpoch.incrementAndGet()
                cachedToken = null
                cachedActiveProfileId = cachedProfiles.firstOrNull()?.id
                localDataCleaner.clear()
                // Issue #2065: the remaining profile becomes active, so its stored
                // credential must be loaded now — switchProfile early-returns for
                // the already-active id and would never do it.
                cachedToken = cachedActiveProfileId?.let { tokenStorage.load(it) }
                sessionState.value = SessionState(
                    serverUrl = profileBaseUrl(activeProfileSync()),
                    isLoggedIn = !cachedToken.isNullOrBlank()
                )
            }
            persistProfiles()
            refreshProfiles()
        }
    }

    override suspend fun pendingInteractionCount(): Int = pendingInteractions?.unsynced()?.size ?: 0

    override suspend fun switchProfile(id: String, discardPending: Boolean): SwitchProfileResult =
        transition {
            val target = cachedProfiles.find { it.id == id }
                ?: return@transition SwitchProfileResult.Switched
            if (id == cachedActiveProfileId) return@transition SwitchProfileResult.Switched

            // 1. Best-effort drain (ADR 0028 Decision 1). A no-op drainer leaves
            //    the rows in place and routes to the confirmation below.
            if (pendingInteractionCount() > 0) {
                runCatching { outboxDrainer.drain() }
                val remaining = pendingInteractionCount()
                if (remaining > 0 && !discardPending) {
                    return@transition SwitchProfileResult.NeedsConfirmation(remaining)
                }
            }

            // 2. The Room mirror is a cache owned by the profile being left.
            localDataCleaner.clear()

            // 3. Load the target token BEFORE changing the origin, then swap the
            //    pair with the old bearer cleared first. Issue #2050: a request
            //    racing this swap can see no bearer, but never the new origin
            //    paired with the previous profile's token.
            val newToken = tokenStorage.load(id)
            cachedToken = null
            cachedActiveProfileId = id
            // Issue #2051: a refresh that started for the previous profile must
            // not write its token into the newly active one.
            sessionEpoch.incrementAndGet()
            cachedToken = newToken
            persistProfiles()
            refreshProfiles()

            // 4. Re-emit the session: MainViewModel re-runs the compatibility gate
            //    because the server URL changed.
            sessionState.value = SessionState(
                serverUrl = profileBaseUrl(target),
                isLoggedIn = !newToken.isNullOrBlank()
            )
            SwitchProfileResult.Switched
        }

    override suspend fun saveProfileToken(id: String, token: String) {
        stateMutex.withLock {
            if (cachedProfiles.none { it.id == id }) return@withLock
            tokenStorage.save(id, token)
            // The active profile's in-memory bearer must follow its stored one.
            if (id == cachedActiveProfileId) cachedToken = token
        }
    }

    override suspend fun setProfileArchived(id: String, archived: Boolean) {
        val current = cachedProfiles.find { it.id == id } ?: return
        // Only the on-device store has data that can be "moved"; a Remote
        // profile is never a read-only archive.
        if (current.kind !is ServerProfileKind.Local) return
        if (current.archived == archived) return
        cachedProfiles = cachedProfiles.map {
            if (it.id == id) {
                it.copy(
                    archived = archived
                )
            } else {
                it
            }
        }
        persistProfiles()
        refreshProfiles()
    }

    override fun isActiveProfileArchived(): Boolean = activeProfileSync()?.archived == true

    // --- internals ------------------------------------------------------------

    private suspend fun persistProfiles() {
        prefsStorage.saveProfiles(
            ProfilesSnapshot(profiles = cachedProfiles, activeProfileId = cachedActiveProfileId)
        )
    }

    private fun refreshProfiles() {
        profilesState.value = cachedProfiles
        activeProfileState.value = activeProfileSync()
    }

    private fun activeProfileSync(): ServerProfile? =
        cachedProfiles.find { it.id == cachedActiveProfileId }

    /**
     * The origin a profile's requests are addressed to: the Remote URL, or the
     * sentinel host the profile-aware transport routes over the embedded Unix
     * socket for a [ServerProfileKind.Local] profile.
     */
    private fun profileBaseUrl(profile: ServerProfile?): String? = when (val kind = profile?.kind) {
        is ServerProfileKind.Remote -> kind.url
        ServerProfileKind.Local -> LOCAL_SERVER_SENTINEL_URL
        null -> null
    }

    private fun refreshState() {
        sessionState.value = SessionState(
            serverUrl = profileBaseUrl(activeProfileSync()),
            isLoggedIn = !cachedToken.isNullOrBlank()
        )
    }
}
