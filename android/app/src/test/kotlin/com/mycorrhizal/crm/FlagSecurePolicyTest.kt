package com.mycorrhizal.crm

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Issue #1741: FLAG_SECURE follows the opt-in "Block screenshots and screen
 * recording" setting exactly, in every build type (masvs-l1.md P6).
 */
class FlagSecurePolicyTest {
    @Test
    fun settingOnSetsFlagSecure() {
        assertTrue(shouldSetFlagSecure(blockScreenshotsSetting = true))
    }

    @Test
    fun settingOffAllowsScreenshots() {
        assertFalse(shouldSetFlagSecure(blockScreenshotsSetting = false))
    }
}
