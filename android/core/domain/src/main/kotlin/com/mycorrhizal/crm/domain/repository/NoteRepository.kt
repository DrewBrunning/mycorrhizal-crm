package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.model.network.NoteInput

/** A page of the N4 unfiled-notes inbox, with its T17 cursor-pagination state and queue depth. */
data class UnfiledNotesPage(
    val notes: List<Note>,
    val nextCursor: String?,
    val total: Int,
)

/** A page of one contact's notes (M19), with its T17 cursor-pagination state. */
data class ContactNotesPage(
    val notes: List<Note>,
    val nextCursor: String?,
)

/**
 * Note data access. Online-first: writes go to the server and the returned
 * record is mirrored into the local cache.
 */
interface NoteRepository {
    /**
     * A page of a contact's notes (M19: search/date-filtered, cursor-paginated).
     * [search] is free-text on content; [fromDate]/[toDate] are `YYYY-MM-DD`
     * bounds on the note date, both inclusive.
     */
    suspend fun listForContact(
        contactId: Int,
        cursor: String? = null,
        limit: Int? = null,
        search: String? = null,
        fromDate: String? = null,
        toDate: String? = null,
    ): Result<ContactNotesPage>

    /** The unfiled-notes inbox (M9 Notes drawer entry) — `GET /notes`, not a contact's history. */
    suspend fun listUnfiled(cursor: String? = null, limit: Int? = null): Result<UnfiledNotesPage>

    /** A single note. */
    suspend fun get(id: Int): Result<Note>

    /** Create a note on a contact; returns the created note. */
    suspend fun create(contactId: Int, input: NoteInput): Result<Note>

    /** Create an unassigned note (`POST /notes`); returns the created note. */
    suspend fun createUnassigned(input: NoteInput): Result<Note>

    /** Update a note; returns the updated note. */
    suspend fun update(id: Int, input: NoteInput): Result<Note>

    /** Delete a note (soft delete server-side; removes the local cache row). */
    suspend fun delete(id: Int): Result<Unit>
}
