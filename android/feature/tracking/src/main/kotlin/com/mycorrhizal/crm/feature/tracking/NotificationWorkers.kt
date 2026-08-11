package com.mycorrhizal.crm.feature.tracking

import android.content.Context
import androidx.hilt.work.HiltWorker
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters
import com.mycorrhizal.crm.domain.repository.TrackingSettingsRepository
import com.mycorrhizal.crm.network.ApiClient
import dagger.assisted.Assisted
import dagger.assisted.AssistedInject

/** Notification ids — must stay distinct across the three channels. */
object AlertNotificationIds {
    const val REMINDERS = 2001
    const val CADENCE = 2002
    const val BIRTHDAYS = 2003
}

/**
 * Posts notifications for upcoming reminders (§6.4 ReminderNotificationWorker).
 * Respects the notifications opt-in and the POST_NOTIFICATIONS runtime
 * permission (a denial is a silent no-op).
 */
@HiltWorker
class ReminderNotificationWorker @AssistedInject constructor(
    @Assisted appContext: Context,
    @Assisted workerParams: WorkerParameters,
    private val apiClient: ApiClient,
    private val trackingSettings: TrackingSettingsRepository,
) : CoroutineWorker(appContext, workerParams) {

    override suspend fun doWork(): Result {
        if (!trackingSettings.notificationsEnabled()) return Result.success()
        val response = apiClient.listUpcomingReminders().getOrNull() ?: return Result.success()
        response.reminders.take(MAX_NOTIFICATIONS).forEachIndexed { i, reminder ->
            val text = reminder.message?.takeIf { it.isNotBlank() } ?: "Reminder"
            NotificationBuilder.notify(
                applicationContext,
                AlertNotificationIds.REMINDERS + i,
                NotificationBuilder.reminder(applicationContext, "Reminder", text),
            )
        }
        return Result.success()
    }

    companion object {
        private const val MAX_NOTIFICATIONS = 5
    }
}

/**
 * Posts notifications for overdue relationship cadences (§6.4
 * CadenceCheckWorker).
 */
@HiltWorker
class CadenceCheckWorker @AssistedInject constructor(
    @Assisted appContext: Context,
    @Assisted workerParams: WorkerParameters,
    private val apiClient: ApiClient,
    private val trackingSettings: TrackingSettingsRepository,
) : CoroutineWorker(appContext, workerParams) {

    override suspend fun doWork(): Result {
        if (!trackingSettings.notificationsEnabled()) return Result.success()
        val response = apiClient.listOverdueCadences().getOrNull() ?: return Result.success()
        response.overdue.take(MAX_NOTIFICATIONS).forEachIndexed { i, cadence ->
            val name = cadence.contactName.takeIf { it.isNotBlank() } ?: "a contact"
            NotificationBuilder.notify(
                applicationContext,
                AlertNotificationIds.CADENCE + i,
                NotificationBuilder.cadence(applicationContext, "Cadence overdue", "$name is overdue for contact."),
            )
        }
        return Result.success()
    }

    companion object {
        private const val MAX_NOTIFICATIONS = 5
    }
}

/**
 * Posts notifications for upcoming birthdays (§6.4 BirthdayCheckWorker).
 */
@HiltWorker
class BirthdayCheckWorker @AssistedInject constructor(
    @Assisted appContext: Context,
    @Assisted workerParams: WorkerParameters,
    private val apiClient: ApiClient,
    private val trackingSettings: TrackingSettingsRepository,
) : CoroutineWorker(appContext, workerParams) {

    override suspend fun doWork(): Result {
        if (!trackingSettings.notificationsEnabled()) return Result.success()
        val response = apiClient.listUpcomingBirthdays().getOrNull() ?: return Result.success()
        response.birthdays.take(MAX_NOTIFICATIONS).forEachIndexed { i, birthday ->
            val name = birthday.name.takeIf { it.isNotBlank() } ?: "Someone"
            NotificationBuilder.notify(
                applicationContext,
                AlertNotificationIds.BIRTHDAYS + i,
                NotificationBuilder.birthday(applicationContext, "Upcoming birthday", "$name's birthday is coming up."),
            )
        }
        return Result.success()
    }

    companion object {
        private const val MAX_NOTIFICATIONS = 5
    }
}
