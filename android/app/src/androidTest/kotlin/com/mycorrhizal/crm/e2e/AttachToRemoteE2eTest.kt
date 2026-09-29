package com.mycorrhizal.crm.e2e

import androidx.test.ext.junit.runners.AndroidJUnit4
import com.mycorrhizal.crm.data.attach.AttachFinishResult
import com.mycorrhizal.crm.data.attach.AttachToRemoteCoordinator
import com.mycorrhizal.crm.data.attach.DefaultAttachRemoteApiFactory
import com.mycorrhizal.crm.data.session.DefaultSessionManager
import com.mycorrhizal.crm.data.session.ProfilesSnapshot
import com.mycorrhizal.crm.data.session.SessionPrefsStorage
import com.mycorrhizal.crm.data.session.TokenStorage
import com.mycorrhizal.crm.domain.profile.ServerProfileKind
import com.mycorrhizal.crm.model.network.RowImportAction
import com.mycorrhizal.crm.network.ArchivedProfileWriteException
import com.mycorrhizal.crm.network.BaseUrlProvider
import com.mycorrhizal.crm.network.ClientVersionProvider
import com.mycorrhizal.crm.network.NetworkFactory
import com.mycorrhizal.crm.network.TokenProvider
import kotlinx.coroutines.runBlocking
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import java.util.UUID

/**
 * ADR 0028 Decision 3 / issue #1265, acceptance test: create data on the "local"
 * side, attach to a remote, and the remote account shows all of it; the local
 * profile is then a read-only archive; re-running the wizard creates no
 * duplicates.
 *
 * The real embedded server needs an arm64 device (ADR 0028, "Local mode is
 * testable only on arm64 hardware"), so the emulator cannot host a genuine
 * `Local` profile. This test therefore stands in the docker-compose.test.yml
 * backend for BOTH ends — the seed account is the "local" server the bundle is
 * exported from, a second account is the remote it is imported into — and drives
 * the real [AttachToRemoteCoordinator] and the real wire (export → multipart
 * upload with an Idempotency-Key → map/preview → confirm → ledger) end to end.
 * The session manager holds a genuine `Local`-kind profile, so the switch and
 * archive steps run for real too. Only the Unix-socket transport is not
 * exercised here; that is the arm64-device runbook's job.
 *
 * It needs no UI (the wizard's screens are covered by Robolectric tests), so it
 * does not extend [E2eBaseTest].
 */
@RunWith(AndroidJUnit4::class)
class AttachToRemoteE2eTest {

    private val localSide = E2eBackend()
    private val remoteSide = E2eBackend()
    private val marker = UUID.randomUUID().toString().replace("-", "").take(8)
    private val names = listOf("E2EAttachAda $marker", "E2EAttachBo $marker", "E2EAttachCy $marker")

    private lateinit var localToken: String

    @Before
    fun setUp() {
        localSide.registerSeedUser()
        remoteSide.registerSeedUser(E2eConfig.SECOND_USERNAME, E2eConfig.SECOND_EMAIL, E2eConfig.SECOND_PASSWORD)
        localToken = localSide.login()
        remoteSide.login(E2eConfig.SECOND_USERNAME, E2eConfig.SECOND_PASSWORD)
        localSide.cleanupTestContacts()
        remoteSide.cleanupTestContacts()
        names.forEach { name ->
            val (given, surname) = name.split(" ", limit = 2)
            localSide.createContact(given, surname)
        }
    }

    @After
    fun tearDown() {
        runCatching { localSide.cleanupTestContacts() }
        runCatching { remoteSide.cleanupTestContacts() }
    }

    private class MemoryTokens : TokenStorage {
        val tokens = mutableMapOf<String, String>()
        override suspend fun save(profileId: String, token: String) { tokens[profileId] = token }
        override suspend fun load(profileId: String) = tokens[profileId]
        override suspend fun clear(profileId: String) { tokens.remove(profileId) }
        override suspend fun loadLegacy(): String? = null
        override suspend fun clearLegacy() = Unit
    }

    private class MemoryPrefs : SessionPrefsStorage {
        var snapshot = ProfilesSnapshot()
        override suspend fun save(serverUrl: String?) = Unit
        override suspend fun loadServerUrl(): String? = null
        override suspend fun saveProfiles(snapshot: ProfilesSnapshot) { this.snapshot = snapshot }
        override suspend fun loadProfiles() = snapshot
        override suspend fun clear() { snapshot = ProfilesSnapshot() }
    }

