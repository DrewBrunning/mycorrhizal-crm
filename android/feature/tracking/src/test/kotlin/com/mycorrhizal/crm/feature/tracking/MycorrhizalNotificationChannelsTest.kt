package com.mycorrhizal.crm.feature.tracking

import android.app.Application
import android.app.NotificationManager
import android.content.Context
import androidx.test.core.app.ApplicationProvider
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
class MycorrhizalNotificationChannelsTest {

    private val context = ApplicationProvider.getApplicationContext<Context>()
    private val manager = context.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager

    @Test
    fun `reminders channel is high importance and vibrates`() {
        MycorrhizalNotificationChannels.createAll(context)

        val channel = manager.getNotificationChannel(MycorrhizalNotificationChannels.REMINDERS)

        assertEquals(NotificationManager.IMPORTANCE_HIGH, channel.importance)
        assertTrue(channel.shouldVibrate())
    }

    @Test
    fun `cadence channel is default importance and vibrates`() {
        MycorrhizalNotificationChannels.createAll(context)

        val channel = manager.getNotificationChannel(MycorrhizalNotificationChannels.CADENCE)

        assertEquals(NotificationManager.IMPORTANCE_DEFAULT, channel.importance)
        assertTrue(channel.shouldVibrate())
    }

    @Test
    fun `birthdays channel shows a badge and does not vibrate`() {
        MycorrhizalNotificationChannels.createAll(context)

        val channel = manager.getNotificationChannel(MycorrhizalNotificationChannels.BIRTHDAYS)

        assertEquals(NotificationManager.IMPORTANCE_DEFAULT, channel.importance)
        assertFalse(channel.shouldVibrate())
        assertTrue(channel.canShowBadge())
    }
}
