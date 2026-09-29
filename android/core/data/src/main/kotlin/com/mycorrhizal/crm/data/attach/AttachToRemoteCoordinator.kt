package com.mycorrhizal.crm.data.attach

import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.data.session.SwitchProfileResult
import com.mycorrhizal.crm.domain.compat.ServerCapability
import com.mycorrhizal.crm.domain.compat.toCapabilitiesInfo
import com.mycorrhizal.crm.domain.profile.ServerProfile
import com.mycorrhizal.crm.domain.profile.ServerProfileKind
import com.mycorrhizal.crm.model.network.ImportConfirmRequest
import com.mycorrhizal.crm.model.network.MycorrhizalBundleCounts
import com.mycorrhizal.crm.model.network.includesPasskey
import com.mycorrhizal.crm.model.network.SourceImportResult
import com.mycorrhizal.crm.model.network.SourceImportStatus
import com.mycorrhizal.crm.model.network.SourceImportPreviewResponse
import com.mycorrhizal.crm.model.network.RowImportAction
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.BaseUrlProvider
import com.mycorrhizal.crm.network.LoginResult
import com.mycorrhizal.crm.network.ClientVersionProvider
import com.mycorrhizal.crm.network.NetworkFactory
import com.mycorrhizal.crm.network.TokenProvider
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.delay
import java.util.UUID
import javax.inject.Inject

/**
 * Builds an [ApiClient] that talks to one fixed remote server with one fixed
 * credential, independent of the app's active profile. The attach wizard needs
 * it because the `Local` profile stays active (and writable) for the whole flow,
 * so the app-wide client — which follows the active profile — cannot be the one
 * that reaches the remote. The production client deliberately has no session-
 * expiry interceptor: a 401 from the remote during the wizard must not log the
 * user out of the profile they are still using.
 */
fun interface AttachRemoteApiFactory {
    fun create(baseUrl: String, tokenProvider: TokenProvider): ApiClient
}

/** Result of the wizard's sign-in step. */
sealed interface AttachSignInResult {
    data object SignedIn : AttachSignInResult

    /**
     * The account has 2FA on; call [AttachToRemoteCoordinator.completeTwoFactor]
     * (or, when a passkey can be used, [AttachToRemoteCoordinator.beginPasskey] /
     * [AttachToRemoteCoordinator.completePasskey]). [methods] is the server's enrolled
     * factor list (null from an older server = TOTP prompt). [serverOffersPasskeys] is
     * whether THIS remote server's `/health` declares `webauthn_android` — asked of the
     * remote being attached, not the active (Local) profile, and fail-closed; the
     * caller adds the device-side check.
     */
    data class TwoFactorRequired(
        val methods: List<String>? = null,
        val serverOffersPasskeys: Boolean = false,
    ) : AttachSignInResult
}

/** What the wizard is doing during the (long) prepare/import steps, for a progress line. */
enum class AttachStage { Exporting, Uploading, Preparing, Importing }

/** Progress callback payload. [done]/[total] are 0 when the server gave no counts. */
data class AttachProgress(val stage: AttachStage, val done: Int = 0, val total: Int = 0)

/** What [AttachToRemoteCoordinator.prepare] hands the review step. */
data class AttachPreview(
    val totals: MycorrhizalBundleCounts,
    val preview: SourceImportPreviewResponse,
)

/** Outcome of [AttachToRemoteCoordinator.finish]. */
sealed interface AttachFinishResult {
    data object Done : AttachFinishResult

    /** Unsynced interactions on the Local profile would be dropped by the switch. */
    data class NeedsConfirmation(val pendingCount: Int) : AttachFinishResult
}

/** A wizard step failed for a reason the user can act on (message is not localized). */
class AttachException(message: String, cause: Throwable? = null) : Exception(message, cause)

/**
 * ADR 0028 Decision 3 / issue #1265: the one-time "move this local data to a
 * server" flow, as a plain-JVM state machine the wizard's ViewModel drives one
 * step at a time. NOT a singleton — one instance per wizard run, holding that
 * run's in-memory state (the exported bundle, the upload's `Idempotency-Key`,
 * the remote session), which is what lets a retry resume from the first step
 * that did not complete.
 *
 * Invariants (each pinned by a test):
 *  - The `Local` profile stays active and writable until [finish]; every
 *    earlier step, including failure at any of them, leaves it untouched.
 *  - The bundle lives only in memory and is dropped when the run ends
 *    ([close]); nothing is written to disk.
 *  - The upload carries one `Idempotency-Key` for the whole run, so an
 *    ambiguous-failure retry replays instead of opening a second session; a
 *    re-run of the wizard is safe regardless, because the server's
 *    `import_source_links` ledger keys on the bundle's stable IDs.
 *  - Nothing ever flows remote → local (ADR 0028: no two-way sync).
 */
