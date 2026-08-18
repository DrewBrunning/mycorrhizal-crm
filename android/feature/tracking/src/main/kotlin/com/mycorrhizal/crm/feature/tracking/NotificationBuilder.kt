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

    /**
     * The extra carrying a notification's deep link (`mycorrhizal://…`) on the
     * content intent. MainActivity reads it in onCreate/onNewIntent and routes
     * the NavHost to the linked screen (M5 §6.6).
     */
    const val EXTRA_DEEP_LINK = "mycorrhizal.extra.deep_link"

    fun reminder(context: Context, title: String, text: String, deepLink: String? = null): Notification =
        base(context, MycorrhizalNotificationChannels.REMINDERS, deepLink)
            .setContentTitle(title)
            .setContentText(text)
            .setAutoCancel(true)
            .build()

    fun cadence(context: Context, title: String, text: String, deepLink: String? = null): Notification =
        base(context, MycorrhizalNotificationChannels.CADENCE, deepLink)
            .setContentTitle(title)
            .setContentText(text)
            .setAutoCancel(true)
            .build()

    fun birthday(context: Context, title: String, text: String, deepLink: String? = null): Notification =
        base(context, MycorrhizalNotificationChannels.BIRTHDAYS, deepLink)
            .setContentTitle(title)
            .setContentText(text)
            .setAutoCancel(true)
            .build()

    fun notify(context: Context, id: Int, notification: Notification) {
        val manager = context.getSystemService(NotificationManager::class.java)
        manager.notify(id, notification)
    }

    private fun base(context: Context, channel: String, deepLink: String?): NotificationCompat.Builder {
        // Deep link into the app on tap. Without a deep link this opens the
        // contacts list by default; with one, the extra is carried so
        // MainActivity can route the NavHost to the linked contact.
        val launch = context.packageManager.getLaunchIntentForPackage(context.packageName)
        val contentIntent = launch?.let {
            if (!deepLink.isNullOrBlank()) it.putExtra(EXTRA_DEEP_LINK, deepLink)
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
