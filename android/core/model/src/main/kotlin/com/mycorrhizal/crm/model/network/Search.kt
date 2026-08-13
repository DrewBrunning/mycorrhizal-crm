package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * GET /search (T11/WP-86; folded into the Android contact list by T87): cross-entity full-text
 * search across notes and activities. The response's `contacts` group is deliberately not
 * modeled here — T87 discards it, since the contact list's own `?search=` query (T85, FTS) is
 * the sole authority for contact results; rendering both would show two disagreeing contact
 * lists on one screen. Moshi ignores JSON keys with no matching property, so the server sending
 * `contacts` is harmless — it simply never reaches this type.
 */
@JsonClass(generateAdapter = true)
data class SearchResult(
    val query: String? = null,
    /** Canonical relation token when the whole query is a registry synonym ("brother" -> sibling_of). */
    @Json(name = "resolved_relation") val resolvedRelation: String? = null,
    val notes: List<SearchNoteHit> = emptyList(),
    val activities: List<SearchActivityHit> = emptyList(),
)

/** One matched note (backend embeds `models.Note`, gorm.Model identity keys serialize PascalCase). */
@JsonClass(generateAdapter = true)
data class SearchNoteHit(
    @Json(name = "ID") val id: Int = 0,
    val content: String? = null,
    val date: String? = null,
    @Json(name = "contact_id") val contactId: Int? = null,
    @Json(name = "contact_name") val contactName: String? = null,
    val snippet: String? = null,
)

/** One matched activity (backend embeds `models.Activity`). */
@JsonClass(generateAdapter = true)
data class SearchActivityHit(
    @Json(name = "ID") val id: Int = 0,
    val title: String? = null,
    val description: String? = null,
    val location: String? = null,
    val date: String? = null,
    val type: String? = null,
    val contacts: List<ContactFlat>? = null,
    val snippet: String? = null,
)
