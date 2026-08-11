package com.mycorrhizal.crm.feature.tracking

import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.Context
import android.os.Build
import com.mycorrhizal.crm.ui.R

/** The three user-facing notification channels (§6.8), distinct importance. */
object MycorrhizalNotificationChannels {
    const val REMINDERS = "reminders"
    const val CADENCE = "cadence"
    const val BIRTHDAYS = "birthdays"

    fun createAll(context: Context) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        val manager = context.getSystemService(NotificationManager::class.java)

        manager.createNotificationChannel(
            NotificationChannel(
                REMINDERS,
                context.getString(R.string.notification_channel_reminders_name),
                NotificationManager.IMPORTANCE_HIGH,
            ).apply {
                description = context.getString(R.string.notification_channel_reminders_description)
                enableVibration(true)
            },
        )

        manager.createNotificationChannel(
            NotificationChannel(
                CADENCE,
                context.getString(R.string.notification_channel_cadence_name),
                NotificationManager.IMPORTANCE_DEFAULT,
            ).apply {
                description = context.getString(R.string.notification_channel_cadence_description)
                enableVibration(true)
            },
        )

        manager.createNotificationChannel(
            NotificationChannel(
                BIRTHDAYS,
                context.getString(R.string.notification_channel_birthdays_name),
                NotificationManager.IMPORTANCE_DEFAULT,
            ).apply {
                description = context.getString(R.string.notification_channel_birthdays_description)
                enableVibration(false)
                setShowBadge(true)
            },
        )
    }
}
