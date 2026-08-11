package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * A reminder. gorm.Model identity keys serialize in PascalCase. The Reminder
 * model doubles as the request body for create/update (required fields:
 * message, remind_at, recurrence, contact_id). `recurrence` is a closed enum:
 * once, weekly, monthly, quarterly, six-months, yearly.
 */
@JsonClass(generateAdapter = true)
data class Reminder(
    @Json(name = "ID") val id: Int = 0,
    @Json(name = "CreatedAt") val createdAt: String? = null,
    @Json(name = "UpdatedAt") val updatedAt: String? = null,
    val message: String? = null,
    @Json(name = "by_mail") val byMail: Boolean? = null,
    @Json(name = "remind_at") val remindAt: String? = null,
    val recurrence: String? = null,
    @Json(name = "reoccur_from_completion") val reoccurFromCompletion: Boolean? = null,
    val completed: Boolean = false,
    @Json(name = "email_sent") val emailSent: Boolean = false,
    @Json(name = "contact_id") val contactId: Int? = null,
    @Json(name = "life_event_id") val lifeEventId: String? = null,
    val contact: ContactFlat? = null,
)

/** GET /contacts/{id}/reminders — wrapped `{ reminders }` array. */
@JsonClass(generateAdapter = true)
data class ContactRemindersResponse(
    val reminders: List<Reminder> = emptyList(),
)

/** POST /contacts/{id}/reminders — wrapped `{ message, reminder }`. */
@JsonClass(generateAdapter = true)
data class CreateReminderResponse(
    val message: String? = null,
    val reminder: Reminder? = null,
)

/**
 * POST /reminders/{id}/complete — recurring reminders reschedule and return
 * `{ message, reminder }`; once reminders are soft-deleted and return only a
 * message.
 */
@JsonClass(generateAdapter = true)
data class ReminderCompleteResponse(
    val message: String? = null,
    val reminder: Reminder? = null,
)

/** Closed recurrence set, mirroring the backend's `recurrence` enum. */
object ReminderRecurrence {
    const val ONCE = "once"
    const val WEEKLY = "weekly"
    const val MONTHLY = "monthly"
    const val QUARTERLY = "quarterly"
    const val SIX_MONTHS = "six-months"
    const val YEARLY = "yearly"

    val ALL = listOf(ONCE, WEEKLY, MONTHLY, QUARTERLY, SIX_MONTHS, YEARLY)
}
