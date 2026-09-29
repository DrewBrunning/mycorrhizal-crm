package com.mycorrhizal.crm.data.attach

import com.mycorrhizal.crm.data.session.DefaultSessionManager
import com.mycorrhizal.crm.data.session.FakeSessionPrefsStorage
import com.mycorrhizal.crm.data.session.FakeTokenStorage
import com.mycorrhizal.crm.domain.profile.ServerProfileKind
import com.mycorrhizal.crm.domain.repository.PendingInteraction
import com.mycorrhizal.crm.domain.repository.PendingInteractionRepository
import com.mycorrhizal.crm.model.network.RowImportAction
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.BaseUrlProvider
import com.mycorrhizal.crm.network.NetworkFactory
import com.mycorrhizal.crm.network.TokenProvider
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.atomic.AtomicInteger

/**
 * ADR 0028 Decision 3 / issue #1265: the attach wizard's orchestration, exercised
 * over real HTTP against two servers — one standing in for the on-device `Local`
 * server (only ever asked to export) and one for the chosen remote. What is pinned
 * here is what the ticket promises: an interruption at any step leaves `Local`
 * active and writable; the upload carries an `Idempotency-Key`; only the final
 * step switches and archives.
 */
class AttachToRemoteCoordinatorTest {

    private lateinit var local: MockWebServer
    private lateinit var remote: MockWebServer
    private lateinit var tokens: FakeTokenStorage
    private lateinit var manager: DefaultSessionManager
    private lateinit var remoteLog: CopyOnWriteArrayList<RecordedRequest>

    private val bundleJson = """{"format":"mycorrhizal-account","version":1,"plan":{"contacts":[{"id":"c1"}]}}"""

    // Scripted remote behaviour, mutated per test.
    private var loginBody = """{"language":"en"}"""
    private var loginStatus = 200
    private var twoFactorRequired = false
    private var uploadStatuses = ArrayDeque<Int>()
    private var fetchStatuses = ArrayDeque<Int>()
    private var preparePhases = ArrayDeque<String>()
    private var importPhases = ArrayDeque<String>()
    private var confirmed = false
    private val uploadCount = AtomicInteger()
    private var exportStatus = 200

    private var pending: PendingInteractionRepository? = null

    @Before
    fun setUp() = runTest {
        local = MockWebServer().apply {
            dispatcher = object : Dispatcher() {
                override fun dispatch(request: RecordedRequest): MockResponse =
                    if (request.path == "/api/v1/export/account" && request.method == "GET") {
                        MockResponse().setResponseCode(exportStatus).setBody(if (exportStatus == 200) bundleJson else "{}")
                    } else {
                        // The Local server must ONLY ever be asked to export.
                        MockResponse().setResponseCode(599)
                    }
            }
            start()
        }
        remoteLog = CopyOnWriteArrayList()
        remote = MockWebServer().apply {
            dispatcher = object : Dispatcher() {
                override fun dispatch(request: RecordedRequest): MockResponse {
                    remoteLog.add(request)
                    val path = request.requestUrl?.encodedPath.orEmpty()
                    return when {
                        path == "/api/v1/login" && twoFactorRequired ->
                            MockResponse().setResponseCode(200)
                                .setHeader("Set-Cookie", "2fa_pending=PENDING; Path=/")
                                .setBody("""{"two_factor_required":true}""")
                        path == "/api/v1/login" ->
                            MockResponse().setResponseCode(loginStatus)
                                .setHeader("Set-Cookie", "auth_token=REMOTE-JWT; Path=/; HttpOnly")
                                .setBody(loginBody)
                        path == "/api/v1/login/2fa" ->
                            MockResponse().setResponseCode(200)
                                .setHeader("Set-Cookie", "auth_token=REMOTE-JWT-2FA; Path=/; HttpOnly")
                                .setBody("""{"language":"en"}""")
                        path == "/api/v1/import/mycorrhizal/upload" -> {
                            uploadCount.incrementAndGet()
                            val status = uploadStatuses.removeFirstOrNull() ?: 200
                            MockResponse().setResponseCode(status).setBody(
                                if (status == 200) """{"session_id":"s-${uploadCount.get()}","version":1,"totals":{"contacts":1,"notes":4,"activities":2}}""" else "{}",
                            )
                        }
                        path == "/api/v1/import/mycorrhizal/fetch" ->
                            MockResponse().setResponseCode(fetchStatuses.removeFirstOrNull() ?: 202).setBody("{}")
                        path == "/api/v1/import/mycorrhizal/status" -> {
                            val queue = if (confirmed) importPhases else preparePhases
                            val phase = queue.firstOrNull() ?: if (confirmed) "done" else "ready"
                            if (queue.size > 1) queue.removeFirst()
                            MockResponse().setResponseCode(200).setBody(
                                """{"session_id":"s","phase":"$phase","phase_done":1,"phase_total":3,
                                   "error":"${if (phase == "failed") "boom" else ""}",
                                   "result":{"total_processed":1,"created":1,"updated":0,"skipped":0}}""",
                            )
                        }
                        path == "/api/v1/import/mycorrhizal/preview" ->
                            MockResponse().setResponseCode(200).setBody(
                                """{"session_id":"s","rows":[{"row_index":0,"suggested_action":"add"}],
                                   "total_rows":1,"valid_rows":1,"duplicate_count":0,"error_count":0,"loss_report":[]}""",
                            )
                        path == "/api/v1/import/mycorrhizal/confirm" -> {
                            confirmed = true
                            MockResponse().setResponseCode(202).setBody("{}")
                        }
                        path == "/api/v1/import/mycorrhizal/cancel" -> MockResponse().setResponseCode(200).setBody("{}")
                        else -> MockResponse().setResponseCode(404)
                    }
                }
            }
            start()
        }
        tokens = FakeTokenStorage()
        pending = null
        manager = newManager()
        manager.init()
        manager.activateLocalProfile("LOCAL-JWT")
    }

