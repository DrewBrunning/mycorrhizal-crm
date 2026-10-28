package com.mycorrhizal.crm.feature.tracking

import android.app.Application
import android.content.ContentProvider
import android.content.ContentValues
import android.database.Cursor
import android.database.MatrixCursor
import android.net.Uri
import android.provider.CallLog
import android.telephony.TelephonyManager
import com.mycorrhizal.crm.model.network.ContactSummary
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestCoroutineScheduler
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.Robolectric
import org.robolectric.RobolectricTestRunner
import org.robolectric.Shadows
import org.robolectric.annotation.Config
import org.robolectric.shadows.ShadowContentResolver

/**
 * CallDetectionService has @Inject lateinit var fields (no Hilt test
 * application/harness exists in this repo -- see issue #320's Phase B
 * caveat on SmsReceiver), so it's built via Robolectric.buildService(...)
 * and the fields assigned directly, bypassing Hilt's injection entirely.
 * The call-idle -> show-overlay decision itself is tested through
 * handleCallStateChanged directly (extracted from the private
 * PhoneStateListener for exactly this reason -- Robolectric has no shadow
 * for a real TelephonyManager/PhoneStateListener callback).
 *
 * Issue #1029: the overlay is suppressed for a caller whose number matches no
 * cached contact unless the user opted into unknown numbers. The lookup is
 * suspend, so handleCallStateChanged launches it on an injectable
 * [CallDetectionService.serviceDispatcher]; these tests drive that with a
 * StandardTestDispatcher and advance its scheduler.
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
    private fun buildService(
        contact: ContactSummary? = null,
        includeUnknown: Boolean = false,
        lookupThrows: Boolean = false
    ): Pair<CallDetectionService, TestCoroutineScheduler> {
        val service = Robolectric.buildService(CallDetectionService::class.java).get()
        val dispatcher = StandardTestDispatcher()
        service.serviceDispatcher = dispatcher
        service.contactRepository = mockk {
            if (lookupThrows) {
                coEvery { findByPhone(any()) } throws RuntimeException("db error")
            } else {
                coEvery { findByPhone(any()) } returns contact
            }
        }
        service.activityRepository = mockk(relaxed = true)
        service.trackingSettings = mockk {
            coEvery { includeUnknownNumbers() } returns includeUnknown
        }
        return service to dispatcher.scheduler
    }

    @Test
    fun `onStartCommand posts a foreground notification and returns START_STICKY`() {
        val (service, _) = buildService()

        val result = service.onStartCommand(null, 0, 0)

        val shadow = Shadows.shadowOf(service)
        assertNotNull(shadow.getLastForegroundNotification())
        assertEquals(CallDetectionService.NOTIFICATION_ID, shadow.getLastForegroundNotificationId())
        assertEquals(android.app.Service.START_STICKY, result)
    }

    @Test
    fun `onDestroy dismisses and drops an active overlay`() {
        val (service, scheduler) = buildService(contact = ContactSummary(id = 9))
        Shadows.shadowOf(
            service.application
        ).grantPermissions(android.Manifest.permission.SYSTEM_ALERT_WINDOW)
        service.onStartCommand(null, 0, 0)
        service.handleCallStateChanged(TelephonyManager.CALL_STATE_IDLE, "+15551234567")
        scheduler.advanceUntilIdle()
        assertNotNull(service.quickCaptureOverlay)

        service.onDestroy()

        assertNull(service.quickCaptureOverlay)
    }

    @Test
    fun `a call-idle state from a known contact shows the overlay`() {
        val (service, scheduler) = buildService(contact = ContactSummary(id = 9))
        Shadows.shadowOf(
            service.application
        ).grantPermissions(android.Manifest.permission.SYSTEM_ALERT_WINDOW)

        service.handleCallStateChanged(TelephonyManager.CALL_STATE_IDLE, "+15551234567")
        scheduler.advanceUntilIdle()

        assertNotNull(service.quickCaptureOverlay)
    }

    @Test
    fun `an unknown caller is suppressed by default (issue #1029)`() {
        val (service, scheduler) = buildService(contact = null, includeUnknown = false)
        Shadows.shadowOf(
            service.application
        ).grantPermissions(android.Manifest.permission.SYSTEM_ALERT_WINDOW)

        service.handleCallStateChanged(TelephonyManager.CALL_STATE_IDLE, "+15559876543")
        scheduler.advanceUntilIdle()

        assertNull(service.quickCaptureOverlay)
    }

    @Test
    fun `an unknown caller shows the overlay when the include-unknown opt-in is on`() {
        val (service, scheduler) = buildService(contact = null, includeUnknown = true)
        Shadows.shadowOf(
            service.application
        ).grantPermissions(android.Manifest.permission.SYSTEM_ALERT_WINDOW)

        service.handleCallStateChanged(TelephonyManager.CALL_STATE_IDLE, "+15559876543")
        scheduler.advanceUntilIdle()

        assertNotNull(service.quickCaptureOverlay)
    }

    @Test
    fun `a call-idle state with the overlay permission denied never creates the overlay`() {
        val (service, scheduler) = buildService(contact = ContactSummary(id = 9))
        Shadows.shadowOf(
            service.application
        ).denyPermissions(android.Manifest.permission.SYSTEM_ALERT_WINDOW)

        service.handleCallStateChanged(TelephonyManager.CALL_STATE_IDLE, "+15551234567")
        scheduler.advanceUntilIdle()

        assertNull(service.quickCaptureOverlay)
    }

    @Test
    fun `a telephony callback resolves the just-ended call number before deciding`() {
        val (service, scheduler) = buildService(contact = ContactSummary(id = 9))
        service.recentCallNumber = { "+15551234567" }
        Shadows.shadowOf(
            service.application
        ).grantPermissions(android.Manifest.permission.SYSTEM_ALERT_WINDOW)

        service.handleTelephonyCallStateChanged(TelephonyManager.CALL_STATE_IDLE)
        scheduler.advanceUntilIdle()

        assertNotNull(service.quickCaptureOverlay)
    }

    @Test
    fun `a non-idle call state never creates the overlay`() {
        val (service, scheduler) = buildService(contact = ContactSummary(id = 9))
        Shadows.shadowOf(
            service.application
        ).grantPermissions(android.Manifest.permission.SYSTEM_ALERT_WINDOW)

        service.handleCallStateChanged(TelephonyManager.CALL_STATE_RINGING, "+15551234567")
        scheduler.advanceUntilIdle()

        assertNull(service.quickCaptureOverlay)
    }

    @Test
    fun `recentCallNumber reads the newest call-log entry for the API 31+ callback`() {
        val (service, _) = buildService()
        val provider = object : ContentProvider() {
            override fun onCreate(): Boolean = true
            override fun query(
                uri: Uri,
                projection: Array<out String>?,
                selection: String?,
                selectionArgs: Array<out String>?,
                sortOrder: String?
            ): Cursor = MatrixCursor(
                arrayOf(
                    CallLog.Calls.NUMBER,
                    CallLog.Calls.TYPE,
                    CallLog.Calls.DATE,
                    CallLog.Calls.DURATION,
                    CallLog.Calls.CACHED_NAME
                )
            ).apply {
                addRow(arrayOf<Any?>("+15551234567", CallLog.Calls.INCOMING_TYPE, 1L, 1L, null))
            }
            override fun getType(uri: Uri): String? = null
            override fun insert(uri: Uri, values: ContentValues?): Uri? = null
            override fun delete(
                uri: Uri,
                selection: String?,
                selectionArgs: Array<out String>?
            ): Int = 0
            override fun update(
                uri: Uri,
                values: ContentValues?,
                selection: String?,
                selectionArgs: Array<out String>?
            ): Int = 0
        }
        ShadowContentResolver.registerProviderInternal(CallLog.AUTHORITY, provider)

        assertEquals("+15551234567", service.recentCallNumber())
    }

    @Test
    fun `below API 31 the legacy listener is registered and receives call states`() {
        val (service, _) = buildService()
        service.sdkInt = 30

        val result = service.onStartCommand(null, 0, 0)
        val telephonyManager = service.getSystemService(TelephonyManager::class.java)
        Shadows.shadowOf(telephonyManager).setCallState(
            TelephonyManager.CALL_STATE_RINGING,
            "+15551234567"
        )
        service.onDestroy()

        assertEquals(android.app.Service.START_STICKY, result)
    }

    @Test
    fun `the API 31+ telephony callback drives the overlay on call end`() {
        val (service, scheduler) = buildService(contact = ContactSummary(id = 9))
        service.recentCallNumber = { "+15551234567" }
        Shadows.shadowOf(
            service.application
        ).grantPermissions(android.Manifest.permission.SYSTEM_ALERT_WINDOW)

        service.onStartCommand(null, 0, 0)
        val telephonyManager = service.getSystemService(TelephonyManager::class.java)
        Shadows.shadowOf(telephonyManager).setCallState(
            TelephonyManager.CALL_STATE_RINGING,
            "+15551234567"
        )
        scheduler.advanceUntilIdle()
        assertNull("a ringing call must not show the overlay", service.quickCaptureOverlay)

        Shadows.shadowOf(telephonyManager).setCallState(
            TelephonyManager.CALL_STATE_IDLE,
            "+15551234567"
        )
        scheduler.advanceUntilIdle()
        assertNotNull(service.quickCaptureOverlay)

        service.onDestroy()
    }

    // --- the decision matrix, directly on shouldShowOverlay ---------------

    @Test
    fun `shouldShowOverlay is true for a known contact`() = runTest {
        val (service, _) = buildService(contact = ContactSummary(id = 9))

        assertTrue(service.shouldShowOverlay("+15551234567"))
    }

    @Test
    fun `shouldShowOverlay is false for an unknown number by default`() = runTest {
        val (service, _) = buildService(contact = null, includeUnknown = false)

        assertFalse(service.shouldShowOverlay("+15559876543"))
    }

    @Test
    fun `shouldShowOverlay is true for an unknown number with the opt-in`() = runTest {
        val (service, _) = buildService(contact = null, includeUnknown = true)

        assertTrue(service.shouldShowOverlay("+15559876543"))
    }

    @Test
    fun `shouldShowOverlay treats a blank number as unknown`() = runTest {
        val (suppressed, _) = buildService(contact = null, includeUnknown = false)
        assertFalse(suppressed.shouldShowOverlay(null))
        assertFalse(suppressed.shouldShowOverlay(""))

        val (included, _) = buildService(contact = null, includeUnknown = true)
        assertTrue(included.shouldShowOverlay(null))
    }

    @Test
    fun `shouldShowOverlay treats a lookup failure as unknown`() = runTest {
        val (suppressed, _) = buildService(lookupThrows = true, includeUnknown = false)
        assertFalse(suppressed.shouldShowOverlay("+15551234567"))

        val (included, _) = buildService(lookupThrows = true, includeUnknown = true)
        assertTrue(included.shouldShowOverlay("+15551234567"))
    }
}
