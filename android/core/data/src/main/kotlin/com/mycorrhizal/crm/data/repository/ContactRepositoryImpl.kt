package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.data.local.CachedContact
import com.mycorrhizal.crm.data.local.CachedContactDao
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.ContactsPage
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
        val rows = page.contacts.map { it.toCached() }
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
                // Offline / server error: try the cache.
                val cached = dao.getById(id)
                val fromCache = cached?.let { it.toRecord() }
                if (fromCache != null) Result.success(fromCache)
                else Result.failure(error.toApiError())
            },
        )
    }

    override fun observeContacts(): Flow<List<ContactSummary>> =
        flow {
            emit(dao.getAll().map { it.toSummary() })
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
