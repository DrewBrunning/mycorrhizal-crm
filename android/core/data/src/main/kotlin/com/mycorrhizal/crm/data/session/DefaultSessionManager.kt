package com.mycorrhizal.crm.data.session

import com.mycorrhizal.crm.domain.profile.ServerProfile
import com.mycorrhizal.crm.domain.profile.ServerProfileKind
import com.mycorrhizal.crm.domain.profile.defaultProfileLabel
import com.mycorrhizal.crm.domain.repository.PendingInteractionRepository
import com.mycorrhizal.crm.domain.repository.SessionState
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.map
import java.util.UUID
import java.util.concurrent.atomic.AtomicBoolean

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
    // Deterministic injection points for tests.
    private val newProfileId: () -> String = { UUID.randomUUID().toString() },
    private val labelForUrl: (String) -> String = ::defaultProfileLabel,
) : SessionManager {

    private var cachedToken: String? = null
    private var cachedProfiles: List<ServerProfile> = emptyList()
    private var cachedActiveProfileId: String? = null

    private val sessionState = MutableStateFlow(SessionState())
    private val profilesState = MutableStateFlow<List<ServerProfile>>(emptyList())
    private val activeProfileState = MutableStateFlow<ServerProfile?>(null)
    private val hydrated = kotlinx.coroutines.CompletableDeferred<Unit>()
    private val clearingSession = AtomicBoolean(false)

    /** Load the profile list, migrate a legacy install if needed, surface initial state. */
    suspend fun init() {
        val snapshot = prefsStorage.loadProfiles()
        cachedProfiles = snapshot.profiles
        cachedActiveProfileId = snapshot.activeProfileId?.takeIf { id -> cachedProfiles.any { it.id == id } }

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
            label = labelForUrl(legacyUrl),
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

    override fun baseUrl(): String = activeProfileSync()?.remoteUrl.orEmpty()

    override fun observeSession(): Flow<SessionState> = sessionState

    /** Raw server URL flow for callers that need the current value. */
    fun serverUrlFlow(): Flow<String?> = sessionState.map { it.serverUrl }

    override suspend fun serverUrl(): String? = activeProfile()?.remoteUrl

    override suspend fun token(): String? = cachedToken

    override suspend fun profiles(): List<ServerProfile> = cachedProfiles

    override fun observeProfiles(): Flow<List<ServerProfile>> = profilesState

    override suspend fun activeProfile(): ServerProfile? =
        cachedProfiles.find { it.id == cachedActiveProfileId }

    override fun observeActiveProfile(): Flow<ServerProfile?> = activeProfileState

    override suspend fun activeProfileId(): String? = cachedActiveProfileId

    override suspend fun setServerUrl(serverUrl: String) {
        val trimmed = serverUrl.trim().trimEnd('/')
        val active = activeProfile()
        if (active == null) {
            // No profile yet: the first URL typed on the Auth screen creates
            // one (the login that follows persists its token onto it).
            val profile = ServerProfile(
                id = newProfileId(),
                kind = ServerProfileKind.Remote(trimmed),
                label = labelForUrl(trimmed),
            )
            cachedProfiles = cachedProfiles + profile
            cachedActiveProfileId = profile.id
            persistProfiles()
            refreshProfiles()
            refreshState()
            return
        }
        // An existing active profile: re-point it in place. The label is the
        // user's (possibly renamed) metadata — editing the URL must not
        // silently rewrite it.
        if (active.remoteUrl != trimmed) {
            cachedProfiles = cachedProfiles.map { profile ->
                if (profile.id == active.id) {
                    profile.copy(kind = ServerProfileKind.Remote(trimmed))
                } else {
                    profile
                }
            }
            persistProfiles()
            refreshProfiles()
            refreshState()
        }
    }

    override suspend fun setSession(serverUrl: String, token: String, state: SessionState) {
        // A full session implies an active profile; if the Auth screen somehow
        // skipped setServerUrl (or a Local profile was active), anchor one now.
        val activeId = cachedActiveProfileId ?: run {
            val profile = ServerProfile(
                id = newProfileId(),
                kind = ServerProfileKind.Remote(serverUrl.trim().trimEnd('/')),
                label = labelForUrl(serverUrl.trim().trimEnd('/')),
            )
            cachedProfiles = cachedProfiles + profile
            cachedActiveProfileId = profile.id
            profile.id
        }

        cachedToken = token
        tokenStorage.save(activeId, token)
        persistProfiles()
        refreshProfiles()
        sessionState.value = state.copy(
            serverUrl = activeProfileSync()?.remoteUrl,
            isLoggedIn = true,
        )
    }

    override suspend fun setProfile(profile: SessionState) {
        val current = sessionState.value
        sessionState.value = current.copy(
            userId = profile.userId ?: current.userId,
            username = profile.username ?: current.username,
            isAdmin = profile.isAdmin,
            language = profile.language ?: current.language,
            dateFormat = profile.dateFormat ?: current.dateFormat,
            enabledContactFields = profile.enabledContactFields ?: current.enabledContactFields,
        )
    }

    override suspend fun setSelfContactVCardUid(vcardUid: String?) {
        sessionState.value = sessionState.value.copy(selfContactVCardUid = vcardUid)
    }

    override suspend fun setToken(token: String) {
        cachedToken = token
        cachedActiveProfileId?.let { tokenStorage.save(it, token) }
    }

    override suspend fun clearSession(keepServerUrl: Boolean) {
        // Issue #957: run the authenticated teardown step (FCM
        // deregistration, server-side session revoke) BEFORE the bearer
        // below is dropped — both need to reach the server as this session,
        // and any request after this point goes out with no Authorization
        // header. Skipped entirely when there is no session to tear down
        // (already logged out, or a redundant/racing clearSession call), so
        // a repeat call makes no network request.
        //
        // clearingSession is up only for the duration of this call so
        // SessionExpiryWiring can tell a 401 caused by the teardown step's
        // own request (the bearer it used is being invalidated right now)
        // apart from an unrelated 401 — the former must not race a
        // device-grant refresh that would silently resurrect the session
        // this call is in the middle of ending. Best-effort: a teardown
        // failure must never block the local clear below.
        if (cachedToken != null) {
            clearingSession.set(true)
            try {
                runCatching { sessionTeardown.beforeClear() }
            } finally {
                clearingSession.set(false)
            }
        }
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
        sessionState.value = SessionState(serverUrl = activeProfile()?.remoteUrl)
        if (cachedActiveProfileId == null) {
            // No profile left: don't keep a stale active pointer in the flow.
            activeProfileState.value = null
        }
    }

    override fun isClearingSession(): Boolean = clearingSession.get()

    // --- ADR 0028 Decision 1: server profiles ---------------------------------

    override suspend fun addRemoteProfile(label: String, url: String): ServerProfile {
        val trimmed = url.trim().trimEnd('/')
        val profile = ServerProfile(
            id = newProfileId(),
            kind = ServerProfileKind.Remote(trimmed),
            label = label.ifBlank { labelForUrl(trimmed) },
        )
        cachedProfiles = cachedProfiles + profile
        persistProfiles()
        refreshProfiles()
        return profile
    }

    override suspend fun renameProfile(id: String, label: String) {
        if (label.isBlank()) return
        val current = cachedProfiles.find { it.id == id } ?: return
        cachedProfiles = cachedProfiles.map { if (it.id == id) it.copy(label = label.trim()) else it }
        persistProfiles()
        refreshProfiles()
        // Keep the active-profile flow in sync when the active one was renamed.
        if (current.id == cachedActiveProfileId) {
            activeProfileState.value = cachedProfiles.find { it.id == id }
        }
    }

    override suspend fun removeProfile(id: String) {
        if (cachedProfiles.none { it.id == id }) return
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
            cachedToken = null
            cachedActiveProfileId = cachedProfiles.firstOrNull()?.id
            localDataCleaner.clear()
            sessionState.value = SessionState(serverUrl = activeProfile()?.remoteUrl)
        }
        persistProfiles()
        refreshProfiles()
    }

    override suspend fun pendingInteractionCount(): Int =
        pendingInteractions?.unsynced()?.size ?: 0

    override suspend fun switchProfile(id: String, discardPending: Boolean): SwitchProfileResult {
        val target = cachedProfiles.find { it.id == id } ?: return SwitchProfileResult.Switched
        if (id == cachedActiveProfileId) return SwitchProfileResult.Switched

        // 1. Best-effort drain (ADR 0028 Decision 1). A no-op drainer leaves
        //    the rows in place and routes to the confirmation below.
        if (pendingInteractionCount() > 0) {
            runCatching { outboxDrainer.drain() }
            val remaining = pendingInteractionCount()
            if (remaining > 0 && !discardPending) {
                return SwitchProfileResult.NeedsConfirmation(remaining)
            }
        }

        // 2. The Room mirror is a cache owned by the profile being left.
        localDataCleaner.clear()

        // 3. Activate the new profile and load its own credential.
        cachedActiveProfileId = id
        cachedToken = tokenStorage.load(id)
        persistProfiles()
        refreshProfiles()

        // 4. Re-emit the session: MainViewModel re-runs the compatibility gate
        //    because the server URL changed.
        sessionState.value = SessionState(
            serverUrl = target.remoteUrl,
            isLoggedIn = !cachedToken.isNullOrBlank(),
        )
        return SwitchProfileResult.Switched
    }

    // --- internals ------------------------------------------------------------

    private suspend fun persistProfiles() {
        prefsStorage.saveProfiles(
            ProfilesSnapshot(profiles = cachedProfiles, activeProfileId = cachedActiveProfileId),
        )
    }

    private fun refreshProfiles() {
        profilesState.value = cachedProfiles
        activeProfileState.value = activeProfileSync()
    }

    private fun activeProfileSync(): ServerProfile? =
        cachedProfiles.find { it.id == cachedActiveProfileId }

    private fun refreshState() {
        sessionState.value = SessionState(
            serverUrl = activeProfileSync()?.remoteUrl,
            isLoggedIn = !cachedToken.isNullOrBlank(),
        )
    }
}
