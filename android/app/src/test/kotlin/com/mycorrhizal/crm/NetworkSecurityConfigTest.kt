package com.mycorrhizal.crm

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Guard test for the production network security config (issue #367, MASVS-L1
 * NETWORK-1 / NETWORK-3). A release build must forbid cleartext traffic and
 * trust only system CAs. The debug variant lives in the `debug` source set and
 * is never part of a release build, so it is deliberately not asserted here.
 *
 * Files resolve relative to the module working directory, the same pattern as
 * `core/ui`'s `LocalesConsistencyTest`.
 */
class NetworkSecurityConfigTest {

    // Strip XML comments: the file's own header comment mentions the
    // debug-overrides block it is telling you *not* to add, so a naive
    // substring match would pass a config that is only correct in prose.
    private val productionConfig =
        File("src/main/res/xml/network_security_config.xml").readText()
            .replace(Regex("""<!--.*?-->""", RegexOption.DOT_MATCHES_ALL), "")

    @Test
    fun `production config forbids cleartext traffic`() {
        assertTrue(
            "The production base-config must set cleartextTrafficPermitted=\"false\"",
            productionConfig.contains("cleartextTrafficPermitted=\"false\""),
        )
        assertFalse(
            "The production config must not permit cleartext for any domain",
            productionConfig.contains("cleartextTrafficPermitted=\"true\""),
        )
    }

    @Test
    fun `production config trusts only system CAs`() {
        assertTrue(
            "Trust anchors must be restricted to the system certificate store",
            productionConfig.contains("<certificates src=\"system\" />"),
        )
        assertFalse(
            "User-installed CAs must not be trusted in production",
            productionConfig.contains("<certificates src=\"user\" />"),
        )
    }

    @Test
    fun `production config has no debug-only trust overrides`() {
        assertFalse(
            "The production config must not carry a <debug-overrides> block",
            productionConfig.contains("<debug-overrides>"),
        )
    }
}
