package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * A note. gorm.Model identity keys serialize in PascalCase; always carries a
 * nested contact. `content` is the note body (notes have no title).
 */
@JsonClass(generateAdapter = true)
data class Note(
    @Json(name = "ID") val id: Int = 0,
    @Json(name = "CreatedAt") val createdAt: String? = null,
    @Json(name = "UpdatedAt") val updatedAt: String? = null,
    val content: String? = null,
    val date: String? = null,
    @Json(name = "contact_id") val contactId: Int? = null,
    val contact: ContactFlat? = null,
    /** T17 change-feed tombstone marker. */
    val deleted: Boolean = false,
)

/** Request body for POST /contacts/{id}/notes and PUT /notes/{id}. */
@JsonClass(generateAdapter = true)
data class NoteInput(
    val content: String? = null,
    val date: String? = null,
    @Json(name = "contact_id") val contactId: Int? = null,
)

/** GET /contacts/{id}/notes — wrapped `{ notes }` array. */
@JsonClass(generateAdapter = true)
data class ContactNotesResponse(
    val notes: List<Note> = emptyList(),
)

/** POST /contacts/{id}/notes — wrapped `{ message, note }`. */
@JsonClass(generateAdapter = true)
data class CreateNoteResponse(
    val message: String? = null,
    val note: Note? = null,
)
