package com.mycorrhizal.crm

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Issue #1740: pins the release behaviour of FLAG_SECURE (masvs-l1.md P6) even
 * though the instrumented suite only ever runs the debug variant.
 */
class FlagSecurePolicyTest {
    @Test
    fun releaseBuildsSetFlagSecure() {
        assertTrue(shouldSetFlagSecure(isDebugBuild = false))
    }

    @Test
    fun debugBuildsAllowScreenshots() {
        assertFalse(shouldSetFlagSecure(isDebugBuild = true))
    }
}
