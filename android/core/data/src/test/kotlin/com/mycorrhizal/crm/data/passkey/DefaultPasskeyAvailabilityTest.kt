package com.mycorrhizal.crm.data.passkey

import com.mycorrhizal.crm.data.compat.DefaultServerCapabilitiesStore
import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.domain.compat.ServerCapabilitiesInfo
import com.mycorrhizal.crm.domain.profile.ServerProfile
import com.mycorrhizal.crm.domain.profile.ServerProfileKind
import com.mycorrhizal.crm.domain.repository.ServerCompatibilityRepository
import com.mycorrhizal.crm.model.network.ServerHealth
import com.mycorrhizal.crm.testing.FakePasskeyClient
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Issue #1293 / ADR 0034 Decision 4: the availability gate matrix. Remote profile
 * + `webauthn_android` in `/health` + Credential Manager usable => open; every
 * other combination stays closed.
 */
class DefaultPasskeyAvailabilityTest {

    private val remote = ServerProfile("p1", ServerProfileKind.Remote("https://crm.example.com"), "Home")
    private val local = ServerProfile("p2", ServerProfileKind.Local, "On this device")

    private class Harness(
        val client: FakePasskeyClient,
        val store: DefaultServerCapabilitiesStore,
        val sessions: SessionManager,
        val compat: ServerCompatibilityRepository,
        val gate: DefaultPasskeyAvailability,
    )

    private fun harness(
        profile: ServerProfile? = remote,
        supported: Boolean = true,
        recorded: ServerCapabilitiesInfo = ServerCapabilitiesInfo(deployment = "server", capabilities = setOf("contacts", "webauthn_android")),
        health: Result<ServerHealth> = Result.failure(IllegalStateException("must not be asked")),
    ): Harness {
        val client = FakePasskeyClient(supported)
        val store = DefaultServerCapabilitiesStore().also { it.record(recorded) }
        val sessions = mockk<SessionManager>()
        coEvery { sessions.activeProfile() } returns profile
        val compat = mockk<ServerCompatibilityRepository>()
        coEvery { compat.getServerHealth() } returns health
        return Harness(client, store, sessions, compat, DefaultPasskeyAvailability(client, store, sessions, compat))
    }

    @Test
    fun `remote profile with the token and a supported device is available`() = runTest {
        assertTrue(harness().gate.isAvailable())
    }

    @Test
    fun `a server that does not declare the token is closed`() = runTest {
        val h = harness(recorded = ServerCapabilitiesInfo(deployment = "server", capabilities = setOf("contacts", "two_factor")))
        assertFalse(h.gate.isAvailable())
    }

    @Test
    fun `an empty declared list is closed`() = runTest {
        assertFalse(harness(recorded = ServerCapabilitiesInfo(deployment = "server", capabilities = emptySet())).gate.isAvailable())
    }

    @Test
    fun `an older server that omits the capability list is closed - the token is an opt-in`() = runTest {
        // deployment set, capabilities null: the fail-open `supports` would say "everything"; this gate must not.
        val h = harness(recorded = ServerCapabilitiesInfo(deployment = "server", capabilities = null))
        assertFalse(h.gate.isAvailable())
    }

    @Test
    fun `the Local profile is never available even if a token were present`() = runTest {
        assertFalse(harness(profile = local).gate.isAvailable())
    }

    @Test
    fun `an embedded deployment is closed even without a profile`() = runTest {
        val h = harness(
            profile = null,
            recorded = ServerCapabilitiesInfo(deployment = "embedded", capabilities = setOf("webauthn_android")),
        )
        assertFalse(h.gate.isAvailable())
    }

    @Test
    fun `an unsupported device is closed before anything else is consulted`() = runTest {
        val h = harness(supported = false)
        assertFalse(h.gate.isAvailable())
        coVerify(exactly = 0) { h.sessions.activeProfile() }
    }

    @Test
    fun `a pre-login gate with no active profile still opens from the typed server's capabilities`() = runTest {
        assertTrue(harness(profile = null).gate.isAvailable())
    }

    @Test
    fun `unknown capabilities ask the server directly and stay closed when it is unreachable`() = runTest {
        val h = harness(recorded = ServerCapabilitiesInfo.Unknown, health = Result.failure(java.io.IOException("offline")))
        assertFalse(h.gate.isAvailable())
    }

    @Test
    fun `unknown capabilities are resolved from health when it answers`() = runTest {
        val open = harness(
            recorded = ServerCapabilitiesInfo.Unknown,
            health = Result.success(ServerHealth(deployment = "server", capabilities = listOf("webauthn_android"))),
        )
        assertTrue(open.gate.isAvailable())

        val closed = harness(
            recorded = ServerCapabilitiesInfo.Unknown,
            health = Result.success(ServerHealth(deployment = "server", capabilities = listOf("contacts"))),
        )
        assertFalse(closed.gate.isAvailable())
    }

    @Test
    fun `resolving from health does not write into the shared store`() = runTest {
        val h = harness(
            recorded = ServerCapabilitiesInfo.Unknown,
            health = Result.success(ServerHealth(deployment = "server", capabilities = listOf("webauthn_android"))),
        )
        h.gate.isAvailable()
        assertTrue(h.store.current() == ServerCapabilitiesInfo.Unknown)
    }

    @Test
    fun `the gate follows a profile switch - the store reset closes it until the new server resolves`() = runTest {
        val h = harness()
        assertTrue(h.gate.isAvailable())

        // MainViewModel resets the store to Unknown on a server/profile switch; the new server declares nothing.
        h.store.record(ServerCapabilitiesInfo.Unknown)
        coEvery { h.compat.getServerHealth() } returns Result.success(ServerHealth(deployment = "server", capabilities = listOf("contacts")))
        assertFalse(h.gate.isAvailable())

        // ... and it reopens once a server that declares the token is the one in the store.
        h.store.record(ServerCapabilitiesInfo(deployment = "server", capabilities = setOf("webauthn_android")))
        assertTrue(h.gate.isAvailable())
    }
}
