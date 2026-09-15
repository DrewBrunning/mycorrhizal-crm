package com.mycorrhizal.crm.data.session

import com.mycorrhizal.crm.domain.repository.SessionState
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.map
import java.util.concurrent.atomic.AtomicBoolean

/**
 * In-memory-backed [SessionManager] used by unit tests and as the base for
 * the production Android implementation. Holds the JWT and server URL in
 * memory; persistence is delegated to the injected [TokenStorage] and
 * [SessionPrefsStorage] so the class itself is a plain JVM object.
 */
class DefaultSessionManager(
    private val tokenStorage: TokenStorage,
    private val prefsStorage: SessionPrefsStorage,
    private val localDataCleaner: SessionDataCleaner = NoopSessionDataCleaner,
    private val sessionTeardown: SessionTeardown = NoopSessionTeardown,
) : SessionManager {

    private var cachedToken: String? = null
    private var cachedServerUrl: String? = null
    private val sessionState = MutableStateFlow(SessionState())
    private val hydrated = kotlinx.coroutines.CompletableDeferred<Unit>()
    private val clearingSession = AtomicBoolean(false)

    /** Load both cached values and surface the initial session. */
    suspend fun init() {
        cachedToken = tokenStorage.load()
        cachedServerUrl = prefsStorage.loadServerUrl()
        refreshState()
        hydrated.complete(Unit)
    }

    override suspend fun awaitHydrated() {
        hydrated.await()
    }

    override fun bearerToken(): String? = cachedToken

    override fun baseUrl(): String = cachedServerUrl.orEmpty()

    override fun observeSession(): Flow<SessionState> = sessionState

    /** Raw server URL flow for callers that need the current value. */
    fun serverUrlFlow(): Flow<String?> = sessionState.map { it.serverUrl }

    override suspend fun serverUrl(): String? = cachedServerUrl

    override suspend fun token(): String? = cachedToken

    override suspend fun setServerUrl(serverUrl: String) {
        cachedServerUrl = serverUrl
        prefsStorage.save(serverUrl)
        sessionState.value = sessionState.value.copy(serverUrl = serverUrl)
    }

    override suspend fun setSession(serverUrl: String, token: String, state: SessionState) {
        cachedServerUrl = serverUrl
        cachedToken = token
        prefsStorage.save(serverUrl)
        tokenStorage.save(token)
        sessionState.value = state.copy(serverUrl = serverUrl, isLoggedIn = true)
    }

    override suspend fun setProfile(profile: SessionState) {
        val current = sessionState.value
        sessionState.value = current.copy(
            userId = profile.userId ?: current.userId,
            username = profile.username ?: current.username,
            isAdmin = profile.isAdmin,
            language = profile.language ?: current.language,
            dateFormat = profile.dateFormat ?: current.dateFormat,
        )
    }

    override suspend fun setToken(token: String) {
        cachedToken = token
        tokenStorage.save(token)
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
        tokenStorage.clear()
        // Issue #723: the server URL is device config, not a session secret —
        // keep it (in memory AND persisted) across logout so the login screen
        // can pre-fill it. Only an explicit keepServerUrl=false wipes it.
        if (!keepServerUrl) {
            cachedServerUrl = null
            prefsStorage.clear()
        }
        // Issue #385: purge the offline PII mirror + image cache on logout /
        // account removal so a dropped session leaves no contact data on disk.
        localDataCleaner.clear()
        sessionState.value = if (keepServerUrl) {
            SessionState(serverUrl = cachedServerUrl)
        } else {
            SessionState()
        }
    }

    override fun isClearingSession(): Boolean = clearingSession.get()

    private fun refreshState() {
        sessionState.value = SessionState(
            serverUrl = cachedServerUrl,
            isLoggedIn = !cachedToken.isNullOrBlank(),
        )
    }
}
