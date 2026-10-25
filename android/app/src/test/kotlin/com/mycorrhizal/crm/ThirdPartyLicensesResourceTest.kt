package com.mycorrhizal.crm

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import com.mikepenz.aboutlibraries.Libs
import com.mikepenz.aboutlibraries.util.withContext
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/**
 * Issue #1880: the AboutLibraries plugin must actually embed the generated
 * release metadata in the packaged app, not just run as a Gradle task. The
 * licenses screen parses this exact resource at runtime.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = android.app.Application::class)
class ThirdPartyLicensesResourceTest {

    @Test
    fun `the app embeds parseable license metadata with license texts`() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        val libs = Libs.Builder().withContext(context).build()

        assertFalse("no third-party libraries were embedded", libs.libraries.isEmpty())
        assertTrue(
            "no embedded library carries a license text",
            libs.libraries.any { library ->
                library.licenses.any { it.licenseContent?.isNotBlank() == true }
            }
        )
        assertTrue(
            "an embedded library has no declared license",
            libs.libraries.all { library -> library.licenses.isNotEmpty() }
        )
        assertTrue(
            "an embedded license has no name",
            libs.libraries.flatMap { it.licenses }.all { license -> license.name.isNotBlank() }
        )
    }

    @Test
    fun `the app packages the Go and font attribution`() {
        val context = ApplicationProvider.getApplicationContext<Context>()

        val notices = context.assets
            .open("thirdparty/THIRD_PARTY_NOTICES.md")
            .bufferedReader()
            .use { it.readText() }
        assertTrue(
            "the packaged notice does not attribute the Go standard library",
            notices.contains("Go standard library")
        )

        val sources = context.assets
            .open("thirdparty/THIRD_PARTY_SOURCES.md")
            .bufferedReader()
            .use { it.readText() }
        assertTrue(
            "the packaged sources notice does not carry the SIL OFL text",
            sources.contains("SIL OPEN FONT LICENSE Version 1.1")
        )
        assertTrue(
            "the packaged sources notice does not identify IBM Plex",
            sources.contains("IBM Plex Sans and IBM Plex Mono")
        )
        assertTrue(
            "the packaged sources notice does not identify EB Garamond",
            sources.contains("EB Garamond")
        )

        val ibmOfl = context.assets
            .open("thirdparty/IBMPlex-OFL.txt")
            .bufferedReader()
            .use { it.readText() }
        assertTrue(
            "the IBM Plex license asset is not the OFL text",
            ibmOfl.contains("Copyright © 2017 IBM Corp.")
        )
    }
}
