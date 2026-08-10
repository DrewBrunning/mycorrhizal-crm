package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

@JsonClass(generateAdapter = true)
data class Tag(
    val id: String = "",
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "updated_at") val updatedAt: String? = null,
    val name: String = "",
)

@JsonClass(generateAdapter = true)
data class ContactTag(
    val id: Int = 0,
    @Json(name = "tag_id") val tagId: String = "",
    @Json(name = "contact_vcard_uid") val contactVCardUid: String = "",
)

/** GET /tags response — cursor-paginated, contacts only when include_contacts=true. */
@JsonClass(generateAdapter = true)
data class TagsPage(
    val tags: List<Tag> = emptyList(),
    val total: Int = 0,
    @Json(name = "next_cursor") val nextCursor: String? = null,
    val limit: Int = 0,
    val contacts: List<ContactTag>? = null,
    val sync: SyncInfo? = null,
)

/** POST /tags response — wrapped `{ message, tag }` (ticket §2.6). */
@JsonClass(generateAdapter = true)
data class CreateTagResponse(
    val message: String? = null,
    val tag: Tag? = null,
)

/** GET /tags/{id} response — `{ tag, contacts }`. */
@JsonClass(generateAdapter = true)
data class TagDetailResponse(
    val tag: Tag? = null,
    val contacts: List<ContactTag>? = null,
)

/** POST /tags/{id}/contacts response — `{ message, tagging }`. */
@JsonClass(generateAdapter = true)
data class AddContactTagResponse(
    val message: String? = null,
    val tagging: ContactTag? = null,
)

/** POST /tags body — the API only models name (taggings are a sub-resource). */
@JsonClass(generateAdapter = true)
data class TagInput(
    val name: String,
)

/** POST /tags/{id}/contacts body. */
@JsonClass(generateAdapter = true)
data class ContactTagInput(
    @Json(name = "contact_vcard_uid") val contactVCardUid: String,
)