    @After
    fun tearDown() {
        local.shutdown()
        remote.shutdown()
    }

    private fun newManager(): DefaultSessionManager {
        var n = 0
        return DefaultSessionManager(
            tokenStorage = tokens,
            prefsStorage = FakeSessionPrefsStorage(),
            pendingInteractions = pending,
            newProfileId = { "profile-${++n}" },
        )
    }

    private fun apiFor(server: MockWebServer, token: TokenProvider = TokenProvider { null }): ApiClient =
        ApiClient(
            NetworkFactory.okHttpClient(token, BaseUrlProvider { server.url("/").toString().trimEnd('/') }),
            NetworkFactory.moshi(),
        )

    private fun coordinator(): AttachToRemoteCoordinator {
        var keys = 0
        return AttachToRemoteCoordinator(
            sessionManager = manager,
            localApi = apiFor(local),
            remoteApiFactory = AttachRemoteApiFactory { _, tp -> apiFor(remote, tp) },
        ).apply {
            pollIntervalMs = 0
            maxPolls = 5
            newKey = { "key-${++keys}" }
        }
    }

    private val remoteUrl get() = remote.url("/").toString().trimEnd('/')

    private suspend fun AttachToRemoteCoordinator.signInNew() =
        signIn(null, "Home", remoteUrl, "alice", "pw")

    private fun remoteRequests(path: String) = remoteLog.filter { it.requestUrl?.encodedPath == path }

    private suspend fun assertLocalStillActiveAndWritable() {
        assertEquals(ServerProfileKind.Local, manager.activeProfile()?.kind)
        assertFalse("Local must not be archived before the final step", manager.isActiveProfileArchived())
        assertEquals("LOCAL-JWT", manager.bearerToken())
    }

    // --- happy path -------------------------------------------------------------

    @Test
    fun `end to end - export upload review confirm then switch and archive Local`() = runTest {
        val c = coordinator()
        val localId = c.begin().getOrThrow().id
        val stages = mutableListOf<AttachStage>()

        assertEquals(AttachSignInResult.SignedIn, c.signInNew().getOrThrow())
        assertLocalStillActiveAndWritable() // signing in does NOT switch

        val prepared = c.prepare { stages += it.stage }.getOrThrow()
        assertLocalStillActiveAndWritable() // reviewing does NOT switch
        assertEquals(1, prepared.totals.contacts)
        assertEquals(4, prepared.totals.notes)
        assertEquals(1, prepared.preview.rows.size)
        assertEquals(listOf(AttachStage.Exporting, AttachStage.Uploading), stages.distinct().take(2))

        val result = c.confirm(listOf(RowImportAction(0, "add"))).getOrThrow()
        assertEquals(1, result.created)
        assertLocalStillActiveAndWritable() // even a finished import has not switched yet

        assertEquals(AttachFinishResult.Done, c.finish())

        val remoteProfile = manager.activeProfile()
        assertEquals(ServerProfileKind.Remote(remoteUrl), remoteProfile?.kind)
        assertEquals("REMOTE-JWT", tokens.tokens[remoteProfile?.id])
        assertEquals("REMOTE-JWT", manager.bearerToken())
        val archivedLocal = manager.profiles().single { it.id == localId }
        assertTrue("the Local profile is now a read-only archive", archivedLocal.archived)
        assertEquals("the Local token is kept so the archive stays browsable", "LOCAL-JWT", tokens.tokens[localId])
    }

