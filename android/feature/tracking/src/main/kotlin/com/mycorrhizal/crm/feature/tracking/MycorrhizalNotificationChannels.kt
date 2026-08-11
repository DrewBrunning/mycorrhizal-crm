package com.mycorrhizal.crm.feature.tracking

import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.Context
import android.os.Build

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
                "Reminders",
                NotificationManager.IMPORTANCE_HIGH,
            ).apply {
                description = "Contact reminders and follow-ups"
                enableVibration(true)
            },
        )

        manager.createNotificationChannel(
            NotificationChannel(
                CADENCE,
                "Cadence alerts",
                NotificationManager.IMPORTANCE_DEFAULT,
            ).apply {
                description = "Overdue relationship cadence warnings"
                enableVibration(true)
            },
        )

        manager.createNotificationChannel(
            NotificationChannel(
                BIRTHDAYS,
                "Birthdays",
                NotificationManager.IMPORTANCE_DEFAULT,
            ).apply {
                description = "Upcoming contact birthdays"
                enableVibration(false)
                setShowBadge(true)
            },
        )
    }
}
