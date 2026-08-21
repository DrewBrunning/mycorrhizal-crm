package com.mycorrhizal.crm.feature.tracking

import android.app.Application
import android.telephony.TelephonyManager
import io.mockk.mockk
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.Robolectric
import org.robolectric.RobolectricTestRunner
import org.robolectric.Shadows
import org.robolectric.annotation.Config

/**
 * CallDetectionService has @Inject lateinit var fields (no Hilt test
 * application/harness exists in this repo -- see issue #320's Phase B
 * caveat on SmsReceiver), so it's built via Robolectric.buildService(...)
 * and the fields assigned directly, bypassing Hilt's injection entirely.
 * The call-idle -> show-overlay decision itself is tested through
 * handleCallStateChanged directly (extracted from the private
 * PhoneStateListener for exactly this reason -- Robolectric has no shadow
 * for a real TelephonyManager/PhoneStateListener callback).
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
class CallDetectionServiceTest {

    // .get() only -- deliberately not .create(). Robolectric.buildService(...)
    // already calls Service.attach(...) internally (real Context/Application
    // wiring, no Hilt involved), which is everything applicationContext/
    // getSystemService/startForeground need. .create() would additionally
    // invoke onCreate(), and since this class is @AndroidEntryPoint that's
    // Hilt-generated code that unconditionally requires a real
    // @HiltAndroidApp Application -- this repo has no Hilt test harness, so
    // that crashes with "Hilt service must be attached to an
    // @HiltAndroidApp Application" under a plain Application::class. Fields
    // are assigned directly below instead of via Hilt's injection.
    private fun buildService(): CallDetectionService {
        val service = Robolectric.buildService(CallDetectionService::class.java).get()
        service.contactRepository = mockk(relaxed = true)
        service.activityRepository = mockk(relaxed = true)
        return service
    }

    @Test
    fun `onStartCommand posts a foreground notification and returns START_STICKY`() {
        val service = buildService()

        val result = service.onStartCommand(null, 0, 0)

        val shadow = Shadows.shadowOf(service)
        assertNotNull(shadow.getLastForegroundNotification())
        assertEquals(CallDetectionService.NOTIFICATION_ID, shadow.getLastForegroundNotificationId())
        assertEquals(android.app.Service.START_STICKY, result)
    }

    @Test
    fun `onDestroy does not throw with no active overlay`() {
        val service = buildService()
        service.onStartCommand(null, 0, 0)

        service.onDestroy()
    }

    @Test
    fun `a call-idle state with the overlay permission granted creates and shows the overlay`() {
        val service = buildService()
        Shadows.shadowOf(service.application).grantPermissions(android.Manifest.permission.SYSTEM_ALERT_WINDOW)

        service.handleCallStateChanged(TelephonyManager.CALL_STATE_IDLE, "+15551234567")

        assertNotNull(service.quickCaptureOverlay)
    }

    @Test
    fun `a call-idle state with the overlay permission denied never creates the overlay`() {
        val service = buildService()
        Shadows.shadowOf(service.application).denyPermissions(android.Manifest.permission.SYSTEM_ALERT_WINDOW)

        service.handleCallStateChanged(TelephonyManager.CALL_STATE_IDLE, "+15551234567")

        assertNull(service.quickCaptureOverlay)
    }

    @Test
    fun `a non-idle call state never creates the overlay`() {
        val service = buildService()
        Shadows.shadowOf(service.application).grantPermissions(android.Manifest.permission.SYSTEM_ALERT_WINDOW)

        service.handleCallStateChanged(TelephonyManager.CALL_STATE_RINGING, "+15551234567")

        assertNull(service.quickCaptureOverlay)
    }
}
