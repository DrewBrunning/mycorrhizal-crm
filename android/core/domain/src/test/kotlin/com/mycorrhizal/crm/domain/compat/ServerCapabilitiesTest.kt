package com.mycorrhizal.crm.domain.compat

import com.mycorrhizal.crm.model.AppVersion
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ServerCapabilitiesTest {

    private fun v(major: Int, minor: Int, patch: Int): AppVersion = AppVersion(major, minor, patch)

    // --- isSupported: the per-feature gate -------------------------------

    @Test
    fun `unknown server version fails open to supported`() {
        // The fail-open rule (issue #692): a null server version (no /health
        // yet, unreachable, or unparseable "dev" build) never hides a feature.
        ServerFeature.entries.forEach { feature ->
            assertTrue(ServerCapabilities.isSupported(server = null, feature = feature))
        }
    }

    @Test
    fun `a server at or above a feature floor supports it`() {
        // 1.7.0 is the newest floor in the registry, so a 1.7.0 server supports everything.
        val newest = v(1, 7, 0)
        ServerFeature.entries.forEach { feature ->
            assertTrue(
                "server 1.7.0 must support ${feature.name}",
                ServerCapabilities.isSupported(newest, feature)
            )
        }
    }

    @Test
    fun `baseline features are supported by any server at or above the baseline`() {
        val baselineServer = v(1, 0, 0)
        ServerFeature.entries
            .filter { it.minServerVersion == ServerCapabilities.MIN_SUPPORTED_SERVER_VERSION }
            .forEach { feature ->
                assertTrue(
                    "server 1.0.0 must support baseline ${feature.name}",
                    ServerCapabilities.isSupported(baselineServer, feature)
                )
            }
    }

    @Test
    fun `a server below a feature floor does not support it`() {
        assertFalse(ServerCapabilities.isSupported(v(0, 6, 0), ServerFeature.SYSTEM_EVENTS))
        assertFalse(ServerCapabilities.isSupported(v(0, 6, 1), ServerFeature.SYSTEM_EVENTS))
        assertFalse(ServerCapabilities.isSupported(v(0, 6, 0), ServerFeature.AUDIT_EXPORT))
        assertFalse(ServerCapabilities.isSupported(v(0, 6, 0), ServerFeature.API_TOKENS_ADVANCED))
        assertFalse(ServerCapabilities.isSupported(v(0, 6, 9), ServerFeature.DEVICE_GRANT_SIGNIN))
    }

    @Test
    fun `a server at or above the floor supports the capability`() {
        assertTrue(ServerCapabilities.isSupported(v(0, 6, 2), ServerFeature.SYSTEM_EVENTS))
        assertTrue(ServerCapabilities.isSupported(v(0, 6, 10), ServerFeature.SYSTEM_EVENTS))
        assertTrue(ServerCapabilities.isSupported(v(0, 6, 1), ServerFeature.AUDIT_EXPORT))
        assertTrue(ServerCapabilities.isSupported(v(0, 6, 1), ServerFeature.API_TOKENS_ADVANCED))
        assertTrue(ServerCapabilities.isSupported(v(0, 6, 10), ServerFeature.DEVICE_GRANT_SIGNIN))
    }

    // --- isServerSupported: the baseline / "server too old" gate -----------

    @Test
    fun `unknown server version fails open past the baseline gate`() {
        assertTrue(ServerCapabilities.isServerSupported(null))
    }

    @Test
    fun `a server at or above the baseline is supported`() {
        assertTrue(ServerCapabilities.isServerSupported(v(1, 0, 0)))
        assertTrue(ServerCapabilities.isServerSupported(v(1, 0, 1)))
        assertTrue(ServerCapabilities.isServerSupported(v(2, 0, 0)))
    }

    @Test
    fun `a server below the baseline is refused`() {
        assertFalse(ServerCapabilities.isServerSupported(v(0, 9, 0)))
        assertFalse(ServerCapabilities.isServerSupported(v(0, 6, 10)))
        assertFalse(ServerCapabilities.isServerSupported(v(0, 5, 9)))
        assertFalse(ServerCapabilities.isServerSupported(v(0, 0, 1)))
    }

    // --- registry sanity: the floors are strictly increasing and derive from
    // released server versions ----------------------------------------------

    @Test
    fun `post-1_6_0 capabilities are floored at 1_7_0 and hidden from a 1_6_0 server`() {
        // Issue #2527. Ground truth: none of share grants, entities (incl. from-contacts,
        // suggestions and the entity trash listing) or the import-run report is in the v1.6.0
        // route table, so the first release carrying them is v1.7.0.
        val floors = mapOf(
            ServerFeature.SHARE_GRANTS to v(1, 7, 0),
            ServerFeature.ENTITIES to v(1, 7, 0),
            ServerFeature.IMPORT_RUN_REPORT to v(1, 7, 0)
        )
        floors.forEach { (feature, floor) ->
            assertEquals(feature.name, floor, feature.minServerVersion)
            assertFalse(feature.name, ServerCapabilities.isSupported(v(1, 6, 0), feature))
            assertFalse(feature.name, ServerCapabilities.isSupported(v(1, 6, 99), feature))
            assertTrue(feature.name, ServerCapabilities.isSupported(v(1, 7, 0), feature))
            assertTrue(feature.name, ServerCapabilities.isSupported(v(2, 0, 0), feature))
            assertTrue(feature.name, ServerCapabilities.isSupported(null, feature))
        }
    }

    @Test
    fun `a post-baseline floor does not hide a baseline capability from a 1_6_0 server`() {
        // The 1.7.0 floors must not leak onto capabilities that shipped by the baseline.
        ServerFeature.entries
            .filter { it.minServerVersion <= ServerCapabilities.MIN_SUPPORTED_SERVER_VERSION }
            .forEach { feature ->
                assertTrue(feature.name, ServerCapabilities.isSupported(v(1, 6, 0), feature))
            }
    }

    @Test
    fun `no feature floor is above the newest released server tag`() {
        // The registry may only name versions that ship (or will ship) as released tags; the
        // highest floor is the unreleased v1.7.0, which this test pins so a typo (1.70.0) fails.
        assertEquals(v(1, 7, 0), ServerFeature.entries.maxOf { it.minServerVersion })
    }
}
