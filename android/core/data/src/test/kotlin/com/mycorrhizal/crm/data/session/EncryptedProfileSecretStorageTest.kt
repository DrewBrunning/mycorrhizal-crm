package com.mycorrhizal.crm.data.session

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * ADR 0028 Decision 1: the device grant (issue #722) and pending OIDC request
 * (issue #965) move from their legacy single-profile slots onto the migrated
 * profile, and are dropped when a profile is removed.
 */
class EncryptedProfileSecretStorageTest {

    private class FakeDeviceGrantStorage : DeviceGrantTokenStorage {
        val grants = mutableMapOf<String, Pair<String, Long>>()
        var legacy: LegacyDeviceGrant? = null

        override suspend fun save(profileId: String, token: String, id: Long) {
            grants[profileId] = token to id
        }
        override suspend fun loadToken(profileId: String): String? = grants[profileId]?.first
        override suspend fun loadGrantId(profileId: String): Long? = grants[profileId]?.second?.takeIf { it != 0L }
        override suspend fun clear(profileId: String) {
            grants.remove(profileId)
        }
        override suspend fun loadLegacy(): LegacyDeviceGrant? = legacy
        override suspend fun clearLegacy() {
            legacy = null
        }
    }

    private class FakeOidcStore : OidcPendingRequestStore {
        val pending = mutableMapOf<String, OidcPendingRequest>()
        var legacy: OidcPendingRequest? = null

        override suspend fun save(profileId: String, state: String, codeVerifier: String) {
            pending[profileId] = OidcPendingRequest(state, codeVerifier, 1L)
        }
        override suspend fun load(profileId: String): OidcPendingRequest? = pending[profileId]
        override suspend fun clear(profileId: String) {
            pending.remove(profileId)
        }
        override suspend fun loadLegacy(): OidcPendingRequest? = legacy
        override suspend fun clearLegacy() {
            legacy = null
        }
    }

    private val deviceGrant = FakeDeviceGrantStorage()
    private val oidc = FakeOidcStore()
    private val storage = EncryptedProfileSecretStorage(deviceGrant, oidc)

    @Test
    fun `migrateLegacy moves the legacy grant and pending request onto the profile`() = runTest {
        deviceGrant.legacy = LegacyDeviceGrant("grant-token", 7L)
        oidc.legacy = OidcPendingRequest("state-a", "verifier-a", 1L)

        storage.migrateLegacy("p1")

        assertEquals("grant-token" to 7L, deviceGrant.grants["p1"])
        assertEquals("state-a" to "verifier-a", oidc.pending["p1"]?.let { it.state to it.codeVerifier })
        assertNull("the legacy grant slot must be cleared", deviceGrant.legacy)
        assertNull("the legacy OIDC slot must be cleared", oidc.legacy)
    }

    @Test
    fun `migrateLegacy with nothing stored is a no-op`() = runTest {
        storage.migrateLegacy("p1")

        assertEquals(emptyMap<String, Pair<String, Long>>(), deviceGrant.grants)
        assertEquals(emptyMap<String, OidcPendingRequest>(), oidc.pending)
    }

    @Test
    fun `clear drops only the named profile's secrets`() = runTest {
        deviceGrant.grants["p1"] = "one" to 1L
        deviceGrant.grants["p2"] = "two" to 2L
        oidc.pending["p1"] = OidcPendingRequest("s1", "v1", 1L)
        oidc.pending["p2"] = OidcPendingRequest("s2", "v2", 2L)

        storage.clear("p1")

        assertNull(deviceGrant.grants["p1"])
        assertNull(oidc.pending["p1"])
        assertEquals("two" to 2L, deviceGrant.grants["p2"])
        assertEquals("s2", oidc.pending["p2"]?.state)
    }

    private fun runTest(block: suspend () -> Unit) = kotlinx.coroutines.test.runTest { block() }
}