class AttachToRemoteCoordinator @Inject constructor(
    private val sessionManager: SessionManager,
    private val localApi: ApiClient,
    private val remoteApiFactory: AttachRemoteApiFactory,
) {
    /** Poll interval and cap; overridable so tests do not sleep. */
    internal var pollIntervalMs: Long = POLL_INTERVAL_MS
    internal var maxPolls: Int = MAX_POLLS
    internal var newKey: () -> String = { UUID.randomUUID().toString() }

    private var remoteProfileId: String? = null
    private var remoteApi: ApiClient? = null
    private var remoteToken: String? = null
    private var pendingTwoFactorCookie: String? = null
    private var pendingLabel: String = ""

    // Resumable step memo — cleared by close() or when the remote session dies.
    private var bundle: ByteArray? = null
    private var uploadKey: String? = null
    private var sessionId: String? = null
    private var totals: MycorrhizalBundleCounts? = null
    private var fetchStarted = false

    /**
     * Step 1: log into a Remote profile WITHOUT activating it. [existingProfileId]
     * picks a configured Remote profile; otherwise a profile is created for [url]
     * (reused across retries, so a failed login does not leak one per attempt).
     */
    suspend fun signIn(
        existingProfileId: String?,
        label: String,
        url: String,
        identifier: String,
        password: String,
    ): Result<AttachSignInResult> {
        val profile = resolveRemoteProfile(existingProfileId, label, url)
            ?: return Result.failure(AttachException("Not a remote server profile"))
        val baseUrl = profile.remoteUrl.orEmpty()
        val api = remoteApiFactory.create(baseUrl) { remoteToken }
        remoteApi = api
        remoteToken = null
        pendingLabel = profile.label

        val login = api.login(identifier, password).getOrElse { return Result.failure(it) }
        return when {
            login.twoFactorRequired -> {
                pendingTwoFactorCookie = login.pending2faCookie
                val offers = login.methods.includesPasskey() &&
                    api.getHealth().getOrNull()?.toCapabilitiesInfo()?.declares(ServerCapability.WEBAUTHN_ANDROID) == true
                Result.success(AttachSignInResult.TwoFactorRequired(login.methods, offers))
            }
            login.token.isNullOrBlank() -> Result.failure(AttachException("Server returned no session"))
            else -> {
                storeToken(profile.id, login.token.orEmpty())
                Result.success(AttachSignInResult.SignedIn)
            }
        }
    }

    /** Step 1b: finish a 2FA challenge started by [signIn]. */
    suspend fun completeTwoFactor(code: String): Result<AttachSignInResult> {
        val cookie = pendingTwoFactorCookie ?: return Result.failure(AttachException("No pending 2FA challenge"))
        val api = remoteApi ?: return Result.failure(AttachException("Not signed in"))
        return finishTwoFactor(api.complete2faLogin(code, cookie))
    }

    /**
     * Step 1b, passkey route (issue #1293 / ADR 0034): POST /webauthn/login/begin
     * for the challenge [signIn] started, returning the raw request options for
     * Credential Manager. The pending cookie never leaves this class.
     */
    suspend fun beginPasskey(): Result<String> {
        val cookie = pendingTwoFactorCookie ?: return Result.failure(AttachException("No pending 2FA challenge"))
        val api = remoteApi ?: return Result.failure(AttachException("Not signed in"))
        return api.webauthnLoginBegin(cookie)
    }

    /** Step 1b, passkey route: exchange the authenticator's assertion for the remote session (same tail as [completeTwoFactor]). */
    suspend fun completePasskey(assertionJson: String): Result<AttachSignInResult> {
        val cookie = pendingTwoFactorCookie ?: return Result.failure(AttachException("No pending 2FA challenge"))
        val api = remoteApi ?: return Result.failure(AttachException("Not signed in"))
        return finishTwoFactor(api.webauthnLoginFinish(assertionJson, cookie))
    }

    private suspend fun finishTwoFactor(result: Result<LoginResult>): Result<AttachSignInResult> {
        val profileId = remoteProfileId ?: return Result.failure(AttachException("Not signed in"))
        val login = result.getOrElse { return Result.failure(it) }
        val token = login.token
        if (token.isNullOrBlank()) return Result.failure(AttachException("Server returned no session"))
        pendingTwoFactorCookie = null
        storeToken(profileId, token)
        return Result.success(AttachSignInResult.SignedIn)
    }

    /**
     * Steps 2–4: export the bundle from the Local server (in memory), upload it
     * to the remote `mycorrhizal` import source, build the preview and return it.
     * Resumes from the first incomplete step on a retry.
     */
    suspend fun prepare(onProgress: (AttachProgress) -> Unit = {}): Result<AttachPreview> = attempt {
        val api = remoteApi ?: throw AttachException("Not signed in")

        val bytes = bundle ?: run {
            onProgress(AttachProgress(AttachStage.Exporting))
            localApi.exportAccountBundle().getOrThrow().also { bundle = it }
        }

        val session = sessionId ?: run {
            onProgress(AttachProgress(AttachStage.Uploading))
            val key = uploadKey ?: newKey().also { uploadKey = it }
            val uploaded = api.uploadMycorrhizalBundle(bytes, "account-bundle.json", key).getOrThrow()
            totals = uploaded.totals
            sessionId = uploaded.sessionId
            uploaded.sessionId
        }

        if (!fetchStarted) {
            api.startMycorrhizalFetch(session).getOrThrow()
            fetchStarted = true
        }
        pollUntil(api, session, AttachStage.Preparing, onProgress) { it.isReady }

        val preview = api.getMycorrhizalImportPreview(session).getOrThrow()
        AttachPreview(totals ?: MycorrhizalBundleCounts(), preview)
    }

    /** Step 5a: apply the reviewed per-contact decisions on the remote and wait for it to finish. */
    suspend fun confirm(
        actions: List<RowImportAction>,
        onProgress: (AttachProgress) -> Unit = {},
    ): Result<SourceImportResult> = attempt {
        val api = remoteApi ?: throw AttachException("Not signed in")
        val session = sessionId ?: throw AttachException("Nothing to confirm")
        onProgress(AttachProgress(AttachStage.Importing))
        api.confirmMycorrhizalImport(ImportConfirmRequest(sessionId = session, actions = actions)).getOrThrow()
        val done = pollUntil(api, session, AttachStage.Importing, onProgress) { it.isDone }
        // A finished session stays live server-side until dropped, and a user may
        // only hold a few (MaxMycorrhizalImportSessionsPerUser) — so a wizard
        // re-run would hit a 429 without this.
        dropRemoteSession()
        done.result ?: SourceImportResult()
    }

    /**
     * Step 5b/6: switch to the Remote profile, then mark the Local one a
     * read-only archive. Switch first: an archived-and-active profile would
     * refuse the outbox drain that [SessionManager.switchProfile] attempts. If
     * the process dies between the two, Local is still writable (the safe side).
     */
    suspend fun finish(discardPending: Boolean = false): AttachFinishResult {
        val remoteId = remoteProfileId ?: throw AttachException("Not signed in")
        val localId = localProfileId ?: throw AttachException("No local profile to archive")
        return when (val switched = sessionManager.switchProfile(remoteId, discardPending)) {
            is SwitchProfileResult.NeedsConfirmation -> AttachFinishResult.NeedsConfirmation(switched.pendingCount)
            SwitchProfileResult.Switched -> {
                sessionManager.setProfileArchived(localId, true)
                close()
                AttachFinishResult.Done
            }
        }
    }

    /** Drop the remote session (best effort) and every in-memory secret/bundle. Safe to call twice. */
    suspend fun cancel() {
        dropRemoteSession()
        close()
    }

    /** Forget the bundle, key, session and credentials. The stored profile token is kept. */
    fun close() {
        bundle = null
        resetRemoteSession()
        uploadKey = null
        remoteApi = null
        remoteToken = null
        pendingTwoFactorCookie = null
    }

    /** The Local profile being migrated — the active one when the wizard opened. */
    var localProfileId: String? = null
        private set

    /** Bind the run to the active Local profile; fails for any other kind or an archived one. */
    suspend fun begin(): Result<ServerProfile> {
        val active = sessionManager.activeProfile()
        return when {
            active == null || active.kind !is ServerProfileKind.Local ->
                Result.failure(AttachException("The attach wizard starts from the Local profile"))
            active.archived -> Result.failure(AttachException("This profile is already archived"))
            else -> {
                localProfileId = active.id
                Result.success(active)
            }
        }
    }

    private suspend fun resolveRemoteProfile(existingId: String?, label: String, url: String): ServerProfile? {
        if (existingId != null) {
            val existing = sessionManager.profiles().find { it.id == existingId }
            return existing?.takeIf { it.kind is ServerProfileKind.Remote }?.also { remoteProfileId = it.id }
        }
        val trimmed = url.trim().trimEnd('/')
        // Reuse a profile this run (or the user) already has for the same URL
        // instead of stacking one per failed attempt.
        val reused = sessionManager.profiles().find { it.remoteUrl == trimmed }
        val profile = reused ?: sessionManager.addRemoteProfile(label.trim(), trimmed)
        remoteProfileId = profile.id
        return profile
    }

    private suspend fun storeToken(profileId: String, token: String) {
        remoteToken = token
        remoteProfileId = profileId
        sessionManager.saveProfileToken(profileId, token)
    }

    /** Best-effort server-side drop of the current remote session, then forget it locally. */
    private suspend fun dropRemoteSession() {
        val api = remoteApi
        val session = sessionId
        if (api != null && session != null) api.cancelMycorrhizalImport(session)
        resetRemoteSession()
    }

    private fun resetRemoteSession() {
        sessionId = null
        totals = null
        fetchStarted = false
        uploadKey = null
    }

    private class AttachSessionDied(message: String) : Exception(message)

    private suspend fun pollUntil(
        api: ApiClient,
        session: String,
        stage: AttachStage,
        onProgress: (AttachProgress) -> Unit,
        predicate: (SourceImportStatus) -> Boolean,
    ): SourceImportStatus {
        repeat(maxPolls) {
            val status = api.getMycorrhizalImportStatus(session).getOrThrow()
            if (status.isFailed) {
                throw AttachSessionDied(status.error?.takeIf { it.isNotBlank() } ?: "Import ${status.phase}")
            }
            if (predicate(status)) return status
            onProgress(AttachProgress(stage, status.phaseDone, status.phaseTotal))
            delay(pollIntervalMs)
        }
        throw AttachException("Timed out waiting for the server")
    }

    /**
     * Runs one wizard step as a [Result]. A remote session the server reports
     * dead cannot be resumed, so it is forgotten (the retry re-uploads under a
     * fresh key) and surfaced as an [AttachException]; a transport error keeps
     * the step memo so the retry resumes. Cancellation is never swallowed.
     */
    @Suppress("TooGenericExceptionCaught")
    private suspend fun <T> attempt(block: suspend () -> T): Result<T> = try {
        Result.success(block())
    } catch (e: CancellationException) {
        throw e
    } catch (e: AttachSessionDied) {
        // Drop the dead session on the server too: it still counts against the cap.
        dropRemoteSession()
        Result.failure(AttachException(e.message.orEmpty(), e))
    } catch (e: Exception) {
        Result.failure(e)
    }

    private companion object {
        const val POLL_INTERVAL_MS = 750L
        const val MAX_POLLS = 1_600 // ~20 min at 750 ms — a large account's map + import
    }
}

/**
 * Production [AttachRemoteApiFactory]: a fresh client per wizard run around fixed
 * URL/token providers, so it is independent of the active (Local) profile. It has
 * no session-expiry interceptor — a 401 from the remote must not log the user out
 * of the profile they are still using — and no local-socket transport (the remote
 * is only ever reached over the network).
 */
class DefaultAttachRemoteApiFactory(
    private val clientVersionProvider: ClientVersionProvider,
    private val debug: Boolean,
) : AttachRemoteApiFactory {
    override fun create(baseUrl: String, tokenProvider: TokenProvider): ApiClient =
        ApiClient(
            NetworkFactory.okHttpClient(
                tokenProvider = tokenProvider,
                baseUrlProvider = BaseUrlProvider { baseUrl },
                debug = debug,
                clientVersionProvider = clientVersionProvider,
            ),
            NetworkFactory.moshi(),
        )
}
