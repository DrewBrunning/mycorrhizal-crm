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

    /**
     * M5 §6.8 (issue #152): the notification id for one reminder occurrence,
     * derived from the (reminder_id, due_at) idempotence key. The FCM push path
     * and the WorkManager polling worker both target the SAME id, so when both
     * fire for the same reminder around the poll boundary the second `notify`
     * replaces the first instead of duplicating it. Hashed into the
     * `0x2D0000..0x2DFFFF` band — far from the static channel ids above.
     *
     * [dueAt] is normalized to epoch seconds before hashing because the two
     * paths see the due time in different string forms: the FCM `data.due_at`
     * is canonical UTC (no sub-seconds), while the polling worker's `remind_at`
     * is Go's RFC3339Nano with nanoseconds and the server's local offset. Both
     * must collapse to one key or the double-notify survives.
     */
    fun forReminder(reminderId: Int, dueAt: String?): Int {
        val epoch = dueAt?.let { raw ->
            runCatching { java.time.Instant.parse(raw).epochSecond }.getOrNull()
        }
        val key = "$reminderId:${epoch ?: dueAt.orEmpty()}"
        return REMINDER_IDEMPTOTENCE_BASE or (key.hashCode() and 0xFFFF)
    }

    private const val REMINDER_IDEMPTOTENCE_BASE = 0x2D0000
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
        response.reminders.take(MAX_NOTIFICATIONS).forEach { reminder ->
            val text = reminder.message?.takeIf { it.isNotBlank() } ?: fallbackTitle
            // Idempotence key (issue #152): the same (reminder_id, due_at) the
            // FCM push uses, so both paths replace rather than duplicate the
            // notification at the poll boundary.
            val id = AlertNotificationIds.forReminder(reminder.id, reminder.remindAt)
            NotificationBuilder.notify(
                applicationContext,
                id,
                NotificationBuilder.reminder(
                    applicationContext,
                    fallbackTitle,
                    text,
                    deepLink = reminder.contactId?.let { "mycorrhizal://contacts/$it" },
                ),
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
