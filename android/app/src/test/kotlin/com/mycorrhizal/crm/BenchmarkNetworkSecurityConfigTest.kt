package com.mycorrhizal.crm

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Guard test for the `benchmark` build type's network security config.
 *
 * The `:macrobenchmark` DashboardRenderBenchmark scenario (issue #263) logs
 * into the plain-HTTP `docker-compose.test.yml` backend at an adb-reversed
 * `127.0.0.1:7300`. The `benchmark` build type inherits the *release* config
 * (system CA only, cleartext forbidden), so without a build-type override the
 * app's login request is blocked by Android's cleartext policy and the
 * dashboard never renders. That failure is silent to the per-PR suite — the
 * macrobenchmark job is `continue-on-error` — so this test pins the override.
 *
 * The production config is asserted separately in
 * [NetworkSecurityConfigTest]; the debug source set is deliberately not
 * asserted there. Files resolve relative to the module working directory, the
 * same pattern as that test and `core/ui`'s `LocalesConsistencyTest`.
 */
class BenchmarkNetworkSecurityConfigTest {

    private val rawConfig =
        File("src/benchmark/res/xml/network_security_config.xml").readText()

    // Strip XML comments so the header comment (which names the loopback hosts
    // and the production rules) cannot make a broken config pass.
    private val config =
        rawConfig.replace(Regex("""<!--.*?-->""", RegexOption.DOT_MATCHES_ALL), "")

    @Test
    fun `benchmark config stays cleartext-forbidden by default`() {
        assertTrue(
            "The benchmark base-config must keep cleartextTrafficPermitted=\"false\"",
            config.contains("cleartextTrafficPermitted=\"false\""),
        )
        assertTrue(
            "Trust anchors must stay restricted to the system certificate store",
            config.contains("<certificates src=\"system\" />"),
        )
    }

    @Test
    fun `benchmark config permits cleartext only for the host loopback aliases`() {
        // Exactly one cleartext-permitting element: the single domain-config.
        // Anything more is a broad cleartext hole in a release-shaped build.
        assertEquals(
            "The benchmark config must have exactly one cleartext carve-out",
            1,
            Regex("""cleartextTrafficPermitted="true"""").findAll(config).count(),
        )
        assertTrue(
            "The benchmark config must carry the loopback cleartext carve-out",
            config.contains("<domain-config cleartextTrafficPermitted=\"true\">"),
        )
        for (host in listOf("10.0.2.2", "127.0.0.1", "localhost")) {
            assertTrue(
                "The benchmark config must permit cleartext to $host",
                config.contains(">$host</domain>"),
            )
        }
    }
}
