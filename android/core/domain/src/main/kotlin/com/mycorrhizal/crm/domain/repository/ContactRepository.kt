package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.SyncInfo
import kotlinx.coroutines.flow.Flow

/** A page of contacts with its T17 cursor-pagination state. */
data class ContactsPage(
    val contacts: List<ContactSummary>,
    val nextCursor: String?,
    val limit: Int,
    val sync: SyncInfo?,
)

/**
 * Contact data access. Implementations are online-first: they write through
 * to the server and keep a Room mirror for offline reads.
 */
interface ContactRepository {
    /**
     * Fetch a page from the server and refresh the local cache.
     * Returns the fetched page on success; on network failure falls back to
     * whatever is cached locally so the UI degrades gracefully offline.
     */
    suspend fun listContacts(
        cursor: String? = null,
        limit: Int = 50,
        search: String? = null,
    ): Result<ContactsPage>

    /** Fetch one contact from the server, falling back to the cached copy. */
    suspend fun getContact(id: Int): Result<ContactRecordResponse>

    /** Cached contact list summaries as a reactive stream (list + offline). */
    fun observeContacts(): Flow<List<ContactSummary>>

    /** Cached contact detail as a reactive stream. */
    fun observeContact(id: Int): Flow<ContactRecordResponse?>
}
