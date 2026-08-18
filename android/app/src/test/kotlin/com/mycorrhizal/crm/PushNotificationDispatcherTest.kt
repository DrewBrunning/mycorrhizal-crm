package com.mycorrhizal.crm

import android.Manifest
import android.app.Application
import android.app.NotificationManager
import android.content.Context
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.feature.tracking.AlertNotificationIds
import com.mycorrhizal.crm.feature.tracking.MycorrhizalNotificationChannels
import com.mycorrhizal.crm.feature.tracking.NotificationBuilder
import com.mycorrhizal.crm.feature.tracking.PushMessage
import com.mycorrhizal.crm.feature.tracking.PushNotificationDispatcher
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.Shadows
import org.robolectric.annotation.Config

/**
 * M5 §6.6 (issue #152): the data-payload → deep-link PendingIntent handler.
 * Lives in :app (not :feature:tracking) because it needs the real launcher
 * activity from the app manifest — `getLaunchIntentForPackage` resolves
 * MainActivity here, so the notification's content intent actually carries the
 * deep link. Pins the channel constants, the idempotent notification id, and
 * the poll-boundary replacement.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
class PushNotificationDispatcherTest {

    private val context = ApplicationProvider.getApplicationContext<Context>()
    private val manager = context.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
    private val shadow = Shadows.shadowOf(manager)

    @Before
    fun setUp() {
        Shadows.shadowOf(context as Application).grantPermissions(Manifest.permission.POST_NOTIFICATIONS)
    }

    @Test
    fun `dispatch posts on the reminders channel with the deep link and idempotent id`() {
        PushNotificationDispatcher().dispatch(
            context,
            PushMessage(
                title = "Reminder",
                body = "Jane: Call about dinner",
                channel = MycorrhizalNotificationChannels.REMINDERS,
                idempotenceKey = "42:2026-08-18T06:00:00Z",
                deepLink = "mycorrhizal://contacts/7",
            ),
        )

        val notification = shadow.allNotifications.single()
        // The idempotent notification id is what collapses the FCM/poll boundary:
        // assert the push landed under the (reminder_id, due_at)-derived id.
        val expectedId = AlertNotificationIds.forReminder(42, "2026-08-18T06:00:00Z")
        assertEquals(notification, shadow.getNotification(expectedId))
        assertEquals(MycorrhizalNotificationChannels.REMINDERS, notification.channelId)
        val savedIntent = Shadows.shadowOf(notification.contentIntent).savedIntent
        assertEquals("mycorrhizal://contacts/7", savedIntent.getStringExtra(NotificationBuilder.EXTRA_DEEP_LINK))
    }

    @Test
    fun `a cadence-channel push posts on the cadence channel`() {
        PushNotificationDispatcher().dispatch(
            context,
            PushMessage(
                title = "Cadence",
                body = "Reach out to Jane",
                channel = MycorrhizalNotificationChannels.CADENCE,
                idempotenceKey = null,
                deepLink = null,
            ),
        )

        val notification = shadow.allNotifications.single()
        assertEquals(MycorrhizalNotificationChannels.CADENCE, notification.channelId)
    }

    @Test
    fun `same idempotence key twice replaces instead of duplicating`() {
        val dispatcher = PushNotificationDispatcher()
        val message = PushMessage(
            title = "Reminder",
            body = "Jane: Call about dinner",
            channel = MycorrhizalNotificationChannels.REMINDERS,
            idempotenceKey = "42:2026-08-18T06:00:00Z",
            deepLink = "mycorrhizal://contacts/7",
        )
        dispatcher.dispatch(context, message)
        dispatcher.dispatch(context, message)

        // One notification remains: the second notify() replaced the first.
        assertEquals(1, shadow.allNotifications.size)
    }

    @Test
    fun `a denied POST_NOTIFICATIONS permission is a silent no-op`() {
        Shadows.shadowOf(context as Application).denyPermissions(Manifest.permission.POST_NOTIFICATIONS)

        PushNotificationDispatcher().dispatch(
            context,
            PushMessage(
                title = "Reminder",
                body = "Jane: Call about dinner",
                channel = MycorrhizalNotificationChannels.REMINDERS,
                idempotenceKey = "42:2026-08-18T06:00:00Z",
                deepLink = "mycorrhizal://contacts/7",
            ),
        )

        assertTrue(shadow.allNotifications.isEmpty())
    }
}
