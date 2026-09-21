package com.mycorrhizal.crm.domain.compat

import com.mycorrhizal.crm.model.AppVersion
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
        // Every capability ships by the v1.0.0 baseline, so a 1.0.0 server
        // supports the whole registry.
        val newest = v(1, 0, 0)
        ServerFeature.entries.forEach { feature ->
            assertTrue(
                "server 1.0.0 must support ${feature.name}",
                ServerCapabilities.isSupported(newest, feature),
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
                    ServerCapabilities.isSupported(baselineServer, feature),
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
    fun `every feature floor ships at or before the baseline`() {
        // The v1.0.0 baseline (issue #1170) is the oldest server this app talks
        // to, and every capability in the registry shipped by then, so every
        // floor is <= the baseline. A floor ABOVE the baseline would be the
        // only way isSupported hides anything on a supported server.
        ServerFeature.entries.forEach { feature ->
            assertTrue(
                "floor of ${feature.name} must be <= the 1.0.0 baseline",
                feature.minServerVersion <= ServerCapabilities.MIN_SUPPORTED_SERVER_VERSION,
            )
        }
    }

    @Test
    fun `a released v1_0_0 server supports every feature`() {
        // Ground truth from the git tags: every capability the registry names
        // shipped by v1.0.0 (the device-grant endpoints in the v0.6.10 cycle
        // were the newest before it), so the baseline server provides all of
        // them.
        val server = v(1, 0, 0)
        ServerFeature.entries.forEach { feature ->
            assertTrue(
                "server 1.0.0 must support ${feature.name}",
                ServerCapabilities.isSupported(server, feature),
            )
        }
    }
}
