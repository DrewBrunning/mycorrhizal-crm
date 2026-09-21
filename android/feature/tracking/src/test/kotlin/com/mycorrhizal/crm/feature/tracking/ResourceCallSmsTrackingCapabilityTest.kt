package com.mycorrhizal.crm.feature.tracking

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/**
 * Issue #1200: pins that the production [CallSmsTrackingCapability] reads the
 * merged `call_sms_tracking_available` bool (the library default here; the
 * `play` flavor overrides it to false in app resources). The play value itself
 * is asserted against the built APK in CI, since only the app module carries
 * the flavor override.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
class ResourceCallSmsTrackingCapabilityTest {

    @Test
    fun `reports the capture feature available under the library default`() {
        val context = ApplicationProvider.getApplicationContext<Context>()

        assertTrue(ResourceCallSmsTrackingCapability(context).isAvailable())
    }
}
