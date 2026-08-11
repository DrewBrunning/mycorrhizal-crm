package com.mycorrhizal.crm.feature.tracking

import android.content.Context
import androidx.hilt.work.HiltWorker
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters
import com.mycorrhizal.crm.domain.repository.TrackingSettingsRepository
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.ui.R
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
        val fallbackTitle = applicationContext.getString(R.string.reminder_title_fallback)
        response.reminders.take(MAX_NOTIFICATIONS).forEachIndexed { i, reminder ->
            val text = reminder.message?.takeIf { it.isNotBlank() } ?: fallbackTitle
            NotificationBuilder.notify(
                applicationContext,
                AlertNotificationIds.REMINDERS + i,
                NotificationBuilder.reminder(applicationContext, fallbackTitle, text),
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
        val title = applicationContext.getString(R.string.notification_cadence_overdue_title)
        val fallbackName = applicationContext.getString(R.string.notification_contact_fallback)
        response.overdue.take(MAX_NOTIFICATIONS).forEachIndexed { i, cadence ->
            val name = cadence.contactName.takeIf { it.isNotBlank() } ?: fallbackName
            val text = applicationContext.getString(R.string.notification_cadence_overdue_body, name)
            NotificationBuilder.notify(
                applicationContext,
                AlertNotificationIds.CADENCE + i,
                NotificationBuilder.cadence(applicationContext, title, text),
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
        val title = applicationContext.getString(R.string.notification_birthday_upcoming_title)
        val fallbackName = applicationContext.getString(R.string.notification_someone_fallback)
        response.birthdays.take(MAX_NOTIFICATIONS).forEachIndexed { i, birthday ->
            val name = birthday.name.takeIf { it.isNotBlank() } ?: fallbackName
            val text = applicationContext.getString(R.string.notification_birthday_upcoming_body, name)
            NotificationBuilder.notify(
                applicationContext,
                AlertNotificationIds.BIRTHDAYS + i,
                NotificationBuilder.birthday(applicationContext, title, text),
            )
        }
        return Result.success()
    }

    companion object {
        private const val MAX_NOTIFICATIONS = 5
    }
}
