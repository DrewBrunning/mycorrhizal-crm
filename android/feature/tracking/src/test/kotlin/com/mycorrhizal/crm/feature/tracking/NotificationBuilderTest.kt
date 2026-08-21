package com.mycorrhizal.crm.feature.tracking

import android.app.Application
import android.app.NotificationManager
import android.content.Context
import android.content.Intent
import android.content.pm.ActivityInfo
import android.content.pm.ResolveInfo
import androidx.core.app.NotificationCompat
import androidx.test.core.app.ApplicationProvider
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.Shadows
import org.robolectric.annotation.Config
import org.robolectric.shadows.ShadowPackageManager

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
class NotificationBuilderTest {

    private val context = ApplicationProvider.getApplicationContext<Context>()
    private val manager = context.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
    private val shadow = Shadows.shadowOf(manager)

    // Context.packageManager.getLaunchIntentForPackage(...) resolves via
    // queryIntentActivities against exactly this Intent shape (ACTION_MAIN +
    // CATEGORY_LAUNCHER + setPackage) -- feature/tracking's own test manifest
    // has no launcher activity, so this must be registered explicitly.
    private fun stubLaunchIntent() {
        val shadowPm = Shadows.shadowOf(context.packageManager) as ShadowPackageManager
        val resolveInfo = ResolveInfo().apply {
            activityInfo = ActivityInfo().apply {
                packageName = context.packageName
                name = "com.mycorrhizal.crm.MainActivity"
            }
        }
        val launcherIntent = Intent(Intent.ACTION_MAIN)
            .addCategory(Intent.CATEGORY_LAUNCHER)
            .setPackage(context.packageName)
        shadowPm.addResolveInfoForIntent(launcherIntent, resolveInfo)
    }

    @Test
    fun `reminder posts under the reminders channel with title and text`() {
        val notification = NotificationBuilder.reminder(context, "Call Jane", "Overdue by 2 days")
        NotificationBuilder.notify(context, 1, notification)

        val posted = shadow.getNotification(1)
        assertNotNull(posted)
        assertEquals(MycorrhizalNotificationChannels.REMINDERS, posted.channelId)
        assertEquals("Call Jane", NotificationCompat.getContentTitle(posted))
        assertEquals("Overdue by 2 days", NotificationCompat.getContentText(posted))
    }

    @Test
    fun `cadence posts under the cadence channel`() {
        val notification = NotificationBuilder.cadence(context, "Reach out", "It's been a while")
        NotificationBuilder.notify(context, 2, notification)

        assertEquals(MycorrhizalNotificationChannels.CADENCE, shadow.getNotification(2).channelId)
    }

    @Test
    fun `birthday posts under the birthdays channel`() {
        val notification = NotificationBuilder.birthday(context, "Birthday", "Jane turns 30 today")
        NotificationBuilder.notify(context, 3, notification)

        assertEquals(MycorrhizalNotificationChannels.BIRTHDAYS, shadow.getNotification(3).channelId)
    }

    @Test
    fun `a deep link is carried on the content intent`() {
        stubLaunchIntent()

        val notification = NotificationBuilder.reminder(context, "Call Jane", "Overdue", deepLink = "mycorrhizal://contacts/7")

        val savedIntent = Shadows.shadowOf(notification.contentIntent).savedIntent
        assertEquals("mycorrhizal://contacts/7", savedIntent.getStringExtra(NotificationBuilder.EXTRA_DEEP_LINK))
    }

    @Test
    fun `a blank deep link carries no extra`() {
        stubLaunchIntent()

        val notification = NotificationBuilder.reminder(context, "Call Jane", "Overdue", deepLink = null)

        val savedIntent = Shadows.shadowOf(notification.contentIntent).savedIntent
        assertNull(savedIntent.getStringExtra(NotificationBuilder.EXTRA_DEEP_LINK))
    }
}
