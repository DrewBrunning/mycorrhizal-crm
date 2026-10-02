package com.mycorrhizal.crm.domain.about

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class AppBuildInfoTest {
    private fun info(applicationId: String) =
        AppBuildInfo("1.3.0", 1042, "debug", "obtainium", applicationId)

    @Test
    fun `the shipping applicationId has no suffix`() {
        assertNull(info("com.mycorrhizal.crm").applicationIdSuffix)
    }

    @Test
    fun `a side-by-side build reports its suffix`() {
        assertEquals(".localtest", info("com.mycorrhizal.crm.localtest").applicationIdSuffix)
    }

    @Test
    fun `an unrelated applicationId reports no suffix`() {
        assertNull(info("org.other.app").applicationIdSuffix)
    }
}
