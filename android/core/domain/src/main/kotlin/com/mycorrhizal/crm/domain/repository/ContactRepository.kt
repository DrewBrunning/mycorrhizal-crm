package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.ApplyContactAddressSuggestionInput
import com.mycorrhizal.crm.model.network.ContactAddressSuggestion
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
     * [circle] filters by circle NAME (the backend's `?circle=` matches
     * `circles.name`); [includeArchived] widens the row set to archived
     * contacts when true.
     */
    suspend fun listContacts(
        cursor: String? = null,
        limit: Int = 50,
        search: String? = null,
        circle: String? = null,
        circleLegacy: String? = null,
        includeArchived: Boolean? = null,
    ): Result<ContactsPage>

    /**
     * M26: GET /contacts/circles?legacy=true — the distinct legacy free-text
     * circle strings still in the old flat `contacts.circles` JSON column, for
     * the circle/tag-triage tool.
     */
    suspend fun listLegacyCircles(): Result<List<String>>

    /** Fetch one contact from the server, falling back to the cached copy. */
    suspend fun getContact(id: Int): Result<ContactRecordResponse>

    /**
     * Batch-resolve Contact.VCardUIDs to display summaries via the server's
     * repeatable `?vcard_uid=` lookup (a display-only helper -- e.g. turning
     * a RelationshipEdge's raw source/target UID into a name -- not cached
     * locally). Empty input short-circuits without a network call. A UID
     * with no matching contact is simply absent from the result map.
     * Includes archived contacts (mirrors web's `getContactsByUid`, which
     * sends `include_archived: true`) so a reference to an archived contact
     * still resolves to a name/link instead of silently vanishing.
     */
    suspend fun resolveByUid(uids: List<String>): Result<Map<String, ContactSummary>>

    /** Create a contact on the server; returns the created record (writes online-first). */
    suspend fun createContact(input: ContactRecordInput): Result<ContactRecordResponse>

    /** Update a contact on the server; returns the updated record. */
    suspend fun updateContact(id: Int, input: ContactRecordInput): Result<ContactRecordResponse>

    // M24: top-level contact actions. Delete/archive/unarchive were repository-level gaps
    // (no client surface at all), not just missing UI; the backend endpoints pre-date the
    // Android client. All three are online-first: on success the local Room mirror is updated
    // so the list/detail degrade consistently offline.

    /**
     * Delete a contact (soft delete per `/CLAUDE.md`'s delete-semantics — the row survives
     * server-side for the audit undo). Removes the local cache row.
     */
    suspend fun deleteContact(id: Int): Result<Unit>

    /** Archive a contact (backend also retires its reminders); flips the cached `archived` flag. */
    suspend fun archiveContact(id: Int): Result<Unit>

    /** Unarchive a contact; flips the cached `archived` flag back. */
    suspend fun unarchiveContact(id: Int): Result<Unit>

    /**
     * Export a single contact as vCard 4.0 (or 3.0 when [version] == 3) — the raw file bytes
     * from `GET /export/vcf?vcard_uid=…`. Matches web's single-contact export (default field
     * selection: all sections, private/secret sensitivity excluded). Not cached.
     */
    suspend fun exportContactVcf(vcardUid: String, version: Int? = null): Result<ByteArray>

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

    /**
     * 167: scan for contact-address suggestions — the addresses a contact
     * probably shares because of a confirmed parent/child, spouse, or roommate
     * edge, or household membership. Read-only and idempotent; nothing is
     * written until [applyContactAddressSuggestion].
     */
    suspend fun suggestContactAddresses(): Result<List<ContactAddressSuggestion>>

    /**
     * 167: apply one address suggestion. The server re-derives the address
     * from the current graph (the relationship or household must still hold),
     * so the client only names the suggestion by identity.
     */
    suspend fun applyContactAddressSuggestion(input: ApplyContactAddressSuggestionInput): Result<Unit>
}
