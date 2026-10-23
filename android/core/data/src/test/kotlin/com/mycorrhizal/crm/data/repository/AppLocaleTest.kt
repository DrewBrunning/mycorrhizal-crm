package com.mycorrhizal.crm.data.repository

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Issue #1921: below API 33 the in-app language override is the only source of
 * the UI locale. From API 33 the OS per-app locale exists and, when explicitly
 * set, must win over the local cache — otherwise a selection made in system
 * Settings would be overwritten by the app on every activity start.
 */
class AppLocaleTest {

    @Test
    fun `the in-app override always applies below API 33`() {
        assertTrue(AppLocale.shouldApplyInAppOverride(sdkInt = 26, systemHasLocale = false))
        assertTrue(AppLocale.shouldApplyInAppOverride(sdkInt = 32, systemHasLocale = false))
        // API 32 cannot have an OS per-app locale, but an unexpected true must not
        // change the answer: the local override is still the only source.
        assertTrue(AppLocale.shouldApplyInAppOverride(sdkInt = 32, systemHasLocale = true))
    }

    @Test
    fun `an explicit OS locale wins from API 33`() {
        assertFalse(AppLocale.shouldApplyInAppOverride(sdkInt = 33, systemHasLocale = true))
        assertFalse(AppLocale.shouldApplyInAppOverride(sdkInt = 37, systemHasLocale = true))
    }

    @Test
    fun `the local override is the fallback when API 33 has no OS locale`() {
        assertTrue(AppLocale.shouldApplyInAppOverride(sdkInt = 33, systemHasLocale = false))
        assertTrue(AppLocale.shouldApplyInAppOverride(sdkInt = 37, systemHasLocale = false))
    }
}
