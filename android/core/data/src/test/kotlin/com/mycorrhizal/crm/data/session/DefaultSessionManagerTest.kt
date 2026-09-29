package com.mycorrhizal.crm.data.session

import com.mycorrhizal.crm.domain.profile.ServerProfile
import com.mycorrhizal.crm.domain.profile.ServerProfileKind
import com.mycorrhizal.crm.domain.repository.PendingInteraction
import com.mycorrhizal.crm.domain.repository.PendingInteractionRepository
import com.mycorrhizal.crm.domain.repository.SessionState
import com.mycorrhizal.crm.network.LOCAL_SERVER_SENTINEL_URL
import app.cash.turbine.test
import kotlinx.coroutines.async
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class DefaultSessionManagerTest {

    /** Profile IDs are sequential so assertions are stable (no random UUIDs). */
    private class Ids {
        var next = 1
        fun new(): String = "profile-${next++}"
    }

    private fun manager(
        tokenStorage: FakeTokenStorage = FakeTokenStorage(),
        prefsStorage: FakeSessionPrefsStorage = FakeSessionPrefsStorage(),
        cleaner: SessionDataCleaner = NoopSessionDataCleaner,
        teardown: SessionTeardown = NoopSessionTeardown,
        pending: PendingInteractionRepository? = null,
        drainer: OutboxDrainer = NoopOutboxDrainer,
        secrets: ProfileSecretStorage = NoopProfileSecretStorage,
    ): DefaultSessionManager {
        val ids = Ids()
        return DefaultSessionManager(
            tokenStorage = tokenStorage,
            prefsStorage = prefsStorage,
            localDataCleaner = cleaner,
            sessionTeardown = teardown,
            pendingInteractions = pending,
            outboxDrainer = drainer,
            profileSecretStorage = secrets,
            newProfileId = ids::new,
        )
    }

    private fun managerPair(): Pair<DefaultSessionManager, FakeTokenStorage> {
        val tokenStorage = FakeTokenStorage()
        return manager(tokenStorage = tokenStorage) to tokenStorage
    }

    @Test
    fun `bearerToken is empty before init`() {
        val (manager, _) = managerPair()
        assertNull(manager.bearerToken())
    }

    @Test
    fun `init hydrates the active profile token from storage`() = runTest {
        val tokenStorage = FakeTokenStorage()
        val prefs = FakeSessionPrefsStorage()
        prefs.snapshot = ProfilesSnapshot(
            profiles = listOf(ServerProfile("p1", ServerProfileKind.Remote("https://a.example"), "a")),
            activeProfileId = "p1",
        )
        tokenStorage.tokens["p1"] = "stored-jwt"
        val manager = manager(tokenStorage = tokenStorage, prefsStorage = prefs)
        manager.init()

        assertEquals("stored-jwt", manager.bearerToken())
        assertEquals("https://a.example", manager.baseUrl())
    }

    @Test
    fun `init migrates a legacy server_url plus jwt into one active Remote profile`() = runTest {
        val tokenStorage = FakeTokenStorage()
        val prefs = FakeSessionPrefsStorage()
        prefs.serverUrl = "https://legacy.example.com"
        tokenStorage.legacy = "legacy-jwt"
        val manager = manager(tokenStorage = tokenStorage, prefsStorage = prefs)

        manager.init()

        // ADR 0028 Decision 1: an existing install becomes one Remote profile,
        // stays the active one, and keeps its token — no re-login.
        val profiles = manager.profiles()
        assertEquals(1, profiles.size)
        assertEquals(ServerProfileKind.Remote("https://legacy.example.com"), profiles.single().kind)
        assertEquals("legacy.example.com", profiles.single().label)
        assertEquals(profiles.single().id, manager.activeProfileId())
        assertEquals("legacy-jwt", manager.bearerToken())
        // The credential moved to the per-profile slot; the legacy slot is gone.
        assertNull(tokenStorage.legacy)
        assertEquals("legacy-jwt", tokenStorage.tokens[profiles.single().id])
    }

    @Test
    fun `init does not migrate a fresh install`() = runTest {
        val manager = manager()
        manager.init()

        assertTrue(manager.profiles().isEmpty())
        assertNull(manager.activeProfileId())
        assertNull(manager.bearerToken())
    }

    @Test
    fun `init moves the migrated profile's non-JWT secrets too`() = runTest {
        val tokenStorage = FakeTokenStorage()
        val prefs = FakeSessionPrefsStorage()
        prefs.serverUrl = "https://legacy.example.com"
        tokenStorage.legacy = "legacy-jwt"
        val secrets = RecordingSecrets()
        val manager = manager(tokenStorage = tokenStorage, prefsStorage = prefs, secrets = secrets)

        manager.init()

        assertEquals(listOf(manager.activeProfileId()), secrets.migrated)
    }

    @Test
    fun `setSession persists token and flips isLoggedIn`() = runTest {
        val (manager, tokenStorage) = managerPair()
        manager.init()
        manager.setServerUrl("https://crm.example.com")
        manager.setSession(
            serverUrl = "https://crm.example.com",
            token = "jwt-1",
            state = SessionState(userId = 7, username = "alice"),
        )

        assertEquals("jwt-1", manager.bearerToken())
        assertEquals("jwt-1", tokenStorage.tokens.values.single())
        assertEquals("https://crm.example.com", manager.baseUrl())
        val state = manager.observeSession().first()
        assertTrue(state.isLoggedIn)
        assertEquals(7, state.userId)
    }

    @Test
    fun `setSession anchors a profile when the login screen skipped setServerUrl`() = runTest {
        val (manager, tokenStorage) = managerPair()
        manager.init()

        manager.setSession("https://direct.example.com", "jwt-x", SessionState(userId = 1))

        assertEquals(1, manager.profiles().size)
        assertEquals("https://direct.example.com", manager.baseUrl())
        assertEquals("jwt-x", tokenStorage.tokens.values.single())
    }

    @Test
    fun `clearSession removes the token and profile but keeps the server url`() = runTest {
        val (manager, tokenStorage) = managerPair()
        manager.setSession(
            serverUrl = "https://crm.example.com",
            token = "jwt-1",
            state = SessionState(userId = 7, username = "alice"),
        )
        manager.clearSession()

        assertNull(manager.bearerToken())
        assertTrue(tokenStorage.tokens.isEmpty())
        // Issue #723 / ADR 0028: the profile is non-credential device config —
        // logout keeps it so the login screen can pre-fill it.
        assertEquals("https://crm.example.com", manager.serverUrl())
        assertEquals("https://crm.example.com", manager.baseUrl())
        val state = manager.observeSession().first()
        assertFalse(state.isLoggedIn)
        assertEquals("https://crm.example.com", state.serverUrl)
    }

    @Test
    fun `clearSession keepServerUrl=false removes the active profile and its secrets`() = runTest {
        val (manager, tokenStorage) = managerPair()
        val secrets = RecordingSecrets()
        val managerWithSecrets = manager(tokenStorage = tokenStorage, secrets = secrets)
        managerWithSecrets.setSession(
            serverUrl = "https://crm.example.com",
            token = "jwt-1",
            state = SessionState(userId = 7, username = "alice"),
        )
        val profileId = managerWithSecrets.activeProfileId()!!

        managerWithSecrets.clearSession(keepServerUrl = false)

        assertNull(managerWithSecrets.bearerToken())
        assertTrue(managerWithSecrets.profiles().isEmpty())
        assertNull(managerWithSecrets.serverUrl())
        assertEquals(listOf(profileId), secrets.cleared)
        assertEquals(SessionState(), managerWithSecrets.observeSession().first())
        // sanity: the plain manager variable is unused but keeps the helper honest
        assertTrue(manager.profiles().isEmpty())
    }

    @Test
    fun `clearSession keeps the server url across a process restart`() = runTest {
        val tokenStorage = FakeTokenStorage()
        val prefsStorage = FakeSessionPrefsStorage()
        val first = manager(tokenStorage = tokenStorage, prefsStorage = prefsStorage)
        first.setSession("https://crm.example.com", "jwt-1", SessionState(userId = 7))

        first.clearSession()

        // A fresh manager (new process) hydrates the retained profile — logout
        // must not force the user to re-type it after an app relaunch.
        val restarted = manager(tokenStorage = tokenStorage, prefsStorage = prefsStorage)
        restarted.init()
        assertEquals("https://crm.example.com", restarted.serverUrl())
        assertNull(restarted.bearerToken())
    }

    @Test
    fun `clearSession wipes cached user data through the session data cleaner`() = runTest {
        val cleaner = RecordingSessionDataCleaner()
        val manager = manager(cleaner = cleaner)
        manager.setSession("https://crm.example.com", "jwt-1", SessionState(userId = 7))

        manager.clearSession()

        assertEquals(1, cleaner.clearCount)
    }

    @Test
    fun `clearSession does not require a session data cleaner`() = runTest {
        val manager = manager()
        manager.setSession("https://crm.example.com", "jwt-1", SessionState(userId = 7))

        manager.clearSession()

        assertNull(manager.bearerToken())
    }

    private class RecordingSessionDataCleaner : SessionDataCleaner {
        var clearCount = 0
        override suspend fun clear() {
            clearCount++
        }
    }

    private class RecordingSecrets : ProfileSecretStorage {
        val migrated = mutableListOf<String>()
        val cleared = mutableListOf<String>()
        override suspend fun migrateLegacy(profileId: String) {
            migrated += profileId
        }

        override suspend fun clear(profileId: String) {
            cleared += profileId
        }
    }

    // Issue #957 (finding #1): the FCM-deregistration bug was that
    // clearSession() dropped the bearer BEFORE the reactive listener that
    // deregisters the device ever ran, so the request went out unauthenticated
    // and 401ed. These pin the fix: SessionTeardown.beforeClear must see the
    // still-valid session, must run exactly once per real clear, must not run
    // again for a redundant/already-logged-out clear (no request to make), and
    // a teardown failure must never block the local clear itself.

    @Test
    fun `clearSession runs the teardown step while the session is still authenticated`() = runTest {
        var tokenSeenDuringTeardown: String? = null
        lateinit var manager: DefaultSessionManager
        manager = manager(teardown = SessionTeardown { tokenSeenDuringTeardown = manager.bearerToken() })
        manager.setSession("https://crm.example.com", "jwt-1", SessionState(userId = 7))

        manager.clearSession()

        assertEquals("jwt-1", tokenSeenDuringTeardown)
        assertNull("the token must still be dropped after teardown runs", manager.bearerToken())
    }

    @Test
    fun `a teardown failure does not block the local clear`() = runTest {
        val manager = manager(teardown = SessionTeardown { error("network unreachable") })
        manager.setSession("https://crm.example.com", "jwt-1", SessionState(userId = 7))

        manager.clearSession()

        assertNull("the local clear must still happen despite the teardown throwing", manager.bearerToken())
        assertFalse(manager.observeSession().first().isLoggedIn)
    }

    @Test
    fun `clearSession on an already-logged-out session makes no teardown call`() = runTest {
        var calls = 0
        val manager = manager(teardown = SessionTeardown { calls++ })

        manager.clearSession()

        assertEquals("no active session means nothing to tear down", 0, calls)
    }

    @Test
    fun `a redundant clearSession call after a real one makes no second teardown call`() = runTest {
        var calls = 0
        val manager = manager(teardown = SessionTeardown { calls++ })
        manager.setSession("https://crm.example.com", "jwt-1", SessionState(userId = 7))

        manager.clearSession()
        manager.clearSession()

        assertEquals(1, calls)
    }

    @Test
    fun `isClearingSession is true only for the duration of the teardown call`() = runTest {
        lateinit var manager: DefaultSessionManager
        var duringTeardown = false
        manager = manager(teardown = SessionTeardown { duringTeardown = manager.isClearingSession() })
        manager.setSession("https://crm.example.com", "jwt-1", SessionState(userId = 7))
        assertFalse("not clearing before logout", manager.isClearingSession())

        manager.clearSession()

        assertTrue("the flag must have been up while teardown ran", duringTeardown)
        assertFalse("the flag must drop back down once clearSession returns", manager.isClearingSession())
    }

    @Test
    fun `setServerUrl creates the first profile`() = runTest {
        val (manager, _) = managerPair()
        manager.setServerUrl("https://beta.example.com")

        assertEquals("https://beta.example.com", manager.baseUrl())
        assertEquals("https://beta.example.com", manager.serverUrl())
        assertEquals(1, manager.profiles().size)
        assertEquals("beta.example.com", manager.profiles().single().label)
    }

    @Test
    fun `setServerUrl re-points the active profile in place without duplicating it`() = runTest {
        val (manager, _) = managerPair()
        manager.setServerUrl("https://one.example.com")
        val originalId = manager.profiles().single().id

        manager.setServerUrl("https://two.example.com")

        assertEquals(1, manager.profiles().size)
        assertEquals(originalId, manager.profiles().single().id)
        assertEquals("https://two.example.com", manager.baseUrl())
    }

    @Test
    fun `setProfile merges profile without clearing login`() = runTest {
        val (manager, _) = managerPair()
        manager.setSession("https://crm.example.com", "jwt", SessionState(username = "alice"))
        manager.setProfile(SessionState(isAdmin = true, language = "de"))

        val state = manager.observeSession().first()
        assertTrue(state.isLoggedIn)
        assertTrue(state.isAdmin)
        assertEquals("de", state.language)
        assertEquals("alice", state.username)
    }

    // T90 / issue #831: setSelfContactVCardUid always overwrites (unlike
    // setProfile's null-keeps-current merge), because clearing the pointer
    // to null IS a legitimate target state here.
    @Test
    fun `setSelfContactVCardUid overwrites the pointer, including clearing it to null`() = runTest {
        val (manager, _) = managerPair()
        manager.setSession("https://crm.example.com", "jwt", SessionState(selfContactVCardUid = "uid-1"))

        manager.setSelfContactVCardUid("uid-2")
        assertEquals("uid-2", manager.observeSession().first().selfContactVCardUid)

        manager.setSelfContactVCardUid(null)
        assertNull(manager.observeSession().first().selfContactVCardUid)
    }

    // Issue #814: a token_version bump (2FA confirm/disable) re-issues the
    // session as a fresh bearer token. setToken swaps it in place — the
    // session stays logged in and the server URL/profile are untouched.
    @Test
    fun `setToken replaces the token in place without disturbing the session`() = runTest {
        val (manager, tokenStorage) = managerPair()
        manager.setSession(
            serverUrl = "https://crm.example.com",
            token = "old-jwt",
            state = SessionState(userId = 7, username = "alice"),
        )

        manager.setToken("reissued-jwt")

        assertEquals("reissued-jwt", manager.bearerToken())
        assertEquals("reissued-jwt", tokenStorage.tokens.values.single())
        assertEquals("https://crm.example.com", manager.baseUrl())
        val state = manager.observeSession().first()
        assertTrue(state.isLoggedIn)
        assertEquals(7, state.userId)
        assertEquals("alice", state.username)
    }

    // M5 §5: a cold-start OIDC deep link must await hydration before reading
    // the server URL / writing the session — otherwise it races the async
    // startup init() (review-pass fix).
    @Test
    fun `awaitHydrated suspends until init completes`() = runTest {
        val tokenStorage = FakeTokenStorage()
        val prefs = FakeSessionPrefsStorage()
        prefs.snapshot = ProfilesSnapshot(
            profiles = listOf(ServerProfile("p1", ServerProfileKind.Remote("https://a.example"), "a")),
            activeProfileId = "p1",
        )
        tokenStorage.tokens["p1"] = "stored-jwt"
        val manager = manager(tokenStorage = tokenStorage, prefsStorage = prefs)

        var hydrated = false
        async { manager.awaitHydrated(); hydrated = true }

        advanceUntilIdle()
        assertFalse(hydrated)

        manager.init()
        advanceUntilIdle()

        assertTrue(hydrated)
        assertEquals("stored-jwt", manager.bearerToken())
    }

    // Issue #678: the session state machine must walk the full lifecycle —
    // logged-out → authenticated → (401) → logged-out → re-authenticated —
    // emitting the right SessionState at every step.
    @Test
    fun `session walks the full lifecycle state machine`() = runTest {
        val (manager, _) = managerPair()

        manager.observeSession().test {
            assertEquals(SessionState(), awaitItem())

            manager.setSession(
                serverUrl = "https://crm.example.com",
                token = "jwt-1",
                state = SessionState(userId = 7, username = "alice"),
            )
            val authenticated = awaitItem()
            assertTrue(authenticated.isLoggedIn)
            assertEquals(7, authenticated.userId)
            assertEquals("alice", authenticated.username)

            manager.clearSession()
            val loggedOut = awaitItem()
            assertFalse(loggedOut.isLoggedIn)
            assertNull(loggedOut.userId)
            assertFalse("no stale username survives logout", loggedOut.username != null)
            assertEquals("https://crm.example.com", loggedOut.serverUrl)

            manager.setSession(
                serverUrl = "https://crm.example.com",
                token = "jwt-2",
                state = SessionState(userId = 7, username = "alice"),
            )
            val reAuthenticated = awaitItem()
            assertTrue(reAuthenticated.isLoggedIn)
            assertEquals("jwt-2", manager.bearerToken())
            assertEquals("alice", reAuthenticated.username)
        }
    }

    // Issue #678: process-death restore — a fresh manager instance (a new
    // process) must hydrate the persisted token and surface the same
    // logged-in state the old instance held.
    @Test
    fun `a fresh instance restores the session after process death`() = runTest {
        val tokenStorage = FakeTokenStorage()
        val prefsStorage = FakeSessionPrefsStorage()
        val first = manager(tokenStorage = tokenStorage, prefsStorage = prefsStorage)
        first.setSession(
            serverUrl = "https://crm.example.com",
            token = "jwt-1",
            state = SessionState(userId = 7, username = "alice"),
        )

        val restarted = manager(tokenStorage = tokenStorage, prefsStorage = prefsStorage)
        restarted.init()

        assertEquals("jwt-1", restarted.bearerToken())
        assertEquals("https://crm.example.com", restarted.baseUrl())
        val state = restarted.observeSession().first()
        assertTrue(state.isLoggedIn)
    }

    // --- ADR 0028 Decision 1: profiles, switching, isolation ------------------

    @Test
    fun `per-profile tokens are isolated across a switch`() = runTest {
        val tokenStorage = FakeTokenStorage()
        val prefs = FakeSessionPrefsStorage()
        val manager = manager(tokenStorage = tokenStorage, prefsStorage = prefs)
        manager.init()

        manager.setServerUrl("https://one.example.com")
        val one = manager.activeProfileId()!!
        manager.setSession("https://one.example.com", "jwt-one", SessionState(userId = 1))

        val two = manager.addRemoteProfile("Two", "https://two.example.com").id
        assertEquals(SwitchProfileResult.Switched, manager.switchProfile(two))
        // The new profile has no credential yet: the app lands logged out.
        assertNull(manager.bearerToken())
        assertEquals("https://two.example.com", manager.baseUrl())
        manager.setSession("https://two.example.com", "jwt-two", SessionState(userId = 2))

        assertEquals("jwt-two", tokenStorage.tokens[two])
        assertEquals("jwt-one", tokenStorage.tokens[one])
        assertEquals("jwt-two", manager.bearerToken())

        // Switch back: profile one's token is still its own.
        assertEquals(SwitchProfileResult.Switched, manager.switchProfile(one))
        assertEquals("jwt-one", manager.bearerToken())
        assertEquals("https://one.example.com", manager.baseUrl())
    }

    @Test
    fun `switchProfile clears the Room mirror through the data cleaner`() = runTest {
        val cleaner = RecordingSessionDataCleaner()
        val manager = manager(cleaner = cleaner)
        manager.init()
        manager.setServerUrl("https://one.example.com")
        val one = manager.activeProfileId()!!
        val two = manager.addRemoteProfile("Two", "https://two.example.com").id

        assertEquals(SwitchProfileResult.Switched, manager.switchProfile(two))
        assertEquals("switching must wipe the previous profile's cached mirror", 1, cleaner.clearCount)

        manager.switchProfile(one)
        assertEquals(2, cleaner.clearCount)
    }

    @Test
    fun `switchProfile is a no-op for the already-active profile`() = runTest {
        val cleaner = RecordingSessionDataCleaner()
        val manager = manager(cleaner = cleaner)
        manager.init()
        manager.setServerUrl("https://one.example.com")
        val one = manager.activeProfileId()!!

        assertEquals(SwitchProfileResult.Switched, manager.switchProfile(one))
        assertEquals("no switch happened, so no cache wipe", 0, cleaner.clearCount)
    }

    @Test
    fun `switchProfile requires confirmation when the outbox is non-empty`() = runTest {
        val pending = FakePendingInteractions(unsyncedCount = 3)
        val cleaner = RecordingSessionDataCleaner()
        val manager = manager(cleaner = cleaner, pending = pending)
        manager.init()
        manager.setServerUrl("https://one.example.com")
        val one = manager.activeProfileId()!!
        val two = manager.addRemoteProfile("Two", "https://two.example.com").id

        // A no-op drainer cannot empty the outbox, so the confirmation is raised
        // and — crucially — nothing is switched or wiped yet.
        val result = manager.switchProfile(two)
        assertEquals(SwitchProfileResult.NeedsConfirmation(3), result)
        assertEquals(one, manager.activeProfileId())
        assertEquals(0, cleaner.clearCount)

        // Confirming the discard proceeds.
        assertEquals(SwitchProfileResult.Switched, manager.switchProfile(two, discardPending = true))
        assertEquals(two, manager.activeProfileId())
        assertEquals(1, cleaner.clearCount)
    }

    @Test
    fun `switchProfile drains the outbox first when a drainer clears it`() = runTest {
        val pending = FakePendingInteractions(unsyncedCount = 2)
        val manager = manager(
            pending = pending,
            drainer = OutboxDrainer { pending.unsyncedCount = 0; true },
        )
        manager.init()
        manager.setServerUrl("https://one.example.com")
        val two = manager.addRemoteProfile("Two", "https://two.example.com").id

        // The drain succeeded, so no confirmation is needed.
        assertEquals(SwitchProfileResult.Switched, manager.switchProfile(two))
        assertEquals(two, manager.activeProfileId())
    }

    @Test
    fun `removeProfile drops a non-active profile's credentials without a teardown`() = runTest {
        val tokenStorage = FakeTokenStorage()
        val secrets = RecordingSecrets()
        var teardownCalls = 0
        val manager = manager(
            tokenStorage = tokenStorage,
            secrets = secrets,
            teardown = SessionTeardown { teardownCalls++ },
        )
        manager.init()
        manager.setServerUrl("https://one.example.com")
        val one = manager.activeProfileId()!!
        val two = manager.addRemoteProfile("Two", "https://two.example.com").id
        tokenStorage.tokens[two] = "jwt-two"

        manager.removeProfile(two)

        assertEquals(listOf(two), secrets.cleared)
        assertNull(tokenStorage.tokens[two])
        assertEquals(one, manager.activeProfileId())
        assertEquals("a non-active profile has no bearer to revoke with", 0, teardownCalls)
    }

    @Test
    fun `removeProfile revokes the active profile session through teardown`() = runTest {
        val tokenStorage = FakeTokenStorage()
        val secrets = RecordingSecrets()
        var tokenSeenDuringTeardown: String? = null
        val manager = manager(
            tokenStorage = tokenStorage,
            secrets = secrets,
            teardown = SessionTeardown { tokenSeenDuringTeardown = managerHolder!!.bearerToken() },
        )
        managerHolder = manager
        manager.init()
        manager.setServerUrl("https://one.example.com")
        val one = manager.activeProfileId()!!
        manager.setSession("https://one.example.com", "jwt-one", SessionState(userId = 1))

        manager.removeProfile(one)

        assertEquals("jwt-one", tokenSeenDuringTeardown)
        assertEquals(listOf(one), secrets.cleared)
        assertTrue(manager.profiles().isEmpty())
        assertNull(manager.bearerToken())
    }

    private var managerHolder: DefaultSessionManager? = null

    @Test
    fun `renameProfile updates the label without touching credentials`() = runTest {
        val (manager, _) = managerPair()
        manager.setServerUrl("https://one.example.com")
        val one = manager.activeProfileId()!!
        manager.setSession("https://one.example.com", "jwt-one", SessionState(userId = 1))

        manager.renameProfile(one, "Home server")

        assertEquals("Home server", manager.profiles().single().label)
        assertEquals("jwt-one", manager.bearerToken())
    }

    @Test
    fun `observeProfiles and observeActiveProfile reflect mutations`() = runTest {
        val (manager, _) = managerPair()
        manager.init()

        assertTrue(manager.observeProfiles().first().isEmpty())
        assertNull(manager.observeActiveProfile().first())

        manager.setServerUrl("https://one.example.com")

        assertEquals(1, manager.observeProfiles().first().size)
        assertEquals("https://one.example.com", manager.observeActiveProfile().first()?.remoteUrl)
    }

    // --- ADR 0028 Decision 2 / issue #1262: Local profiles -------------------

    @Test
    fun `activateLocalProfile creates one Local profile and mints a sentinel session`() = runTest {
        val tokenStorage = FakeTokenStorage()
        val manager = manager(tokenStorage = tokenStorage)
        manager.init()

        val profile = manager.activateLocalProfile("local-token")

        assertTrue(profile.kind is ServerProfileKind.Local)
        assertEquals("local-token", tokenStorage.tokens[profile.id])
        // The sentinel origin is what routes requests over the Unix socket and
        // drives the /health compatibility check for a Local profile.
        assertEquals(LOCAL_SERVER_SENTINEL_URL, manager.baseUrl())
        assertEquals(LOCAL_SERVER_SENTINEL_URL, manager.serverUrl())
        assertTrue(manager.observeSession().first().isLoggedIn)
        assertEquals(profile.id, manager.activeProfileId())
    }

    @Test
    fun `activating a Local profile twice reuses the same profile and refreshes its token`() = runTest {
        val tokenStorage = FakeTokenStorage()
        val manager = manager(tokenStorage = tokenStorage)
        manager.init()

        val first = manager.activateLocalProfile("token-one")
        val second = manager.activateLocalProfile("token-two")

        assertEquals(first.id, second.id)
        assertEquals(1, manager.profiles().size)
        // Each app start mints a fresh token; the stored credential tracks it.
        assertEquals("token-two", tokenStorage.tokens[first.id])
    }

    @Test
    fun `switching into a Local profile clears the previous profile's Room mirror`() = runTest {
        val cleaner = RecordingSessionDataCleaner()
        val manager = manager(cleaner = cleaner)
        manager.init()
        manager.setServerUrl("https://remote.example.com")
        manager.setSession("https://remote.example.com", "remote-jwt", SessionState(userId = 1))

        manager.activateLocalProfile("local-token")

        assertEquals(1, cleaner.clearCount)
        assertEquals(LOCAL_SERVER_SENTINEL_URL, manager.baseUrl())
    }

    @Test
    fun `switching from Local back to a Remote profile restores that URL`() = runTest {
        val manager = manager()
        manager.init()
        val remote = manager.addRemoteProfile("Remote", "https://remote.example.com")
        manager.setSession("https://remote.example.com", "remote-jwt", SessionState(userId = 1))

        manager.activateLocalProfile("local-token")
        assertEquals(LOCAL_SERVER_SENTINEL_URL, manager.baseUrl())

        manager.switchProfile(remote.id)

        assertEquals("https://remote.example.com", manager.baseUrl())
    }

    /** Minimal pending-interactions fake: only the count is exercised here. */
    private class FakePendingInteractions(var unsyncedCount: Int) : PendingInteractionRepository {
        override suspend fun record(interaction: PendingInteraction) = Unit
        override suspend fun unsynced(): List<PendingInteraction> =
            List(unsyncedCount) { PendingInteraction(timestampMillis = it.toLong(), kind = "call") }

        override suspend fun markSynced(id: Long, syncedAt: String) = Unit
        override suspend fun deleteSynced() = Unit
        override suspend fun recordIfNew(interaction: PendingInteraction): Boolean = true
        override suspend fun setIdempotencyKey(id: Long, key: String) = Unit
        override suspend fun clearMatchedContact(id: Long) = Unit
    }

    // --- Issue #1265: attach-to-remote support ---------------------------------

    @Test
    fun `saveProfileToken stores a non-active profile's token without activating it`() = runTest {
        val tokenStorage = FakeTokenStorage()
        val manager = manager(tokenStorage = tokenStorage)
        manager.init()
        manager.activateLocalProfile("local-token")
        val remote = manager.addRemoteProfile("Home", "https://home.example")

        manager.saveProfileToken(remote.id, "remote-token")

        assertEquals("remote-token", tokenStorage.tokens[remote.id])
        // The active (Local) session is untouched — the wizard has not switched yet.
        assertEquals("local-token", manager.bearerToken())
        assertEquals(ServerProfileKind.Local, manager.activeProfile()?.kind)
    }

    @Test
    fun `saveProfileToken on the active profile also updates the in-memory bearer`() = runTest {
        val manager = manager()
        manager.init()
        val local = manager.activateLocalProfile("old")

        manager.saveProfileToken(local.id, "new")

        assertEquals("new", manager.bearerToken())
    }

    @Test
    fun `saveProfileToken ignores an unknown profile`() = runTest {
        val tokenStorage = FakeTokenStorage()
        val manager = manager(tokenStorage = tokenStorage)
        manager.init()

        manager.saveProfileToken("nope", "t")

        assertTrue(tokenStorage.tokens.isEmpty())
    }

    @Test
    fun `setProfileArchived marks a local profile and reports it when active`() = runTest {
        val prefs = FakeSessionPrefsStorage()
        val manager = manager(prefsStorage = prefs)
        manager.init()
        val local = manager.activateLocalProfile("t")
        assertFalse(manager.isActiveProfileArchived())

        manager.setProfileArchived(local.id, true)

        assertTrue(manager.isActiveProfileArchived())
        assertTrue(manager.activeProfile()?.archived == true)
        assertTrue("the flag is persisted", prefs.snapshot.profiles.single().archived)
        manager.observeActiveProfile().test {
            assertTrue(awaitItem()?.archived == true)
        }
    }

    @Test
    fun `an archived profile is not reported archived once another profile is active`() = runTest {
        val tokenStorage = FakeTokenStorage()
        val manager = manager(tokenStorage = tokenStorage)
        manager.init()
        val local = manager.activateLocalProfile("t")
        val remote = manager.addRemoteProfile("Home", "https://home.example")
        manager.saveProfileToken(remote.id, "r")
        manager.setProfileArchived(local.id, true)

        manager.switchProfile(remote.id)

        assertFalse(manager.isActiveProfileArchived())
    }

    @Test
    fun `setProfileArchived can clear the mark`() = runTest {
        val manager = manager()
        manager.init()
        val local = manager.activateLocalProfile("t")
        manager.setProfileArchived(local.id, true)

        manager.setProfileArchived(local.id, false)

        assertFalse(manager.isActiveProfileArchived())
    }

    @Test
    fun `a remote profile can never be archived`() = runTest {
        val manager = manager()
        manager.init()
        val remote = manager.addRemoteProfile("Home", "https://home.example")
        manager.switchProfile(remote.id)

        manager.setProfileArchived(remote.id, true)

        assertFalse(manager.isActiveProfileArchived())
        assertFalse(manager.profiles().single().archived)
    }

    @Test
    fun `setProfileArchived ignores an unknown id`() = runTest {
        val manager = manager()
        manager.init()

        manager.setProfileArchived("nope", true)

        assertFalse(manager.isActiveProfileArchived())
    }
}

