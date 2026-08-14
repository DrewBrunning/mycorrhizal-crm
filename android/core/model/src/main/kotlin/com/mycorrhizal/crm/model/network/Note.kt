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

/** GET /contacts/{id}/notes — wrapped `{ notes }` array (M19: T17 cursor envelope). */
@JsonClass(generateAdapter = true)
data class ContactNotesResponse(
    val notes: List<Note> = emptyList(),
    @Json(name = "next_cursor") val nextCursor: String? = null,
    val limit: Int = 0,
)

/** POST /contacts/{id}/notes — wrapped `{ message, note }`. */
@JsonClass(generateAdapter = true)
data class CreateNoteResponse(
    val message: String? = null,
    val note: Note? = null,
)

/**
 * GET /notes — the N4 unfiled-notes inbox, T17 cursor-paginated. `GetUnassignedNotes` does
 * `var notes []models.Note; ...Find(&notes)`, and Go marshals a nil slice (zero unfiled notes) as
 * JSON `null`, not `[]` (`/CLAUDE.md` frontend trap #8). A non-null Kotlin default only covers an
 * *absent* key, not an explicit `null` value — Moshi codegen still rejects the latter — so the raw
 * field stays nullable and [notes] normalizes absent/null/`[]` to a plain empty list, mirroring
 * `FieldDefinitionsResponse.definitions`' fix for the same trap.
 */
@JsonClass(generateAdapter = true)
data class NotesPage(
    @Json(name = "notes") val notesRaw: List<Note>? = null,
    @Json(name = "next_cursor") val nextCursor: String? = null,
    val limit: Int = 0,
    /** N4 queue depth — total unfiled notes matching the filters, not just this page. */
    val total: Int = 0,
    val sync: SyncInfo? = null,
) {
    val notes: List<Note> get() = notesRaw.orEmpty()
}
