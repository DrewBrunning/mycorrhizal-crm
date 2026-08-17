package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * The M3 "today/overview" composite — one `GET /dashboard` call
 * replacing the four-request fan-out (`/contacts/birthdays`,
 * `/contacts/random`, `/reminders/upcoming`, `/cadence-policies/overdue`)
 * the dashboard used to fire. Every block degrades to `[]` when empty — the
 * backend normalizes nil slices so a key is never absent/null (the same
 * discipline `normalizeDashboardSlices` enforces server-side; CLUADE.md
 * frontend trap 8 is the bug it prevents).
 *
 * [DashboardReminder] is a flattened mirror of [Reminder] plus the
 * `contact_name` the backend embeds (M3 design decision 2) — the wire has the
 * reminder fields at the top level, so Moshi needs them spelled out rather
 * than nested. `random_contacts` is [DashboardRandomContact] (NOT
 * [ContactSummary], see that type's doc) and `overdue` reuses
 * [OverdueCadence]; their wire shapes are unchanged by the composite.
 */
@JsonClass(generateAdapter = true)
data class DashboardResponse(
    val birthdays: List<Birthday> = emptyList(),
    @Json(name = "random_contacts") val randomContacts: List<DashboardRandomContact> = emptyList(),
    @Json(name = "upcoming_reminders") val upcomingReminders: List<DashboardReminder> = emptyList(),
    val overdue: List<OverdueCadence> = emptyList(),
)

/**
 * One `random_contacts` entry. Deliberately NOT [ContactSummary]: the
 * dashboard composite serializes this block as `ContactResponse`, which
 * embeds the raw `models.Contact` (gorm.Model identity keys serialize in
 * PascalCase — `ID`), while the contacts list endpoint returns the slim
 * `ContactSummaryDTO` with a lowercase `id`. Reusing [ContactSummary] here
 * silently defaulted every random contact's id to 0 (Moshi found no `id`
 * key), so the dashboard's LazyColumn keyed every row `"0"` and crashed with
 * "Key \"0\" was already used" on any dashboard that had 2+ random contacts.
 *
 * Mirrors the wire's PascalCase ID the way [DashboardReminder] does.
 */
@JsonClass(generateAdapter = true)
data class DashboardRandomContact(
    @Json(name = "ID") val id: Int = 0,
    val firstname: String? = null,
    val lastname: String? = null,
    val nickname: String? = null,
    @Json(name = "photo_thumbnail") val photoThumbnail: String? = null,
)

/** One `upcoming_reminders` entry: a reminder with its contact's display name embedded. */
@JsonClass(generateAdapter = true)
data class DashboardReminder(
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
    @Json(name = "contact_name") val contactName: String = "",
)
