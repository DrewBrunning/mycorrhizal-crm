package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

// ---------------------------------------------------------------------------
// Contact briefing (N2 / M11) — GET /contacts/:id/briefing
// ---------------------------------------------------------------------------

/**
 * Read-only N2 prep-view composition (GET /contacts/:id/briefing): everything
 * the user wants to remember before seeing a person, assembled server-side
 * from existing data (activities, notes, cadence health, agenda items,
 * relationship edges, life events, reminders, upcoming dates). A pure
 * aggregation of existing data — never persisted, never cached; every block
 * degrades to its zero value when its source feature is empty.
 *
 * **The six collection blocks are the trap `/CLAUDE.md` frontend trap #8
 * warns about.** The backend contract (`backend/models/briefing.go`) says they
 * serialize as `[]`, always — they previously had `omitempty`, so a contact
 * with no history returned every block *absent* and web's prep view crashed
 * into its ErrorBoundary (`briefing.open_agenda_items.length` on an
 * undefined array) on first use. The same regression is defended here the way
 * `NotesPage`/`FieldDefinitionsResponse` do: the raw fields stay nullable and
 * [recentNotes]&co normalize absent/null/`[]` to a plain empty list, so the
 * screen can dereference `.size` unconditionally.
 */
@JsonClass(generateAdapter = true)
data class ContactBriefing(
    @Json(name = "contact_id") val contactId: Int = 0,
    val uid: String = "",
    val name: String = "",
    /** The avatar's data-URL/thumbnail, empty when none. */
    @Json(name = "photo_thumbnail") val photoThumbnail: String? = null,
    /** CRMEnvelope.Kind (human|animal), used for the header badge. */
    val kind: String? = null,
    /** The single most recent activity; null when the contact has none. */
    @Json(name = "last_activity") val lastActivity: BriefingActivity? = null,
    @Json(name = "recent_notes") val recentNotesRaw: List<Note>? = null,
    /** Derived relationship health (T19), or null when no CadencePolicy exists. */
    val cadence: BriefingCadence? = null,
    @Json(name = "open_agenda_items") val openAgendaItemsRaw: List<ConversationAgenda>? = null,
    @Json(name = "relationships") val relationshipsRaw: List<BriefingRelationship>? = null,
    @Json(name = "life_events") val lifeEventsRaw: List<LifeEvent>? = null,
    @Json(name = "upcoming_reminders") val upcomingRemindersRaw: List<Reminder>? = null,
    @Json(name = "upcoming_dates") val upcomingDatesRaw: List<BriefingUpcomingDate>? = null,
) {
    val recentNotes: List<Note> get() = recentNotesRaw.orEmpty()
    val openAgendaItems: List<ConversationAgenda> get() = openAgendaItemsRaw.orEmpty()
    val relationships: List<BriefingRelationship> get() = relationshipsRaw.orEmpty()
    val lifeEvents: List<LifeEvent> get() = lifeEventsRaw.orEmpty()
    val upcomingReminders: List<Reminder> get() = upcomingRemindersRaw.orEmpty()
    val upcomingDates: List<BriefingUpcomingDate> get() = upcomingDatesRaw.orEmpty()
}

/**
 * The slim projection of an Activity shown as the briefing's "last
 * interaction" block — deliberately not the full Activity (no join-table
 * contacts preload), just what the scan needs.
 */
@JsonClass(generateAdapter = true)
data class BriefingActivity(
    val id: Int = 0,
    val uuid: String? = null,
    val title: String = "",
    val description: String? = null,
    val type: String? = null,
    val location: String? = null,
    /** ISO 8601 timestamp. */
    val date: String? = null,
)

/**
 * Bundles a contact's CadencePolicy with its derived health. Health is the
 * DERIVED relationship-health read-model (§91.10) computed server-side — the
 * client must never recompute it locally (see M11's health-card test case).
 */
@JsonClass(generateAdapter = true)
data class BriefingCadence(
    val policy: BriefingCadencePolicy? = null,
    val health: BriefingCadenceHealth = BriefingCadenceHealth(),
)

/** Wire mirror of the backend's CadencePolicy as carried on the briefing. */
@JsonClass(generateAdapter = true)
data class BriefingCadencePolicy(
    val id: String = "",
    @Json(name = "entity_id") val entityId: String = "",
    @Json(name = "target_interval_days") val targetIntervalDays: Int = 0,
    @Json(name = "qualifying_types") val qualifyingTypes: List<String> = emptyList(),
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "updated_at") val updatedAt: String? = null,
)

/**
 * The derived relationship health as carried on the briefing — the same
 * fields `services.CadenceHealth` computes. [hasQualifyingInteraction] is
 * false until the contact has at least one qualifying interaction;
 * [overdueBy] is whole calendar days past due (0 when due today or later).
 */
@JsonClass(generateAdapter = true)
data class BriefingCadenceHealth(
    @Json(name = "has_qualifying_interaction") val hasQualifyingInteraction: Boolean = false,
    @Json(name = "last_interaction") val lastInteraction: String? = null,
    @Json(name = "next_due") val nextDue: String? = null,
    @Json(name = "overdue_by") val overdueBy: Int = 0,
)

/**
 * One confirmed edge resolved for display: the raw edge plus the other
 * party's name and the display label token as read from the viewed contact's
 * perspective (already inverse-applied server-side; the frontend renders the
 * token with underscores replaced). [otherPartyContactID] is the numeric ID
 * the detail route is addressed by.
 */
@JsonClass(generateAdapter = true)
data class BriefingRelationship(
    val edge: RelationshipEdge? = null,
    @Json(name = "other_party_contact_id") val otherPartyContactId: Int? = null,
    @Json(name = "other_party_name") val otherPartyName: String? = null,
    @Json(name = "other_party_uid") val otherPartyUid: String? = null,
    @Json(name = "display_token") val displayToken: String? = null,
)

/** One upcoming birthday/anniversary for the briefing. */
@JsonClass(generateAdapter = true)
data class BriefingUpcomingDate(
    /** Stable token the client renders: "birthday" or "anniversary". */
    val label: String = "",
    /** Stored YYYY-MM-DD or --MM-DD value. */
    val date: String = "",
    /** Whole days from today; 0 when the date is today. */
    @Json(name = "days_until") val daysUntil: Int = 0,
)
