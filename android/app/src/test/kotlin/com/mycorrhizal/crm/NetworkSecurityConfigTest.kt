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

    private val rawProductionConfig =
        File("src/main/res/xml/network_security_config.xml").readText()

    // Strip XML comments: the file's own header comment mentions the
    // debug-overrides block it is telling you *not* to add, so a naive
    // substring match would pass a config that is only correct in prose.
    private val productionConfig =
        rawProductionConfig.replace(Regex("""<!--.*?-->""", RegexOption.DOT_MATCHES_ALL), "")

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

    // Regression guard for issue #961: the file's own header comment used to
    // claim self-signed certs were "handled via the KeyChain import flow
    // (installCustomCertificate)" — a flow that was never implemented
    // anywhere in this codebase, and that release builds trust system CAs
    // only makes untrue regardless. Pinned against the exact false claim's
    // phrasing (not a bare "KeyChain"/"installCustomCertificate" substring
    // check) since the corrected comment legitimately names both terms to
    // say no such path exists. Checked against the *raw* file (comments
    // included) since that is exactly the text this pins.
    @Test
    fun `production config comment does not claim an unimplemented KeyChain trust flow`() {
        assertFalse(
            "The comment must not claim self-signed certs are trusted via a KeyChain " +
                "import flow — no such flow exists, and this config's trust anchor is " +
                "system CAs only",
            rawProductionConfig.contains("handled via the KeyChain import flow"),
        )
        assertFalse(
            "The comment must not claim self-signed certs are handled/trusted at all — " +
                "they are not, in a release build",
            rawProductionConfig.contains("self-signed certs are handled"),
        )
    }
}
