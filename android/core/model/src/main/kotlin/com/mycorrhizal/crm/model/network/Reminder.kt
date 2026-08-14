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

/**
 * A reminder completion timeline record (backend `ReminderCompletion`). Written
 * when a reminder is completed; `DELETE /reminder-completions/:id` removes it
 * (the web's "undo" of a completed reminder's timeline entry).
 */
@JsonClass(generateAdapter = true)
data class ReminderCompletion(
    @Json(name = "ID") val id: Int = 0,
    @Json(name = "CreatedAt") val createdAt: String? = null,
    @Json(name = "UpdatedAt") val updatedAt: String? = null,
    @Json(name = "reminder_id") val reminderId: Int? = null,
    @Json(name = "contact_id") val contactId: Int = 0,
    val message: String? = null,
    @Json(name = "completed_at") val completedAt: String? = null,
)

/**
 * GET /contacts/{id}/reminder-completions — wrapped `{ completions }` array.
 * `GetCompletionsForContact` does `var completions []models.ReminderCompletion;
 * ...Find(&completions)`, and Go marshals a nil slice (no completions) as JSON
 * `null`, not `[]` (`/CLAUDE.md` frontend trap #8). A non-null Kotlin default
 * only covers an *absent* key, not an explicit `null` — Moshi codegen still
 * rejects the latter — so the raw field stays nullable and [completions]
 * normalizes absent/null/`[]` to a plain empty list (the `ActivitiesPage`
 * pattern).
 */
@JsonClass(generateAdapter = true)
data class CompletionsResponse(
    @Json(name = "completions") val completionsRaw: List<ReminderCompletion>? = null,
) {
    val completions: List<ReminderCompletion> get() = completionsRaw.orEmpty()
}

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
