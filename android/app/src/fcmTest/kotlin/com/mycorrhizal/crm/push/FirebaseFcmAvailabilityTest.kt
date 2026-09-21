package com.mycorrhizal.crm.push

import android.app.Application
import android.content.Context
import androidx.test.core.app.ApplicationProvider
import com.google.firebase.FirebaseApp
import io.mockk.every
import io.mockk.mockk
import io.mockk.mockkStatic
import io.mockk.unmockkAll
import org.junit.After
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/**
 * Issue #1133: `FirebaseFcmAvailability` is the obtainium/play-only
 * implementation of the [com.mycorrhizal.crm.feature.tracking.FcmAvailability]
 * seam, so it exists only in `src/fcmTest` (which the FOSS variant's unit-test
 * source set does not include). Pins both outcomes of the one line its contract
 * reduces to: FCM is available iff at least one `FirebaseApp` is initialized.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
class FirebaseFcmAvailabilityTest {

    private val context = ApplicationProvider.getApplicationContext<Context>()
    private val availability = FirebaseFcmAvailability()

    @After
    fun tearDown() = unmockkAll()

    @Test
    fun `reports unavailable when no FirebaseApp is initialized`() {
        mockkStatic(FirebaseApp::class)
        every { FirebaseApp.getApps(any()) } returns emptyList()

        assertFalse(availability.isAvailable(context))
    }

    @Test
    fun `reports available when a FirebaseApp is initialized`() {
        mockkStatic(FirebaseApp::class)
        every { FirebaseApp.getApps(any()) } returns listOf(mockk(relaxed = true))

        assertTrue(availability.isAvailable(context))
    }
}
