package com.mycorrhizal.crm.domain.compat

import com.mycorrhizal.crm.model.network.ServerHealth
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * ADR 0028 Decision 2 / issue #1263: capability-gated UI. The load-bearing
 * property is fail-open — an absent `capabilities` list (an older server, or an
 * unreachable /health) must never hide functionality.
 */
class ServerCapabilitiesInfoTest {

    @Test
    fun `an absent capability list fails open to everything present`() {
        val unknown = ServerCapabilitiesInfo(deployment = null, capabilities = null)
        assertTrue(unknown.supports(ServerCapability.PUSH))
        assertTrue(unknown.supports(ServerCapability.CONTACT_SHARES))
        assertTrue(unknown.supports("some_future_token"))
    }

    @Test
    fun `a present capability list is authoritative`() {
        val embedded = ServerCapabilitiesInfo(
            deployment = ServerCapabilitiesInfo.DEPLOYMENT_EMBEDDED,
            capabilities = setOf(ServerCapability.CONTACTS, ServerCapability.EXPORT),
        )
        assertTrue(embedded.supports(ServerCapability.CONTACTS))
        assertTrue(embedded.supports(ServerCapability.EXPORT))
        assertFalse(embedded.supports(ServerCapability.CALENDAR))
        assertFalse(embedded.supports(ServerCapability.LOGIN))
        assertFalse(embedded.supports(ServerCapability.PUSH))
        assertFalse(embedded.supports(ServerCapability.CARDDAV))
        assertTrue(embedded.isEmbedded)
    }

    @Test
    fun `a present but empty list means nothing is available`() {
        val empty = ServerCapabilitiesInfo(deployment = "server", capabilities = emptySet())
        assertFalse(empty.supports(ServerCapability.CONTACTS))
    }

    @Test
    fun `embedded detection is exact, not a substring`() {
        assertFalse(ServerCapabilitiesInfo(deployment = "server").isEmbedded)
        assertFalse(ServerCapabilitiesInfo(deployment = null).isEmbedded)
        assertTrue(ServerCapabilitiesInfo(deployment = "embedded").isEmbedded)
    }

    @Test
    fun `health projects onto the capability model`() {
        val health = ServerHealth(
            version = "1.2.0",
            deployment = "embedded",
            capabilities = listOf("contacts", "calendar"),
        )
        val info = health.toCapabilitiesInfo()
        assertEquals("embedded", info.deployment)
        assertTrue(info.supports("contacts"))
        assertFalse(info.supports("push"))
    }

    @Test
    fun `health without the fields projects to the fail-open default`() {
        val info = ServerHealth(version = "1.0.0").toCapabilitiesInfo()
        assertEquals(ServerCapabilitiesInfo.Unknown, info)
        assertTrue(info.supports("push"))
    }
}
