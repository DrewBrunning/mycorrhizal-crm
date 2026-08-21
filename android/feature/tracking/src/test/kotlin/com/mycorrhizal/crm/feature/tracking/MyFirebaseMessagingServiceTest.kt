package com.mycorrhizal.crm.feature.tracking

import android.app.Application
import android.app.NotificationManager
import android.content.Context
import androidx.test.core.app.ApplicationProvider
import com.google.firebase.messaging.RemoteMessage
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.Robolectric
import org.robolectric.RobolectricTestRunner
import org.robolectric.Shadows
import org.robolectric.annotation.Config

/**
 * Like CallDetectionService, MyFirebaseMessagingService has @Inject lateinit
 * var fields with no Hilt test harness in this repo -- built via
 * Robolectric.buildService(...) and the fields assigned directly. dispatcher
 * is a real PushNotificationDispatcher (a trivial no-arg class already
 * covered end-to-end by app/src/test/.../PushNotificationDispatcherTest.kt)
 * so this test exercises the wiring, not re-testing dispatch()'s internals.
 *
 * RemoteMessage's public Builder API gives no path to a non-null
 * `.notification` block (that's populated internally from a raw GCM
 * Bundle/intent) -- only the data-only push path is coverable here, which
 * matches the class's own doc comment: in-foreground delivery is exactly the
 * case where the system does NOT auto-populate a notification and
 * onMessageReceived must post one itself.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
class MyFirebaseMessagingServiceTest {

    private val context = ApplicationProvider.getApplicationContext<Context>()
    private val manager = context.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
    private val shadow = Shadows.shadowOf(manager)

    // .get() only, not .create() -- see CallDetectionServiceTest's buildService
    // for why: .create() would invoke Hilt-generated onCreate(), which needs a
    // real @HiltAndroidApp Application this repo's tests don't have. Fields
    // are assigned directly below instead of via Hilt's injection.
    private fun buildService(): MyFirebaseMessagingService {
        val service = Robolectric.buildService(MyFirebaseMessagingService::class.java).get()
        service.dispatcher = PushNotificationDispatcher()
        service.deviceRegistration = mockk(relaxed = true)
        return service
    }

    private fun dataOnlyMessage(data: Map<String, String>): RemoteMessage =
        RemoteMessage.Builder("dummy@fcm.googleapis.com").apply {
            data.forEach { (k, v) -> addData(k, v) }
        }.build()

    @Test
    fun `a data-only reminder push posts a notification`() {
        val service = buildService()
        Shadows.shadowOf(context as Application).grantPermissions(android.Manifest.permission.POST_NOTIFICATIONS)
        val message = dataOnlyMessage(
            mapOf("title" to "Call Jane", "body" to "Overdue by 2 days", "reminder_id" to "42", "due_at" to "2026-08-18T14:14:28Z"),
        )

        service.onMessageReceived(message)

        val expectedId = AlertNotificationIds.forReminder(42, "2026-08-18T14:14:28Z")
        assertTrue(shadow.getNotification(expectedId) != null)
    }

    @Test
    fun `an unparseable data payload posts nothing`() {
        val service = buildService()
        Shadows.shadowOf(context as Application).grantPermissions(android.Manifest.permission.POST_NOTIFICATIONS)

        service.onMessageReceived(dataOnlyMessage(emptyMap()))

        assertTrue(shadow.allNotifications.isEmpty())
    }

    @Test
    fun `onNewToken re-registers the device with the new token`() {
        val service = buildService()
        coEvery { service.deviceRegistration.register(tokenOverride = "new-token") } returns Result.success(Unit)

        service.onNewToken("new-token")

        coVerify(timeout = 1000) { service.deviceRegistration.register(tokenOverride = "new-token") }
    }
}
