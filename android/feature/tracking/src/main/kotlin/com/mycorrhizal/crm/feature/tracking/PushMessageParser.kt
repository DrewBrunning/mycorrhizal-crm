package com.mycorrhizal.crm.feature.tracking

/**
 * M5 §5a (issue #152): a parsed FCM `data` payload — the machine-readable half
 * of a push. The visible [title]/[body] may come from the message's
 * `notification` block (the server always sends both); [channel] selects the
 * local OS channel to post on; [idempotenceKey] is the `reminder_id:due_at`
 * pair that lets the push and the polling worker target one notification id;
 * [deepLink] routes a tap into the linked contact's page.
 */
data class PushMessage(
    val title: String,
    val body: String,
    val channel: String,
    val idempotenceKey: String?,
    val deepLink: String?,
)

/** Data-payload keys — kept in step with the server's `fcmReminderData` in
 *  backend/services/notification_service.go (no dynamic type-list endpoint,
 *  `/CLAUDE.md` frontend trap #4). */
private const val KEY_TYPE = "type"
private const val KEY_REMINDER_ID = "reminder_id"
private const val KEY_DUE_AT = "due_at"
private const val KEY_DEEP_LINK = "deep_link"
private const val KEY_CHANNEL = "channel"

/**
 * Pure parser (unit-testable without Android/Firebase): turns an FCM message's
 * data map plus its optional notification block into a [PushMessage].
 *
 * Returns null when there is nothing worth displaying — the FCM payload is not
 * one of the app's own push types. [title]/[body] fall back to the
 * `notification` block so a foreground-delivered message (where the system does
 * NOT auto-display and `onMessageReceived` must post itself) still shows.
 */
object PushMessageParser {

    fun parse(
        data: Map<String, String>,
        title: String? = null,
        body: String? = null,
    ): PushMessage? {
        val type = data[KEY_TYPE]
        val pushTitle = data["title"] ?: title
        val pushBody = data["body"] ?: body
        if (type == null && pushTitle == null && pushBody == null) return null

        val channel = when (data[KEY_CHANNEL]) {
            MycorrhizalNotificationChannels.CADENCE -> MycorrhizalNotificationChannels.CADENCE
            MycorrhizalNotificationChannels.BIRTHDAYS -> MycorrhizalNotificationChannels.BIRTHDAYS
            // Reminders are the only FCM push the server sends today; anything
            // without an explicit channel lands on the reminders channel.
            else -> MycorrhizalNotificationChannels.REMINDERS
        }

        val reminderId = data[KEY_REMINDER_ID]
        val dueAt = data[KEY_DUE_AT]
        val idempotenceKey = if (!reminderId.isNullOrBlank()) "$reminderId:$dueAt" else null

        return PushMessage(
            title = pushTitle.orEmpty(),
            body = pushBody.orEmpty(),
            channel = channel,
            idempotenceKey = idempotenceKey,
            deepLink = data[KEY_DEEP_LINK]?.takeIf { it.isNotBlank() },
        )
    }
}
