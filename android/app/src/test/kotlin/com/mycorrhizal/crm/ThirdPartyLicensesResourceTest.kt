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
    }
}
