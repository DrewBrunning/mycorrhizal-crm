package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.data.local.CachedNote
import com.mycorrhizal.crm.data.local.CachedNoteDao
import com.mycorrhizal.crm.domain.repository.ContactNotesPage
import com.mycorrhizal.crm.domain.repository.NoteRepository
import com.mycorrhizal.crm.domain.repository.NotesScope
import com.mycorrhizal.crm.domain.repository.UnfiledNotesPage
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.model.network.NoteImage
import com.mycorrhizal.crm.model.network.NoteInput
import com.mycorrhizal.crm.network.ApiClient
import javax.inject.Inject

/**
 * The Notes list (issue #2616, backend #2744) and a contact's notes (issue #2797, backend #2813)
 * order by the note's own date. Their cursors are sort-specific, so every page of a list must send
 * this same value.
 */
private const val NOTES_SORT_DATE = "date"

/**
 * Online-first note access. Writes go to the server; successful responses are
 * mirrored into the Room cache.
 */
class NoteRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
    private val dao: CachedNoteDao
) : NoteRepository {

    override suspend fun listForContact(
        contactId: Int,
        cursor: String?,
        limit: Int?,
        search: String?,
        fromDate: String?,
        toDate: String?
    ): Result<ContactNotesPage> {
        val result = apiClient.listContactNotes(
            contactId,
            cursor,
            limit,
            search,
            fromDate,
            toDate,
            NOTES_SORT_DATE
        )
        val response = result.getOrNull()
        if (response != null) {
            dao.upsertAll(response.notes.map { it.toCached() })
        }
        return result.map { response ->
            ContactNotesPage(
                notes = response.notes,
                nextCursor = response.nextCursor
            )
        }
    }

    override suspend fun listNotes(
        scope: NotesScope,
        cursor: String?,
        limit: Int?
    ): Result<UnfiledNotesPage> {
        val result = when (scope) {
            NotesScope.ALL -> apiClient.listNotes(cursor, limit, "all", true, NOTES_SORT_DATE)

            NotesScope.UNFILED -> apiClient.listNotes(
                cursor,
                limit,
                "unfiled",
                false,
                NOTES_SORT_DATE
            )
        }
        result.getOrNull()?.let { page -> dao.upsertAll(page.notes.map { it.toCached() }) }
        return result.map { page ->
            UnfiledNotesPage(
                notes = page.notes,
                nextCursor = page.nextCursor,
                total = page.total
            )
        }
    }

    override suspend fun get(id: Int): Result<Note> = apiClient.getNote(id)

    override suspend fun create(contactId: Int, input: NoteInput): Result<Note> {
        val result = apiClient.createNote(contactId, input)
        result.getOrNull()?.let { dao.upsert(it.toCached()) }
        return result
    }

    override suspend fun createUnassigned(input: NoteInput): Result<Note> {
        val result = apiClient.createUnassignedNote(input)
        result.getOrNull()?.let { dao.upsert(it.toCached()) }
        return result
    }

    override suspend fun update(id: Int, input: NoteInput, revision: Int?): Result<Note> {
        val result = apiClient.updateNote(id, input, revision)
        result.getOrNull()?.let { dao.upsert(it.toCached()) }
        return result
    }

    override suspend fun delete(id: Int, revision: Int?): Result<Unit> {
        val result = apiClient.deleteNote(id, revision)
        if (result.isSuccess) dao.deleteById(id)
        return result
    }

    override suspend fun listImages(noteId: Int): Result<List<NoteImage>> =
        apiClient.listNoteImages(noteId).map { it.images }

    override suspend fun uploadImage(
        noteId: Int,
        fileName: String,
        mimeType: String,
        bytes: ByteArray
    ): Result<NoteImage> = apiClient.uploadNoteImage(noteId, fileName, mimeType, bytes)

    override suspend fun deleteImage(noteId: Int, imageId: Int): Result<Unit> =
        apiClient.deleteNoteImage(noteId, imageId)

    override suspend fun setImageSensitivity(
        noteId: Int,
        imageId: Int,
        sensitivity: String
    ): Result<NoteImage> = apiClient.patchNoteImageSensitivity(noteId, imageId, sensitivity)

    private fun Note.toCached(): CachedNote = CachedNote(
        id = id,
        content = content,
        date = date,
        deleted = deleted
    )
}
