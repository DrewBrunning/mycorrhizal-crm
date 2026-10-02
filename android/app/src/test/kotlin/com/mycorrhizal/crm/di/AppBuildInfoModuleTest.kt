package com.mycorrhizal.crm.di

import com.mycorrhizal.crm.BuildConfig
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class AppBuildInfoModuleTest {
    @Test
    fun `blank commit and build date become null`() {
        val info = appBuildInfo("1.0", 7, "release", "foss", "com.mycorrhizal.crm", commit = "", buildDate = "  ")
        assertNull(info.commit)
        assertNull(info.buildDate)
    }

    @Test
    fun `stamped commit and build date are kept`() {
        val info = appBuildInfo("1.0", 7, "release", "foss", "com.mycorrhizal.crm", "abc1234", "2026-10-02T00:00:00Z")
        assertEquals("abc1234", info.commit)
        assertEquals("2026-10-02T00:00:00Z", info.buildDate)
    }

    @Test
    fun `the provider reflects this build's BuildConfig`() {
        val info = AppBuildInfoModule.provideAppBuildInfo()
        assertEquals(BuildConfig.VERSION_NAME, info.versionName)
        assertEquals(BuildConfig.VERSION_CODE, info.versionCode)
        assertEquals(BuildConfig.BUILD_TYPE, info.buildType)
        assertEquals(BuildConfig.APPLICATION_ID, info.applicationId)
    }
}
