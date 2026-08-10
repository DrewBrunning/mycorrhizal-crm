package com.mycorrhizal.crm.feature.tracking

import android.app.Notification
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import androidx.core.app.NotificationCompat
import com.mycorrhizal.crm.ui.R

/** Builds the user-facing notifications for the tracking/alert workers. */
object NotificationBuilder {

    fun reminder(context: Context, title: String, text: String): Notification =
        base(context, MycorrhizalNotificationChannels.REMINDERS)
            .setContentTitle(title)
            .setContentText(text)
            .setAutoCancel(true)
            .build()

    fun cadence(context: Context, title: String, text: String): Notification =
        base(context, MycorrhizalNotificationChannels.CADENCE)
            .setContentTitle(title)
            .setContentText(text)
            .setAutoCancel(true)
            .build()

    fun birthday(context: Context, title: String, text: String): Notification =
        base(context, MycorrhizalNotificationChannels.BIRTHDAYS)
            .setContentTitle(title)
            .setContentText(text)
            .setAutoCancel(true)
            .build()

    fun notify(context: Context, id: Int, notification: Notification) {
        val manager = context.getSystemService(NotificationManager::class.java)
        manager.notify(id, notification)
    }

    private fun base(context: Context, channel: String): NotificationCompat.Builder {
        // Deep link into the app on tap (opens the contacts list by default).
        val launch = context.packageManager.getLaunchIntentForPackage(context.packageName)
        val contentIntent = launch?.let {
            PendingIntent.getActivity(
                context,
                0,
                it,
                PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
            )
        }
        return NotificationCompat.Builder(context, channel)
            .setSmallIcon(R.drawable.ic_stat_mycorrhizal)
            .setContentIntent(contentIntent)
            .setPriority(NotificationCompat.PRIORITY_DEFAULT)
    }
}
