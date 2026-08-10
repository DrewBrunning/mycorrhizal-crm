package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.data.local.CachedContact
import com.mycorrhizal.crm.data.local.CachedContactDao
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.ContactsPage
import com.mycorrhizal.crm.model.network.ContactRecordInput
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.SyncInfo
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.toApiError
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.map
import javax.inject.Inject

/**
 * Online-first contact access. Fetches pages from the server and mirrors them
 * into Room; on network failure, falls back to the cached rows so the list
 * and detail screens degrade gracefully offline.
 */
class ContactRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
    private val dao: CachedContactDao,
) : ContactRepository {

    override suspend fun listContacts(
        cursor: String?,
        limit: Int,
        search: String?,
    ): Result<ContactsPage> {
        val result = apiClient.listContacts(
            cursor = cursor,
            limit = limit,
            search = search,
        )
        val page = result.getOrElse { error ->
            // Network failure: serve whatever is cached for this search term.
            return Result.failure(error.toApiError())
        }
        val rows = page.contacts.map { it.toCached() }.toMutableList()
        // A list row only carries the summary projection; preserve any cached
        // full detail (card/crm) so a list refresh doesn't wipe offline detail.
        mergePreservingDetail(rows)
        dao.upsertAll(rows)
        applySync(page.sync)
        return Result.success(
            ContactsPage(
                contacts = page.contacts,
                nextCursor = page.nextCursor,
                limit = page.limit,
                sync = page.sync,
            ),
        )
    }

    override suspend fun getContact(id: Int): Result<ContactRecordResponse> {
        val result = apiClient.getContact(id)
        return result.fold(
            onSuccess = { record ->
                dao.upsert(record.toCached())
                Result.success(record)
            },
            onFailure = { error ->
                // Only fall back to the cache for connectivity-class failures.
                // A 404 means the contact is gone server-side; serving stale
                // cached data for a deleted contact would be misleading.
                val shouldUseCache = error is ApiError.Network ||
                    error is ApiError.Timeout ||
                    error is ApiError.Unknown ||
                    error is ApiError.Server
                if (!shouldUseCache) {
                    return@fold Result.failure(error)
                }
                val cached = dao.getById(id)
                val fromCache = cached?.let { it.toRecord() }
                if (fromCache != null) Result.success(fromCache)
                else Result.failure(error)
            },
        )
    }

    override suspend fun createContact(input: ContactRecordInput): Result<ContactRecordResponse> {
        val result = apiClient.createContact(input)
        return result.fold(
            onSuccess = { record ->
                dao.upsert(record.toCached())
                Result.success(record)
            },
            onFailure = { error -> Result.failure(error) },
        )
    }

    override suspend fun updateContact(id: Int, input: ContactRecordInput): Result<ContactRecordResponse> {
        val result = apiClient.updateContact(id, input)
        return result.fold(
            onSuccess = { record ->
                dao.upsert(record.toCached())
                Result.success(record)
            },
            onFailure = { error -> Result.failure(error) },
        )
    }

    override fun observeContacts(): Flow<List<ContactSummary>> =
        flow {
            emit(dao.getAll().map { it.toSummary() })
        }

    override suspend fun findByPhone(phone: String): ContactSummary? =
        dao.findByPhoneDigits(phone)?.toSummary()

    override suspend fun searchLocal(query: String): List<ContactSummary> {
        val trimmed = query.trim()
        if (trimmed.isEmpty()) return dao.getAll().map { it.toSummary() }
        // Sanitize the query so a plain search term can never break the FTS
        // MATCH expression (unbalanced parens or a bare NEAR throw
        // "malformed MATCH expression" and crash the offline path).
        val safe = trimmed
            .replace("\"", " ")
            .replace(Regex("""[()*:\-]"""), " ")
            .replace(Regex("""\b(AND|OR|NOT|NEAR)\b""", RegexOption.IGNORE_CASE), " ")
            .replace(Regex("""\s+"""), " ")
            .trim()
        if (safe.isEmpty()) return dao.getAll().map { it.toSummary() }
        return try {
            dao.searchFts(safe).map { it.toSummary() }
        } catch (_: Exception) {
            // FTS rejected the expression after all — fall back to the LIKE scan.
            dao.search(trimmed).map { it.toSummary() }
        }
    }

    override fun observeContact(id: Int): Flow<ContactRecordResponse?> =
        flow {
            emit(dao.getById(id)?.toRecord())
        }

    /** Apply the T17 sync signal from a list response to the cache. */
    private suspend fun applySync(sync: SyncInfo?) {
        if (sync == null) return
        val ids = sync.incremental.orEmpty()
            .mapNotNull { it.toIntOrNull() }
        if (ids.isNotEmpty()) dao.deleteByIds(ids)
        // full_resync handling is a Phase-3 concern (multi-table); the
        // contact table is always replaced by the fetched page anyway.
    }

    /**
     * A list page's rows are summaries with null `card`/`crm`. Before
     * upserting, carry over the cached full detail for rows we already hold,
     * so a plain list refresh never destroys offline-usable detail.
     */
    private suspend fun mergePreservingDetail(rows: MutableList<CachedContact>) {
        if (rows.isEmpty()) return
        val cachedById = dao.getByIds(rows.map { it.id })
            .associateBy { it.id }
        for (i in rows.indices) {
            val cached = cachedById[rows[i].id] ?: continue
            val row = rows[i]
            rows[i] = row.copy(
                card = row.card ?: cached.card,
                crm = row.crm ?: cached.crm,
                photoThumbnail = row.photoThumbnail ?: cached.photoThumbnail,
            )
        }
    }

    private fun ContactSummary.toCached(): CachedContact = CachedContact(
        id = id,
        uid = uid,
        firstname = firstname,
        lastname = lastname,
        nickname = nickname,
        fn = fn,
        primaryEmail = primaryEmail,
        primaryPhone = primaryPhone,
        birthday = birthday,
        org = org,
        photoThumbnail = photoThumbnail,
        circles = circles,
        archived = archived,
        deleted = deleted,
    )

    private fun ContactRecordResponse.toCached(): CachedContact = CachedContact(
        id = id,
        uid = uid ?: card?.uid,
        firstname = card?.name?.components?.firstOrNull { it.kind == "given" }?.value,
        lastname = card?.name?.components?.firstOrNull { it.kind == "surname" }?.value,
        nickname = card?.nicknames?.firstOrNull()?.name,
        fn = card?.name?.full,
        primaryEmail = card?.emails?.firstOrNull()?.address,
        primaryPhone = card?.phones?.firstOrNull()?.number,
        org = card?.organizations?.firstOrNull()?.name,
        photoThumbnail = photoThumbnail,
        circles = crm?.circles,
        archived = archived,
        card = card,
        crm = crm,
    )

    private fun CachedContact.toSummary(): ContactSummary = ContactSummary(
        id = id,
        uid = uid,
        firstname = firstname,
        lastname = lastname,
        nickname = nickname,
        fn = fn,
        primaryEmail = primaryEmail,
        primaryPhone = primaryPhone,
        birthday = birthday,
        org = org,
        photoThumbnail = photoThumbnail,
        circles = circles,
        archived = archived,
        deleted = deleted,
    )

    private fun CachedContact.toRecord(): ContactRecordResponse? {
        val c = card ?: return null
        return ContactRecordResponse(
            id = id,
            uid = uid,
            card = c,
            crm = crm,
            photoThumbnail = photoThumbnail,
            archived = archived,
        )
    }
}
