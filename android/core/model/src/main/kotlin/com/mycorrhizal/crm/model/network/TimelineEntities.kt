package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

// PartialDate lives in ContactRecord.kt (shared by birthdays and life events).

// ---------------------------------------------------------------------------
// Life events
// ---------------------------------------------------------------------------

@JsonClass(generateAdapter = true)
data class LifeEvent(
    val id: String = "",
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "updated_at") val updatedAt: String? = null,
    @Json(name = "entity_id") val entityId: String = "",
    val type: String? = null,
    val category: String? = null,
    val date: PartialDate? = null,
    val description: String? = null,
    val source: String? = null,
    @Json(name = "related_entity_ids") val relatedEntityIds: List<String>? = null,
    val remind: Boolean? = null,
    val deleted: Boolean? = null,
)

@JsonClass(generateAdapter = true)
data class LifeEventInput(
    @Json(name = "entity_id") val entityId: String,
    val type: String? = null,
    val category: String? = null,
    val date: PartialDate? = null,
    val description: String? = null,
    val source: String? = null,
    @Json(name = "related_entity_ids") val relatedEntityIds: List<String>? = null,
    val remind: Boolean? = null,
)

@JsonClass(generateAdapter = true)
data class LifeEventsPage(
    @Json(name = "life_events") val lifeEvents: List<LifeEvent> = emptyList(),
    @Json(name = "next_cursor") val nextCursor: String? = null,
    val limit: Int = 0,
    val sync: SyncInfo? = null,
)

@JsonClass(generateAdapter = true)
data class CreateLifeEventResponse(
    val message: String? = null,
    @Json(name = "life_event") val lifeEvent: LifeEvent? = null,
)

// ---------------------------------------------------------------------------
// Gifts
// ---------------------------------------------------------------------------

object GiftStatuses {
    const val IDEA = "idea"
    const val PURCHASED = "purchased"
    const val GIVEN = "given"
    const val RECEIVED = "received"
    val ALL: List<String> = listOf(IDEA, PURCHASED, GIVEN, RECEIVED)
}

@JsonClass(generateAdapter = true)
data class Gift(
    val id: String = "",
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "updated_at") val updatedAt: String? = null,
    @Json(name = "entity_id") val entityId: String = "",
    val status: String = GiftStatuses.IDEA,
    val occasion: String? = null,
    val description: String = "",
    val url: String? = null,
    val notes: String? = null,
    val date: String? = null,
    @Json(name = "value_cents") val valueCents: Long? = null,
    val currency: String? = null,
    @Json(name = "life_event_id") val lifeEventId: String? = null,
    @Json(name = "activity_id") val activityId: Int? = null,
    val deleted: Boolean? = null,
)

@JsonClass(generateAdapter = true)
data class GiftInput(
    @Json(name = "entity_id") val entityId: String,
    val status: String = GiftStatuses.IDEA,
    val occasion: String? = null,
    val description: String = "",
    val url: String? = null,
    val notes: String? = null,
    val date: String? = null,
    @Json(name = "value_cents") val valueCents: Long? = null,
    val currency: String? = null,
    @Json(name = "life_event_id") val lifeEventId: String? = null,
    @Json(name = "activity_id") val activityId: Int? = null,
)

@JsonClass(generateAdapter = true)
data class GiftsPage(
    val gifts: List<Gift> = emptyList(),
    @Json(name = "next_cursor") val nextCursor: String? = null,
    val limit: Int = 0,
    val sync: SyncInfo? = null,
)

@JsonClass(generateAdapter = true)
data class CreateGiftResponse(
    val message: String? = null,
    val gift: Gift? = null,
)

// ---------------------------------------------------------------------------
// Preferences
// ---------------------------------------------------------------------------

object PreferenceSensitivities {
    const val NORMAL = "normal"
    const val PRIVATE = "private"
    const val SECRET = "secret"
    val ALL: List<String> = listOf(NORMAL, PRIVATE, SECRET)
}

@JsonClass(generateAdapter = true)
data class Preference(
    val id: String = "",
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "updated_at") val updatedAt: String? = null,
    @Json(name = "entity_id") val entityId: String = "",
    val category: String = "",
    val key: String? = null,
    val value: String = "",
    val source: String? = null,
    val confidence: Double? = null,
    @Json(name = "last_confirmed") val lastConfirmed: String? = null,
    val sensitivity: String = PreferenceSensitivities.NORMAL,
    val deleted: Boolean? = null,
)

@JsonClass(generateAdapter = true)
data class PreferenceInput(
    @Json(name = "entity_id") val entityId: String,
    val category: String,
    val key: String? = null,
    val value: String,
    val source: String? = null,
    val confidence: Double? = null,
    @Json(name = "last_confirmed") val lastConfirmed: String? = null,
    val sensitivity: String = PreferenceSensitivities.NORMAL,
)

@JsonClass(generateAdapter = true)
data class PreferencesPage(
    val preferences: List<Preference> = emptyList(),
    val total: Int = 0,
    @Json(name = "next_cursor") val nextCursor: String? = null,
    val limit: Int = 0,
    val sync: SyncInfo? = null,
)

@JsonClass(generateAdapter = true)
data class CreatePreferenceResponse(
    val message: String? = null,
    val preference: Preference? = null,
)

// ---------------------------------------------------------------------------
// Conversation agenda
// ---------------------------------------------------------------------------

@JsonClass(generateAdapter = true)
data class ConversationAgenda(
    val id: String = "",
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "updated_at") val updatedAt: String? = null,
    @Json(name = "entity_id") val entityId: String = "",
    val content: String = "",
    @Json(name = "reference_url") val referenceUrl: String? = null,
    @Json(name = "discussed_at") val discussedAt: String? = null,
    @Json(name = "activity_id") val activityId: Int? = null,
    val deleted: Boolean? = null,
)

@JsonClass(generateAdapter = true)
data class ConversationAgendaInput(
    @Json(name = "entity_id") val entityId: String,
    val content: String,
    @Json(name = "reference_url") val referenceUrl: String? = null,
)

/** PATCH /conversation-agenda/:id/discuss body — `{}` when unlinked (M18). */
@JsonClass(generateAdapter = true)
data class DiscussConversationAgendaInput(
    @Json(name = "activity_id") val activityId: Int? = null,
)

@JsonClass(generateAdapter = true)
data class ConversationAgendaPage(
    @Json(name = "conversation_agenda") val conversationAgenda: List<ConversationAgenda> = emptyList(),
    @Json(name = "next_cursor") val nextCursor: String? = null,
    val limit: Int = 0,
    val sync: SyncInfo? = null,
)

@JsonClass(generateAdapter = true)
data class CreateConversationAgendaResponse(
    val message: String? = null,
    @Json(name = "conversation_agenda") val conversationAgenda: ConversationAgenda? = null,
)
