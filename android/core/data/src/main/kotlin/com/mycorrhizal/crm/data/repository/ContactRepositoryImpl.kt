package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.data.local.CachedContact
import com.mycorrhizal.crm.data.local.CachedContactDao
import com.mycorrhizal.crm.data.local.PhoneKey
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.ContactsPage
import com.mycorrhizal.crm.model.network.ApplyContactAddressSuggestionInput
import com.mycorrhizal.crm.model.network.ContactAddressSuggestion
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
        circle: String?,
        circleLegacy: String?,
        includeArchived: Boolean?,
    ): Result<ContactsPage> {
        val result = apiClient.listContacts(
            cursor = cursor,
            limit = limit,
            search = search,
            includeArchived = includeArchived,
            circle = circle,
            circleLegacy = circleLegacy,
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

    override suspend fun listLegacyCircles(): Result<List<String>> =
        apiClient.listLegacyCircles()

    override suspend fun resolveByUid(uids: List<String>): Result<Map<String, ContactSummary>> {
        val distinctUids = uids.distinct()
        if (distinctUids.isEmpty()) return Result.success(emptyMap())
        // includeArchived: true matches web's getContactsByUid — a reference (an audit
        // event, a relationship edge) can point at an archived contact, and it must still
        // resolve to a name/link rather than silently vanishing (GetContacts excludes
        // archived by default, as does the ?vcard_uid= batch path).
        val result = apiClient.listContacts(vcardUids = distinctUids, includeArchived = true)
        return result.fold(
            onSuccess = { page ->
                Result.success(page.contacts.mapNotNull { c -> c.uid?.let { it to c } }.toMap())
            },
            onFailure = { error -> Result.failure(error.toApiError()) },
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

    override suspend fun deleteContact(id: Int): Result<Unit> {
        val result = apiClient.deleteContact(id)
        if (result.isSuccess) dao.deleteById(id)
        return result
    }

    override suspend fun archiveContact(id: Int): Result<Unit> {
        val result = apiClient.archiveContact(id)
        if (result.isSuccess) dao.setArchived(id, true)
        return result
    }

    override suspend fun unarchiveContact(id: Int): Result<Unit> {
        val result = apiClient.unarchiveContact(id)
        if (result.isSuccess) dao.setArchived(id, false)
        return result
    }

    override suspend fun exportContactVcf(vcardUid: String, version: Int?): Result<ByteArray> =
        apiClient.exportContactVcf(vcardUid, version)

    override fun observeContacts(): Flow<List<ContactSummary>> =
        flow {
            emit(dao.getAll().map { it.toSummary() })
        }

    override suspend fun findByPhone(phone: String): ContactSummary? =
        dao.findByPhoneDigits(phone)?.toSummary()

    override suspend fun findByEmail(email: String): ContactSummary? =
        dao.findByEmail(email)?.toSummary()

    override suspend fun setDeviceLookupKey(id: Int, lookupKey: String) {
        dao.setDeviceLookupKey(id, lookupKey)
    }

    override suspend fun getDeviceLookupKey(id: Int): String? =
        dao.getById(id)?.deviceLookupKey

    override suspend fun searchLocal(query: String): List<ContactSummary> {
        val trimmed = query.trim()
        if (trimmed.isEmpty()) return dao.getAll().map { it.toSummary() }
        return try {
            // T76: a phone-shaped query (mostly digits) is matched against the normalized
            // digit/key tokens of phonesNormalized rather than the raw-tokenizer path, which
            // splits "(800) 555-1234" into three FTS tokens that "8005551234" never
            // prefix-matches. Mirrors backend ContactFTSMatch's phone-vs-plain choice.
            val phoneQuery = PhoneKey.queryTokens(trimmed)
            if (phoneQuery != null) {
                return dao.searchFtsMatch(phoneMatchExpr(phoneQuery)).map { it.toSummary() }
            }
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
            dao.searchFts(safe).map { it.toSummary() }
        } catch (_: Exception) {
            // FTS rejected the expression after all — fall back to the LIKE scan.
            dao.search(trimmed).map { it.toSummary() }
        }
    }

    /**
     * Builds the FTS4 MATCH expression for a phone-shaped query, mirroring backend
     * `phoneFTSMatch`: an OR of prefix-matches on the query's normalized digit string and its
     * [PhoneKey.key] (deduped when they coincide), restricted to the `phonesNormalized` column
     * so a phone-shaped query can't accidentally match an unrelated text column's digits.
     *
     * Deliberately unquoted, unlike the backend's version — quoting a phrase breaks FTS4's
     * `column:term*` filter syntax entirely (`column:"term"*` matches nothing; confirmed
     * empirically, not documented). Safe without it: [PhoneKey.digits]/[PhoneKey.key] are
     * always pure `0`-`9` by construction, so there is no FTS syntax character to escape.
     */
    private fun phoneMatchExpr(query: PhoneKey.Query): String {
        val tokens = mutableListOf("phonesNormalized:${query.digits}*")
        if (query.key.isNotEmpty() && query.key != query.digits) {
            tokens.add("phonesNormalized:${query.key}*")
        }
        return tokens.joinToString(" OR ")
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
                // A list row only ever knows primaryPhone; preserve a richer multi-phone
                // index from a previously cached full detail fetch (cached.card != null,
                // same signal card/crm above key off) rather than downgrading it on every
                // plain list refresh.
                phonesNormalized = if (cached.card != null) cached.phonesNormalized else row.phonesNormalized,
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
        // A list row only ever knows the single primary phone; see phonesNormalized's
        // doc comment and mergePreservingDetail for how a richer index survives past it.
        phonesNormalized = primaryPhone?.let { PhoneKey.flatten(listOf(it)) },
        birthday = birthday,
        org = org,
        photoThumbnail = photoThumbnail,
        circles = circles,
        archived = archived,
        deleted = deleted,
        deviceLookupKey = deviceLookupKey,
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
        phonesNormalized = PhoneKey.flatten(card?.phones.orEmpty().map { it.number }),
        org = card?.organizations?.firstOrNull()?.name,
        photoThumbnail = photoThumbnail,
        circles = crm?.circles,
        archived = archived,
        card = card,
        crm = crm,
    )

    override suspend fun suggestContactAddresses(): Result<List<ContactAddressSuggestion>> =
        apiClient.suggestContactAddresses().map { it.suggestions }

    override suspend fun applyContactAddressSuggestion(input: ApplyContactAddressSuggestionInput): Result<Unit> =
        apiClient.applyContactAddressSuggestion(input)

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
        deviceLookupKey = deviceLookupKey,
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
