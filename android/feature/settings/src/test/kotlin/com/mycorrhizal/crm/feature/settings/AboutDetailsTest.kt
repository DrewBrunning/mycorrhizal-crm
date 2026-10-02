package com.mycorrhizal.crm.feature.settings

import com.mycorrhizal.crm.domain.about.AppBuildInfo
import com.mycorrhizal.crm.model.network.ServerHealth
import org.junit.Assert.assertEquals
import org.junit.Test

class AboutDetailsTest {
    private val base = AppBuildInfo("1.3.0", 1042, "release", "obtainium", "com.mycorrhizal.crm")

    @Test
    fun `without commit or build date only the always-present lines appear`() {
        assertEquals(
            "version: 1.3.0\ncode: 1042\nbuild: release / obtainium",
            AboutDetails.format(base, null),
        )
    }

    @Test
    fun `with commit and build date they are included in the web card's shape`() {
        val info = base.copy(commit = "abc1234", buildDate = "2026-10-02T10:00:00Z")
        assertEquals(
            "version: 1.3.0\ncode: 1042\nbuild: release / obtainium\ncommit: abc1234\nbuilt: 2026-10-02T10:00:00Z",
            AboutDetails.format(info, null),
        )
    }

    @Test
    fun `blank commit and build date are omitted`() {
        assertEquals(
            "version: 1.3.0\ncode: 1042\nbuild: release / obtainium",
            AboutDetails.format(base.copy(commit = " ", buildDate = ""), null),
        )
    }

    @Test
    fun `a debug build with a suffix is labelled as such`() {
        val info = base.copy(buildType = "debug", applicationId = "com.mycorrhizal.crm.localtest")
        assertEquals("debug / obtainium .localtest", AboutDetails.buildLabel(info))
    }

    @Test
    fun `a blank flavor is not rendered`() {
        assertEquals("release", AboutDetails.buildLabel(base.copy(flavor = "")))
    }

    @Test
    fun `server lines are appended when the lookup succeeded`() {
        val text = AboutDetails.format(
            base,
            ServerHealth(version = "1.3.0", commit = "def5678", buildDate = "2026-09-30T00:00:00Z"),
        )
        assertEquals(
            "version: 1.3.0\ncode: 1042\nbuild: release / obtainium\n" +
                "server: 1.3.0 (def5678)\nserver built: 2026-09-30T00:00:00Z",
            text,
        )
    }

    @Test
    fun `a server without a version contributes nothing`() {
        assertEquals(
            "version: 1.3.0\ncode: 1042\nbuild: release / obtainium",
            AboutDetails.format(base, ServerHealth(version = null, commit = "x")),
        )
    }

    @Test
    fun `versionWithCommit falls back to the bare version`() {
        assertEquals("1.3.0", AboutDetails.versionWithCommit("1.3.0", null))
        assertEquals("1.3.0", AboutDetails.versionWithCommit("1.3.0", ""))
        assertEquals("1.3.0 (abc)", AboutDetails.versionWithCommit("1.3.0", "abc"))
    }
}
