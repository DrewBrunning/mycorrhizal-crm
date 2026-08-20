package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/** GET /contacts/birthdays — `{ birthdays: [...] }`. */
@JsonClass(generateAdapter = true)
data class Birthday(
    val type: String = "contact",
    val name: String = "",
    val birthday: String? = null,
    @Json(name = "photo_thumbnail") val photoThumbnail: String? = null,
    @Json(name = "contact_id") val contactId: Long = 0,
)

@JsonClass(generateAdapter = true)
data class BirthdaysResponse(
    val birthdays: List<Birthday> = emptyList(),
)

/** GET /cadence-policies/overdue — `{ overdue: [...] }`. */
@JsonClass(generateAdapter = true)
data class OverdueCadence(
    val policy: CadencePolicy? = null,
    val health: CadenceHealth? = null,
    @Json(name = "contact_id") val contactId: Long = 0,
    @Json(name = "contact_name") val contactName: String = "",
    @Json(name = "photo_thumbnail") val photoThumbnail: String? = null,
)

@JsonClass(generateAdapter = true)
data class OverdueCadencesResponse(
    val overdue: List<OverdueCadence> = emptyList(),
)

/** Kind tokens on [ReachOutSuggestion.kind] — mirrors the backend's ReachOutKind* constants (issue #177). */
object ReachOutKind {
    const val ORGANIZATION = "organization"
    const val TITLE = "title"
    const val ADDRESS = "address"
}

/**
 * GET /reach-out-suggestions, and embedded in the dashboard composite's
 * `reach_out_suggestions` block (issue #177) — a detected organization/title/
 * address change on a contact, surfaced as a dismissible suggestion. The
 * event-driven counterpart to [OverdueCadence]'s time-based trigger.
 */
@JsonClass(generateAdapter = true)
data class ReachOutSuggestion(
    val id: String = "",
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "updated_at") val updatedAt: String? = null,
    @Json(name = "contact_vcard_uid") val contactVCardUid: String = "",
    val kind: String = "",
    @Json(name = "old_value") val oldValue: String = "",
    @Json(name = "new_value") val newValue: String = "",
    @Json(name = "audit_event_id") val auditEventId: Long = 0,
    @Json(name = "reminder_id") val reminderId: Int? = null,
    val status: String = "",
    @Json(name = "contact_id") val contactId: Long = 0,
    @Json(name = "contact_name") val contactName: String = "",
    @Json(name = "photo_thumbnail") val photoThumbnail: String? = null,
)

@JsonClass(generateAdapter = true)
data class ReachOutSuggestionsResponse(
    val suggestions: List<ReachOutSuggestion> = emptyList(),
)
