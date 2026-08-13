package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.data.local.CachedNote
import com.mycorrhizal.crm.data.local.CachedNoteDao
import com.mycorrhizal.crm.domain.repository.NoteRepository
import com.mycorrhizal.crm.domain.repository.UnfiledNotesPage
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.model.network.NoteInput
import com.mycorrhizal.crm.network.ApiClient
import javax.inject.Inject

/**
 * Online-first note access. Writes go to the server; successful responses are
 * mirrored into the Room cache.
 */
class NoteRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
    private val dao: CachedNoteDao,
) : NoteRepository {

    override suspend fun listForContact(contactId: Int): Result<List<Note>> {
        val result = apiClient.listContactNotes(contactId)
        val response = result.getOrNull()
        if (response != null) {
            dao.upsertAll(response.notes.map { it.toCached() })
        }
        return result.map { response -> response.notes }
    }

    override suspend fun listUnfiled(cursor: String?, limit: Int?): Result<UnfiledNotesPage> {
        val result = apiClient.listNotes(cursor, limit)
        result.getOrNull()?.let { page -> dao.upsertAll(page.notes.map { it.toCached() }) }
        return result.map { page -> UnfiledNotesPage(notes = page.notes, nextCursor = page.nextCursor, total = page.total) }
    }

    override suspend fun get(id: Int): Result<Note> = apiClient.getNote(id)

    override suspend fun create(contactId: Int, input: NoteInput): Result<Note> {
        val result = apiClient.createNote(contactId, input)
        result.getOrNull()?.let { dao.upsert(it.toCached()) }
        return result
    }

    override suspend fun update(id: Int, input: NoteInput): Result<Note> {
        val result = apiClient.updateNote(id, input)
        result.getOrNull()?.let { dao.upsert(it.toCached()) }
        return result
    }

    private fun Note.toCached(): CachedNote = CachedNote(
        id = id,
        content = content,
        date = date,
        deleted = deleted,
    )
}
