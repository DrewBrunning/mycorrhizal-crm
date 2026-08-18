package com.mycorrhizal.crm.feature.tracking

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import androidx.core.content.ContextCompat
import javax.inject.Inject

/**
 * M5 §5a (issue #152): posts a parsed [PushMessage] as a notification on the
 * right local channel, with the deep-link content intent and the
 * reminder-idempotent notification id. This is the in-foreground path FCM
 * requires `onMessageReceived` to run itself (the system auto-displays only for
 * background messages); in the background it replaces the system's own display
 * with an identical one, which is idempotent.
 *
 * A denied POST_NOTIFICATIONS runtime permission is a silent no-op, matching
 * the polling workers' documented behavior (`/CLAUDE.md` tracking note) rather
 * than crashing on `manager.notify` (API 33+ throws SecurityException).
 */
class PushNotificationDispatcher @Inject constructor() {

    fun dispatch(context: Context, message: PushMessage) {
        if (!hasNotificationPermission(context)) return

        val id = notificationId(message)
        val notification = when (message.channel) {
            MycorrhizalNotificationChannels.CADENCE ->
                NotificationBuilder.cadence(context, message.title, message.body, message.deepLink)
            MycorrhizalNotificationChannels.BIRTHDAYS ->
                NotificationBuilder.birthday(context, message.title, message.body, message.deepLink)
            else ->
                NotificationBuilder.reminder(context, message.title, message.body, message.deepLink)
        }
        NotificationBuilder.notify(context, id, notification)
    }

    /**
     * The idempotence key drives the notification id: same (reminder_id, due_at)
     * → same id, so the polling worker's notify() replaces this push (or vice
     * versa) at the poll boundary instead of duplicating it. Messages without a
     * key (a server test push) fall back to a channel-scoped fixed id.
     */
    private fun notificationId(message: PushMessage): Int {
        val (reminderId, dueAt) = message.idempotenceKey?.split(":", limit = 2) ?: return 0x2E0001
        val id = reminderId.toIntOrNull() ?: return 0x2E0001
        return AlertNotificationIds.forReminder(id, dueAt)
    }

    private fun hasNotificationPermission(context: Context): Boolean =
        ContextCompat.checkSelfPermission(
            context,
            Manifest.permission.POST_NOTIFICATIONS,
        ) == PackageManager.PERMISSION_GRANTED
}
