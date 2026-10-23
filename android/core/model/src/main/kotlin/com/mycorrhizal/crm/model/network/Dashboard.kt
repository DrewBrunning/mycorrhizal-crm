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
    // Issue #212: the favorites quick-access block (web #173) — a list of
    // ContactResponse exactly like random_contacts, so it reuses
    // [DashboardRandomContact] (PascalCase ID and all). Always `[]` on the
    // wire, never absent.
    val favorites: List<DashboardRandomContact> = emptyList(),
    // Issue #177: pending event-driven reach-out suggestions. Same
    // no-omitempty/always-`[]` discipline as every other block.
    @Json(name = "reach_out_suggestions") val reachOutSuggestions: List<ReachOutSuggestion> = emptyList(),
    // Issue #352: contacts whose info is due for re-verification. Same
    // no-omitempty/always-`[]` discipline as every other block.
    @Json(name = "data_decay_overdue") val dataDecayOverdue: List<OverdueDataDecayPolicy> = emptyList(),
    // Issue #1824 (server #1791/#1806): each list block above is capped at 25 rows;
    // the matching `*_total` is its size before the cap. Nullable, not defaulted to
    // 0: an older server omits them, and absent must mean "show no caption".
    @Json(name = "birthdays_total") val birthdaysTotal: Int? = null,
    @Json(name = "upcoming_reminders_total") val upcomingRemindersTotal: Int? = null,
    @Json(name = "overdue_total") val overdueTotal: Int? = null,
    @Json(name = "reach_out_suggestions_total") val reachOutSuggestionsTotal: Int? = null,
    @Json(name = "data_decay_overdue_total") val dataDecayOverdueTotal: Int? = null,
)

/**
 * Issue #1761: the compact "why is this contact here" signal behind a Stay in
 * Touch card, derived server-side from the same cadence computation as the
 * cadence view. Day counts (not timestamps) so this client renders the same
 * reminder-zone calendar-day line (ADR 0015) without recomputing dates.
 */
@JsonClass(generateAdapter = true)
data class DashboardContactRelationship(
    @Json(name = "has_cadence") val hasCadence: Boolean = false,
    /** 0 = today; null when no qualifying interaction has been recorded. */
    @Json(name = "days_since_last_interaction") val daysSinceLastInteraction: Int? = null,
    /** Negative when overdue, 0 = due today; null when no policy anchors a due date. */
    @Json(name = "days_until_due") val daysUntilDue: Int? = null,
)

/**
 * One `random_contacts` — and, since issue #212, one `favorites` — entry.
 * Deliberately NOT [ContactSummary]: the
 * dashboard composite serializes these blocks as `ContactResponse`, which
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
    /**
     * Issue #1761: the "why is this contact here" signal used as the Stay in
     * Touch card's subtitle. Absent (null) on an older server, which the UI
     * falls back to a generic reason for.
     */
    val relationship: DashboardContactRelationship? = null,
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
