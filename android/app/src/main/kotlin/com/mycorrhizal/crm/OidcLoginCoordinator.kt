package com.mycorrhizal.crm

import com.mycorrhizal.crm.data.session.OidcPendingRequestStore
import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.SessionState
import java.net.URLEncoder
import java.nio.charset.StandardCharsets

/** The outcome of handling an OIDC native-return deep link. */
internal enum class OidcCallbackOutcome {
    /** The URI was not this app's OIDC callback; nothing happened. */
    Ignored,

    /** The callback was rejected (or the exchange failed); show the error. */
    Failed,

    /** The session was redeemed and persisted. */
    Succeeded,
}

/**
 * Issue #965: the orchestration behind the Android OIDC native flow, factored
 * out of [MainActivity] so the security-critical decisions are unit-testable
 * without an Activity:
 *
 *  - only a callback whose `state` matches a request *this app* started is
 *    redeemed, and only while that request is within its TTL;
 *  - the callback's single-use code is redeemed with the PKCE verifier that
 *    never left the device, so an interceptor of the interceptable
 *    `mycorrhizal://` redirect gets nothing usable;
 *  - the browser is opened with only the state + S256 challenge, never the
 *    verifier.
 */
internal class OidcLoginCoordinator(
    private val sessionManager: SessionManager,
    private val authRepository: AuthRepository,
    private val pendingStore: OidcPendingRequestStore,
    private val launchBrowser: (String) -> Unit,
    private val nowMillis: () -> Long = System::currentTimeMillis,
) {

    /**
     * Generate a fresh state nonce + PKCE verifier, persist them, and open the
     * backend's OIDC login URL with only the state and the S256 challenge.
     */
    suspend fun start(serverUrl: String) {
        val state = OidcPkce.generateState()
        val verifier = OidcPkce.generateVerifier()
        pendingStore.save(state, verifier)
        // The verifier never goes in the URL — only its one-way S256 hash.
        val url = serverUrl.trim().trimEnd('/') +
            "/api/v1/auth/oidc/login?client=android" +
            "&state=" + encode(state) +
            "&code_challenge=" + encode(OidcPkce.challenge(verifier)) +
            "&code_challenge_method=S256"
        launchBrowser(url)
    }

    /**
     * Handle a deep link already parsed by [parseOidcReturn]. [result] is null
     * when the URI is not this app's OIDC callback.
     */
    suspend fun onCallback(result: OidcReturn?): OidcCallbackOutcome = when (result) {
        null -> OidcCallbackOutcome.Ignored

        is OidcReturn.Failure -> {
            // Best-effort clear so a later callback cannot match a stale
            // pending state; the flow is over either way.
            pendingStore.clear()
            OidcCallbackOutcome.Failed
        }

        is OidcReturn.Success -> redeem(result)
    }

    private suspend fun redeem(result: OidcReturn.Success): OidcCallbackOutcome {
        // A cold-start return reads the persisted session asynchronously at
        // startup; without awaiting it the server URL could still be null.
        sessionManager.awaitHydrated()
        val serverUrl = sessionManager.serverUrl()
        if (serverUrl.isNullOrBlank()) return OidcCallbackOutcome.Failed

        // Accept only a callback bound to a live request this app started. The
        // pending request is consumed whatever the outcome.
        val pending = pendingStore.load()
        pendingStore.clear()
        if (pending == null ||
            pending.isExpired(nowMillis()) ||
            !OidcPkce.verifyState(pending.state, result.state)
        ) {
            return OidcCallbackOutcome.Failed
        }

        val token = authRepository
            .completeOidcNativeLogin(result.code, pending.codeVerifier)
            .getOrElse { return OidcCallbackOutcome.Failed }

        sessionManager.setSession(
            serverUrl = serverUrl,
            token = token,
            state = SessionState(language = result.language, dateFormat = result.dateFormat),
        )

        // Validate the JWT against the server and enrich the profile the way a
        // normal login does; a stale/expired token never flips to logged-in.
        val profile = authRepository.fetchCurrentUser()
        return profile.fold(
            onSuccess = { user ->
                sessionManager.setProfile(
                    SessionState(
                        userId = user.id.takeIf { it != 0 },
                        username = user.username,
                        isAdmin = user.isAdmin,
                    ),
                )
                OidcCallbackOutcome.Succeeded
            },
            onFailure = {
                sessionManager.clearSession()
                OidcCallbackOutcome.Failed
            },
        )
    }

    private fun encode(value: String): String =
        // The String-charset overload (API 1); the Charset overload is API 33.
        URLEncoder.encode(value, StandardCharsets.UTF_8.name())
}
