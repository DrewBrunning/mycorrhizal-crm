package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.ContactRecordInput
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

    /** Create a contact on the server; returns the created record (writes online-first). */
    suspend fun createContact(input: ContactRecordInput): Result<ContactRecordResponse>

    /** Update a contact on the server; returns the updated record. */
    suspend fun updateContact(id: Int, input: ContactRecordInput): Result<ContactRecordResponse>

    /** Cached contact list summaries as a reactive stream (list + offline). */
    fun observeContacts(): Flow<List<ContactSummary>>

    /** Local phone-match for call/SMS tracking (digits-normalized). */
    suspend fun findByPhone(phone: String): ContactSummary?

    /** Local email match (case-insensitive) for T57 dedup. */
    suspend fun findByEmail(email: String): ContactSummary?

    /** Records the device Contacts LOOKUP_KEY for a contact after T57 import. */
    suspend fun setDeviceLookupKey(id: Int, lookupKey: String)

    /** Reads a contact's cached device LOOKUP_KEY, if any (§7.5.4). */
    suspend fun getDeviceLookupKey(id: Int): String?

    /**
     * Local full-text search over the Room cache (Phase 2 item 13). Returns
     * cached rows matching [query] via the FTS mirror; empty query returns
     * the whole cached list. Used as the offline fallback when the server is
     * unreachable.
     */
    suspend fun searchLocal(query: String): List<ContactSummary>

    /** Cached contact detail as a reactive stream. */
    fun observeContact(id: Int): Flow<ContactRecordResponse?>
}