    /** A session manager whose active profile is a genuine Local-kind one. */
    private fun newSessionManager(): DefaultSessionManager = runBlocking {
        DefaultSessionManager(tokenStorage = MemoryTokens(), prefsStorage = MemoryPrefs()).also {
            it.init()
            it.activateLocalProfile("local-session-token")
        }
    }

    private fun coordinator(manager: DefaultSessionManager): AttachToRemoteCoordinator {
        // The "local server" export goes straight at the seed account's session.
        val factory = DefaultAttachRemoteApiFactory(ClientVersionProvider { HARNESS_VERSION }, debug = false)
        return AttachToRemoteCoordinator(
            sessionManager = manager,
            localApi = factory.create(E2eConfig.serverUrl, TokenProvider { localToken }),
            remoteApiFactory = factory,
        )
    }

    /** Runs the whole wizard, accepting each row's suggested action; returns the finish result. */
    private fun runWizard(manager: DefaultSessionManager): AttachFinishResult = runBlocking {
        val c = coordinator(manager)
        c.begin().getOrThrow()
        c.signIn(null, "E2E remote", E2eConfig.serverUrl, E2eConfig.SECOND_USERNAME, E2eConfig.SECOND_PASSWORD)
            .getOrThrow()
        val prepared = c.prepare().getOrThrow()
        assertTrue("the bundle carried the created contacts", prepared.totals.contacts >= names.size)
        val actions = prepared.preview.rows.map { RowImportAction(it.rowIndex, it.suggestedAction) }
        c.confirm(actions).getOrThrow()
        c.finish()
    }

    private fun remoteNames(): List<String> {
        val body = remoteSide.contactsJson()
        return names.filter { body.contains(it.substringBefore(" ")) && body.contains(marker) }
    }

    @Test
    fun attachMovesAllDataArchivesLocalAndARerunCreatesNoDuplicates() {
        val manager = newSessionManager()
        val localProfileId = runBlocking { manager.activeProfile() }!!.id

        // The remote account starts without any of the local data.
        assertTrue(remoteNames().isEmpty())

        // 1st run: everything arrives, the app switches to the remote, Local is archived.
        assertEquals(AttachFinishResult.Done, runWizard(manager))
        assertEquals(names.size, remoteNames().size)
        runBlocking {
            assertTrue(manager.activeProfile()?.kind is ServerProfileKind.Remote)
            assertTrue("Local is a read-only archive", manager.profiles().single { it.id == localProfileId }.archived)
        }
        val countAfterFirst = remoteSide.countContactsWithMarker(marker)
        assertEquals(names.size, countAfterFirst)

        // 2nd run on a fresh Local profile (the wizard is re-runnable): the import
        // ledger keys on the bundle's stable IDs, so nothing is created twice.
        val second = newSessionManager()
        assertEquals(AttachFinishResult.Done, runWizard(second))
        assertEquals("re-running must not duplicate contacts", countAfterFirst, remoteSide.countContactsWithMarker(marker))
    }

    @Test
    fun anArchivedLocalProfileCanBeReadButEveryWriteIsBlocked() {
        val manager = newSessionManager()
        runWizard(manager)
        val localId = runBlocking { manager.profiles().first { it.kind is ServerProfileKind.Local }.id }
        // Browse the archive again: switch back to it, as the Servers list allows.
        runBlocking { manager.switchProfile(localId) }
        assertTrue(manager.isActiveProfileArchived())

        // Same interceptor wiring the app uses, pointed at the real backend (the
        // sentinel transport is the arm64 runbook's concern).
        val client = NetworkFactory.okHttpClient(
            tokenProvider = TokenProvider { localToken },
            baseUrlProvider = BaseUrlProvider { E2eConfig.serverUrl },
            archivedProfileProvider = manager,
            clientVersionProvider = ClientVersionProvider { HARNESS_VERSION },
        )

        client.newCall(Request.Builder().url("${E2eConfig.apiBaseUrl}/contacts?limit=1").get().build())
            .execute().use { assertEquals(200, it.code) }

        val before = localSide.contactsJson()
        try {
            client.newCall(
                Request.Builder().url("${E2eConfig.apiBaseUrl}/contacts")
                    .post("{}".toRequestBody()).build(),
            ).execute().close()
            fail("a write on the archived profile must be blocked")
        } catch (e: ArchivedProfileWriteException) {
            assertEquals("POST", e.method)
        }
        assertEquals("the blocked write must not have reached the server", before, localSide.contactsJson())
    }

    private companion object {
        const val HARNESS_VERSION = "0.9.0"
    }
}
