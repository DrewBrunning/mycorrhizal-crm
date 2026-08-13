package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.data.local.CachedContactTag
import com.mycorrhizal.crm.data.local.CachedContactTagDao
import com.mycorrhizal.crm.data.local.CachedTag
import com.mycorrhizal.crm.data.local.CachedTagDao
import com.mycorrhizal.crm.domain.repository.TagDetail
import com.mycorrhizal.crm.domain.repository.TagRepository
import com.mycorrhizal.crm.model.network.ContactTag
import com.mycorrhizal.crm.model.network.ContactTagInput
import com.mycorrhizal.crm.model.network.Tag
import com.mycorrhizal.crm.model.network.TagInput
import com.mycorrhizal.crm.network.ApiClient
import javax.inject.Inject

/**
 * Online-first tag access. Writes go to the server; successful responses are
 * mirrored into the Room cache. Tagging join rows hard-delete.
 */
class TagRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
    private val tagDao: CachedTagDao,
    private val contactTagDao: CachedContactTagDao,
) : TagRepository {

    override suspend fun list(cursor: String?, limit: Int): Result<List<Tag>> {
        val result = apiClient.listTags(cursor = cursor, limit = limit)
        val page = result.getOrNull()
        if (page != null) {
            tagDao.upsertAll(page.tags.map { it.toCached() })
        }
        return result.map { page -> page.tags }
    }

    override suspend fun tagsForContact(vcardUid: String): Result<List<Tag>> {
        if (vcardUid.isBlank()) return Result.success(emptyList())
        // 100 is the backend's maxLimit (the default is 25 — omitting it would silently
        // truncate a user with >25 tags and miss the contact's later tags), matching web's
        // own listTags default. A user with >100 tags is a documented truncation.
        val result = apiClient.listTags(includeContacts = true, limit = 100)
        if (result.isFailure) return result.map { it.tags }
        val page = result.getOrThrow()
        tagDao.upsertAll(page.tags.map { it.toCached() })
        // Full snapshot — replace the tagging mirror wholesale rather than merging, so a
        // removed tagging doesn't linger in the cache (join rows hard-delete server-side).
        contactTagDao.deleteAll()
        contactTagDao.upsertAll(page.contacts.orEmpty().map { it.toCached() })
        val tagIds = page.contacts.orEmpty()
            .filter { it.contactVCardUid == vcardUid }
            .map { it.tagId }
            .toSet()
        return Result.success(page.tags.filter { it.id in tagIds })
    }

    override suspend fun getWithContacts(id: String): Result<TagDetail> {
        val result = apiClient.getTag(id)
        val detail = result.getOrNull()
        val tag = detail?.tag
        if (tag != null) {
            tagDao.upsert(tag.toCached())
            val contacts = detail.contacts.orEmpty()
            contactTagDao.deleteByTagId(id)
            contactTagDao.upsertAll(contacts.map { it.toCached() })
            return Result.success(TagDetail(tag = tag, contacts = contacts))
        }
        return result.map { d ->
            TagDetail(tag = d.tag ?: Tag(), contacts = d.contacts.orEmpty())
        }
    }

    override suspend fun create(name: String): Result<Tag> {
        val result = apiClient.createTag(TagInput(name = name))
        result.getOrNull()?.let { tagDao.upsert(it.toCached()) }
        return result
    }

    override suspend fun rename(id: String, name: String): Result<Tag> {
        val result = apiClient.updateTag(id, TagInput(name = name))
        result.getOrNull()?.let { tagDao.upsert(it.toCached()) }
        return result
    }

    override suspend fun delete(id: String): Result<Unit> {
        val result = apiClient.deleteTag(id)
        if (result.isSuccess) {
            tagDao.deleteById(id)
            contactTagDao.deleteByTagId(id)
        }
        return result
    }

    override suspend fun addContact(tagId: String, vcardUid: String): Result<ContactTag> {
        val result = apiClient.addContactTag(tagId, ContactTagInput(contactVCardUid = vcardUid))
        result.getOrNull()?.let { contactTagDao.upsertAll(listOf(it.toCached())) }
        return result
    }

    override suspend fun removeContact(tagId: String, vcardUid: String): Result<Unit> {
        val result = apiClient.removeContactTag(tagId, vcardUid)
        if (result.isSuccess) {
            contactTagDao.deleteTagging(tagId, vcardUid)
        }
        return result
    }

    private fun Tag.toCached(): CachedTag = CachedTag(
        id = id,
        name = name,
        updatedAt = updatedAt,
    )

    private fun ContactTag.toCached(): CachedContactTag = CachedContactTag(
        id = id,
        tagId = tagId,
        contactVCardUid = contactVCardUid,
    )
}