    @Test
    fun `the exact exported bytes are uploaded with an Idempotency-Key and the remote bearer`() = runTest {
        val c = coordinator()
        c.begin().getOrThrow()
        c.signInNew().getOrThrow()
        c.prepare().getOrThrow()

        val upload = remoteRequests("/api/v1/import/mycorrhizal/upload").single()
        assertEquals("key-1", upload.getHeader("Idempotency-Key"))
        assertEquals("Bearer REMOTE-JWT", upload.getHeader("Authorization"))
        assertTrue(upload.body.readUtf8().contains(bundleJson))
        // The Local server saw only the export, and never the remote credential.
        val localRequests = generateSequence { local.takeRequest(0, java.util.concurrent.TimeUnit.MILLISECONDS) }.toList()
        assertEquals(listOf("GET /api/v1/export/account"), localRequests.map { "${it.method} ${it.path}" })
    }

    @Test
    fun `nothing is sent to the Local server except the export - no remote to local flow`() = runTest {
        val c = coordinator()
        c.begin().getOrThrow()
        c.signInNew().getOrThrow()
        c.prepare().getOrThrow()
        c.confirm(listOf(RowImportAction(0, "add"))).getOrThrow()
        c.finish()

        assertEquals(1, local.requestCount)
    }

    @Test
    fun `confirm forwards the reviewed per-row actions and session`() = runTest {
        val c = coordinator()
        c.begin().getOrThrow()
        c.signInNew().getOrThrow()
        c.prepare().getOrThrow()

        c.confirm(listOf(RowImportAction(0, "update"), RowImportAction(1, "skip"))).getOrThrow()

        val body = remoteRequests("/api/v1/import/mycorrhizal/confirm").single().body.readUtf8()
        assertTrue(body, body.contains(""""session_id":"s-1""""))
        assertTrue(body, body.contains(""""row_index":0"""))
        assertTrue(body, body.contains(""""action":"update""""))
        assertTrue(body, body.contains(""""action":"skip""""))
    }

    // --- begin ------------------------------------------------------------------

    @Test
    fun `begin refuses when the active profile is a Remote one`() = runTest {
        val remoteProfile = manager.addRemoteProfile("Home", remoteUrl)
        manager.saveProfileToken(remoteProfile.id, "t")
        manager.switchProfile(remoteProfile.id)

        val result = coordinator().begin()

        assertTrue(result.exceptionOrNull() is AttachException)
    }

    @Test
    fun `begin refuses an already archived Local profile`() = runTest {
        manager.setProfileArchived(manager.activeProfile()!!.id, true)

        assertTrue(coordinator().begin().exceptionOrNull() is AttachException)
    }

    @Test
    fun `begin refuses when there is no profile at all`() = runTest {
        tokens = FakeTokenStorage()
        manager = newManager().also { it.init() }

        assertTrue(coordinator().begin().isFailure)
    }

    // --- sign in ----------------------------------------------------------------

    @Test
    fun `a wrong password fails sign-in and leaves Local active with nothing stored`() = runTest {
        loginStatus = 401
        loginBody = """{"error":{"code":"UNAUTHORIZED","message":"bad credentials"}}"""
        val c = coordinator()
        c.begin().getOrThrow()

        val result = c.signInNew()

        assertTrue(result.exceptionOrNull() is ApiError.Client)
        assertLocalStillActiveAndWritable()
        assertTrue(tokens.tokens.keys.none { it != manager.activeProfileId() })
    }

    @Test
    fun `retrying a failed sign-in reuses the profile instead of stacking one per attempt`() = runTest {
        loginStatus = 401
        val c = coordinator()
        c.begin().getOrThrow()
        c.signInNew()
        c.signInNew()
        loginStatus = 200
        c.signInNew().getOrThrow()

        assertEquals(1, manager.profiles().count { it.kind is ServerProfileKind.Remote })
    }

    @Test
    fun `an existing Remote profile can be chosen and is not duplicated`() = runTest {
        val existing = manager.addRemoteProfile("Home", remoteUrl)
        val c = coordinator()
        c.begin().getOrThrow()

        c.signIn(existing.id, "", "", "alice", "pw").getOrThrow()

        assertEquals(1, manager.profiles().count { it.kind is ServerProfileKind.Remote })
        assertEquals("REMOTE-JWT", tokens.tokens[existing.id])
    }

    @Test
    fun `choosing a Local or unknown profile as the target fails`() = runTest {
        val c = coordinator()
        val localId = c.begin().getOrThrow().id

        assertTrue(c.signIn(localId, "", "", "a", "b").isFailure)
        assertTrue(c.signIn("missing", "", "", "a", "b").isFailure)
    }

    @Test
    fun `a login that returns no session token is a failure`() = runTest {
        // Same server, but without a Set-Cookie: the remote said 200 yet issued nothing.
        remote.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest) = MockResponse().setResponseCode(200).setBody("{}")
        }
        val c = coordinator()
        c.begin().getOrThrow()

        assertTrue(c.signInNew().exceptionOrNull() is AttachException)
    }

    @Test
    fun `two-factor accounts complete the challenge before anything is exported`() = runTest {
        twoFactorRequired = true
        val c = coordinator()
        c.begin().getOrThrow()

        assertEquals(AttachSignInResult.TwoFactorRequired, c.signInNew().getOrThrow())
        assertEquals("no bundle may be exported before the login completes", 0, local.requestCount)

        assertEquals(AttachSignInResult.SignedIn, c.completeTwoFactor("123456").getOrThrow())
        val profile = manager.profiles().single { it.kind is ServerProfileKind.Remote }
        assertEquals("REMOTE-JWT-2FA", tokens.tokens[profile.id])
        val twoFa = remoteRequests("/api/v1/login/2fa").single()
        assertEquals("2fa_pending=PENDING", twoFa.getHeader("Cookie"))
        assertLocalStillActiveAndWritable()
    }

    @Test
    fun `completeTwoFactor without a pending challenge or sign-in fails`() = runTest {
        val c = coordinator()
        c.begin().getOrThrow()

        assertTrue(c.completeTwoFactor("1").exceptionOrNull() is AttachException)
    }

    // --- prepare ----------------------------------------------------------------

    @Test
    fun `prepare before sign-in fails`() = runTest {
        val c = coordinator()
        c.begin().getOrThrow()

        assertTrue(c.prepare().exceptionOrNull() is AttachException)
    }

    @Test
    fun `a failed export leaves Local active and nothing uploaded`() = runTest {
        exportStatus = 500
        val c = coordinator()
        c.begin().getOrThrow()
        c.signInNew().getOrThrow()

        assertTrue(c.prepare().isFailure)

        assertEquals(0, uploadCount.get())
        assertLocalStillActiveAndWritable()
    }

    @Test
    fun `a failed upload retries with the SAME Idempotency-Key and reuses the exported bundle`() = runTest {
        uploadStatuses += 500
        val c = coordinator()
        c.begin().getOrThrow()
        c.signInNew().getOrThrow()

        assertTrue(c.prepare().isFailure)
        assertLocalStillActiveAndWritable()
        c.prepare().getOrThrow()

        val uploads = remoteRequests("/api/v1/import/mycorrhizal/upload")
        assertEquals(2, uploads.size)
        assertEquals("key-1", uploads[0].getHeader("Idempotency-Key"))
        assertEquals("key-1", uploads[1].getHeader("Idempotency-Key"))
        assertEquals("the bundle is exported once and held in memory", 1, local.requestCount)
    }

    @Test
    fun `a failed fetch resumes without exporting or uploading again`() = runTest {
        fetchStatuses += 500
        val c = coordinator()
        c.begin().getOrThrow()
        c.signInNew().getOrThrow()

        assertTrue(c.prepare().isFailure)
        c.prepare().getOrThrow()

        assertEquals(1, local.requestCount)
        assertEquals(1, uploadCount.get())
        assertEquals(2, remoteRequests("/api/v1/import/mycorrhizal/fetch").size)
    }

    @Test
    fun `progress is reported while the server prepares the preview`() = runTest {
        preparePhases += listOf("fetching_contacts", "building_preview", "ready")
        val c = coordinator()
        c.begin().getOrThrow()
        c.signInNew().getOrThrow()
        val seen = mutableListOf<AttachProgress>()

        c.prepare { seen += it }.getOrThrow()

        assertTrue(seen.any { it.stage == AttachStage.Preparing && it.total == 3 })
    }

    @Test
    fun `a server-side failed phase is surfaced and the next attempt re-uploads under a fresh key`() = runTest {
        preparePhases += "failed"
        val c = coordinator()
        c.begin().getOrThrow()
        c.signInNew().getOrThrow()

        val failure = c.prepare().exceptionOrNull()
        assertTrue(failure is AttachException)
        assertEquals("boom", failure?.message)
        assertLocalStillActiveAndWritable()

        preparePhases.clear()
        c.prepare().getOrThrow()

        val keys = remoteRequests("/api/v1/import/mycorrhizal/upload").map { it.getHeader("Idempotency-Key") }
        assertEquals(listOf("key-1", "key-2"), keys)
        assertNotEquals(keys[0], keys[1])
    }

    @Test
    fun `the dead session is dropped on the server so it does not count against the per-user cap`() = runTest {
        preparePhases += "failed"
        val c = coordinator()
        c.begin().getOrThrow()
        c.signInNew().getOrThrow()

        assertTrue(c.prepare().isFailure)

        val cancels = remoteRequests("/api/v1/import/mycorrhizal/cancel")
        assertEquals(1, cancels.size)
        assertEquals("session_id=s-1", cancels.single().requestUrl?.query)
    }

    @Test
    fun `a cancelled phase is a failure too`() = runTest {
        preparePhases += "cancelled"
        val c = coordinator()
        c.begin().getOrThrow()
        c.signInNew().getOrThrow()

        assertTrue(c.prepare().exceptionOrNull() is AttachException)
    }

    @Test
    fun `a server that never becomes ready times out instead of hanging`() = runTest {
        preparePhases += "building_preview"
        val c = coordinator().apply { maxPolls = 3 }
        c.begin().getOrThrow()
        c.signInNew().getOrThrow()

        val failure = c.prepare().exceptionOrNull()

        assertTrue(failure is AttachException)
        assertTrue(failure?.message.orEmpty().contains("Timed out"))
        assertLocalStillActiveAndWritable()
    }

    // --- confirm ----------------------------------------------------------------

    @Test
    fun `confirm without a prepared session fails`() = runTest {
        val c = coordinator()
        c.begin().getOrThrow()

        assertTrue(c.confirm(emptyList()).exceptionOrNull() is AttachException)

        c.signInNew().getOrThrow()
        assertTrue(c.confirm(emptyList()).exceptionOrNull() is AttachException)
    }

    @Test
    fun `an import that fails on the remote leaves Local active and unarchived`() = runTest {
        importPhases += "failed"
        val c = coordinator()
        c.begin().getOrThrow()
        c.signInNew().getOrThrow()
        c.prepare().getOrThrow()

        val failure = c.confirm(listOf(RowImportAction(0, "add"))).exceptionOrNull()

        assertTrue(failure is AttachException)
        assertLocalStillActiveAndWritable()
    }

    @Test
    fun `a finished import drops its remote session so a re-run is not refused by the session cap`() = runTest {
        val c = coordinator()
        c.begin().getOrThrow()
        c.signInNew().getOrThrow()
        c.prepare().getOrThrow()
        assertTrue(remoteRequests("/api/v1/import/mycorrhizal/cancel").isEmpty())

        c.confirm(listOf(RowImportAction(0, "add"))).getOrThrow()

        val cancels = remoteRequests("/api/v1/import/mycorrhizal/cancel")
        assertEquals(1, cancels.size)
        assertEquals("session_id=s-1", cancels.single().requestUrl?.query)
        // ...and the wizard's own later cancel() has nothing left to drop.
        c.cancel()
        assertEquals(1, remoteRequests("/api/v1/import/mycorrhizal/cancel").size)
    }

    @Test
    fun `a failed import does not drop the session - the user may retry the confirm`() = runTest {
        importPhases += "importing"
        importPhases += "importing"
        val c = coordinator().apply { maxPolls = 2 }
        c.begin().getOrThrow()
        c.signInNew().getOrThrow()
        c.prepare().getOrThrow()

        assertTrue(c.confirm(listOf(RowImportAction(0, "add"))).isFailure) // timed out, not "failed"

        assertTrue(remoteRequests("/api/v1/import/mycorrhizal/cancel").isEmpty())
    }

    @Test
    fun `importing progress is reported until the remote is done`() = runTest {
        importPhases += listOf("importing", "importing_photos", "done")
        val c = coordinator()
        c.begin().getOrThrow()
        c.signInNew().getOrThrow()
        c.prepare().getOrThrow()
        val seen = mutableListOf<AttachStage>()

        c.confirm(listOf(RowImportAction(0, "add"))) { seen += it.stage }.getOrThrow()

        assertTrue(seen.contains(AttachStage.Importing))
    }

    // --- finish -----------------------------------------------------------------

    @Test
    fun `finish asks for confirmation when unsent Local interactions would be dropped and changes nothing`() = runTest {
        val repo = mockk<PendingInteractionRepository>()
        coEvery { repo.unsynced() } returns listOf(mockk<PendingInteraction>(), mockk())
        pending = repo
        manager = newManager().also {
            it.init()
            it.activateLocalProfile("LOCAL-JWT")
        }
        val c = coordinator()
        c.begin().getOrThrow()
        c.signInNew().getOrThrow()
        c.prepare().getOrThrow()
        c.confirm(listOf(RowImportAction(0, "add"))).getOrThrow()

        val outcome = c.finish()

        assertEquals(AttachFinishResult.NeedsConfirmation(2), outcome)
        assertLocalStillActiveAndWritable()

        assertEquals(AttachFinishResult.Done, c.finish(discardPending = true))
        assertTrue(manager.profiles().single { it.kind is ServerProfileKind.Local }.archived)
        assertEquals(ServerProfileKind.Remote(remoteUrl), manager.activeProfile()?.kind)
    }

    @Test
    fun `finish before sign-in or begin fails`() = runTest {
        val c = coordinator()
        val noRemote = runCatching { c.finish() }.exceptionOrNull()
        assertTrue(noRemote is AttachException)

        c.begin().getOrThrow()
        c.signInNew().getOrThrow()
        // Local id is known after begin(); simulate a coordinator that never began.
        val fresh = coordinator()
        val notBegun = runCatching { fresh.finish() }.exceptionOrNull()
        assertTrue(notBegun is AttachException)
    }

    // --- cancel / close ---------------------------------------------------------

    @Test
    fun `cancel drops the remote session and forgets the bundle and credentials`() = runTest {
        val c = coordinator()
        c.begin().getOrThrow()
        c.signInNew().getOrThrow()
        c.prepare().getOrThrow()

        c.cancel()

        assertEquals(1, remoteRequests("/api/v1/import/mycorrhizal/cancel").size)
        assertTrue(c.prepare().exceptionOrNull() is AttachException)
        assertLocalStillActiveAndWritable()
    }

    @Test
    fun `cancel with no session makes no cancel request`() = runTest {
        val c = coordinator()
        c.begin().getOrThrow()

        c.cancel()

        assertTrue(remoteRequests("/api/v1/import/mycorrhizal/cancel").isEmpty())
        assertNull(c.prepare().exceptionOrNull()?.cause)
    }

    @Test
    fun `an interrupted run can simply be re-run from the start`() = runTest {
        val first = coordinator()
        first.begin().getOrThrow()
        first.signInNew().getOrThrow()
        first.prepare().getOrThrow()
        first.close() // the app was killed / the user backed out

        assertLocalStillActiveAndWritable()

        val second = coordinator()
        second.begin().getOrThrow()
        second.signInNew().getOrThrow()
        second.prepare().getOrThrow()
        second.confirm(listOf(RowImportAction(0, "add"))).getOrThrow()
        assertEquals(AttachFinishResult.Done, second.finish())
        assertEquals(1, manager.profiles().count { it.kind is ServerProfileKind.Remote })
        assertNotNull(manager.profiles().single { it.kind is ServerProfileKind.Local }.takeIf { it.archived })
    }
}
