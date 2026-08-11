package com.mycorrhizal.crm.feature.tracking

import android.content.Context
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import java.util.concurrent.TimeUnit

/**
 * Enqueues the periodic Phase-4 workers (§6.4). Idempotent — safe to call
 * from BootReceiver, the app entry point, and the Settings toggles.
 */
object TrackingWorkerScheduler {

    const val UNIQUE_INTERACTION_SYNC = "interaction-sync"
    const val UNIQUE_REMINDER_CHECK = "reminder-check"
    const val UNIQUE_CADENCE_CHECK = "cadence-check"
    const val UNIQUE_BIRTHDAY_CHECK = "birthday-check"

    fun schedulePeriodic(context: Context) {
        val workManager = WorkManager.getInstance(context)

        // Sync pending interactions every 15 min (WorkManager minimum interval).
        val interactionSync = PeriodicWorkRequestBuilder<InteractionSyncWorker>(
            15,
            TimeUnit.MINUTES,
        ).build()
        workManager.enqueueUniquePeriodicWork(
            UNIQUE_INTERACTION_SYNC,
            ExistingPeriodicWorkPolicy.UPDATE,
            interactionSync,
        )

        // Notification checks (§6.4). Reminders hourly, cadences 4-hourly,
        // birthdays daily.
        val reminderCheck = PeriodicWorkRequestBuilder<ReminderNotificationWorker>(
            1,
            TimeUnit.HOURS,
        ).build()
        workManager.enqueueUniquePeriodicWork(
            UNIQUE_REMINDER_CHECK,
            ExistingPeriodicWorkPolicy.UPDATE,
            reminderCheck,
        )

        val cadenceCheck = PeriodicWorkRequestBuilder<CadenceCheckWorker>(
            4,
            TimeUnit.HOURS,
        ).build()
        workManager.enqueueUniquePeriodicWork(
            UNIQUE_CADENCE_CHECK,
            ExistingPeriodicWorkPolicy.UPDATE,
            cadenceCheck,
        )

        val birthdayCheck = PeriodicWorkRequestBuilder<BirthdayCheckWorker>(
            24,
            TimeUnit.HOURS,
        ).build()
        workManager.enqueueUniquePeriodicWork(
            UNIQUE_BIRTHDAY_CHECK,
            ExistingPeriodicWorkPolicy.UPDATE,
            birthdayCheck,
        )
    }
}
