package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * GET /search (T11/WP-86; folded into the Android contact list by T87): cross-entity full-text
 * search across notes, activities and (#2472) reusable entities. The response's `contacts` group
 * is deliberately never rendered — T87 discards it as a list, since the contact list's own
 * `?search=` query (T85, FTS) is the sole authority for contact results; rendering both would show
 * two disagreeing contact lists on one screen. Only each hit's id and `matched_on` (#2630) are
 * read, to label the contact list's own rows with why they matched ([matchedOnByContactId]).
 */
@JsonClass(generateAdapter = true)
data class SearchResult(
    val query: String? = null,
    /** Canonical relation token when the whole query is a registry synonym ("brother" -> sibling_of). */
    @Json(name = "resolved_relation") val resolvedRelation: String? = null,
    val notes: List<SearchNoteHit> = emptyList(),
    val activities: List<SearchActivityHit> = emptyList(),
    /** Reusable entities (organisation/place/pet) matched by name (#2472); older servers omit it. */
    val entities: List<SearchEntityHit> = emptyList(),
    /** Contact hits, read only for [SearchContactHit.matchedOn]; never rendered as a list. */
    val contacts: List<SearchContactHit> = emptyList()
) {
    /**
     * Contact id -> the fields the query matched (#2630, backend #2757). A contact the server did
     * not report on (older server, omitted when unknown, beyond the page) is absent.
     */
    fun matchedOnByContactId(): Map<Int, List<String>> = contacts
        .filter { it.id != 0 && !it.matchedOn.isNullOrEmpty() }
        .associate { it.id to it.matchedOn.orEmpty() }
}

/**
 * The slice of a `GET /search` contact hit that is used: its id and which of its own fields the
 * query matched. [matchedOn] is a subset of `name`, `organization`, `email`, `phone`, `address`
 * in that order (a hardcoded mirror of the backend vocabulary; unknown tokens are ignored by the
 * UI) and is null when the server omits it.
 */
@JsonClass(generateAdapter = true)
data class SearchContactHit(
    val id: Int = 0,
    @Json(name = "matched_on") val matchedOn: List<String>? = null
)

/** One matched reusable entity; `kind` and `sensitivity` are the wire tokens (see [EntityKind]). */
@JsonClass(generateAdapter = true)
data class SearchEntityHit(
    val id: String = "",
    val kind: String = EntityKind.ORGANISATION,
    val name: String = "",
    val sensitivity: String = "normal",
    /** Always `["name"]` from a server that reports it (#2630); null from an older one. */
    @Json(name = "matched_on") val matchedOn: List<String>? = null
)

/** One matched note (backend embeds `models.Note`, gorm.Model identity keys serialize PascalCase). */
@JsonClass(generateAdapter = true)
data class SearchNoteHit(
    @Json(name = "ID") val id: Int = 0,
    val content: String? = null,
    val date: String? = null,
    @Json(name = "contact_id") val contactId: Int? = null,
    @Json(name = "contact_name") val contactName: String? = null,
    val snippet: String? = null
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
    val snippet: String? = null
)
