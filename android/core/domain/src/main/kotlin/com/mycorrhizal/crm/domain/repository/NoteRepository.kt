package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.model.network.NoteInput

/**
 * Note data access. Online-first: writes go to the server and the returned
 * record is mirrored into the local cache.
 */
interface NoteRepository {
    /** A contact's notes. */
    suspend fun listForContact(contactId: Int): Result<List<Note>>

    /** A single note. */
    suspend fun get(id: Int): Result<Note>

    /** Create a note on a contact; returns the created note. */
    suspend fun create(contactId: Int, input: NoteInput): Result<Note>

    /** Update a note; returns the updated note. */
    suspend fun update(id: Int, input: NoteInput): Result<Note>
}
