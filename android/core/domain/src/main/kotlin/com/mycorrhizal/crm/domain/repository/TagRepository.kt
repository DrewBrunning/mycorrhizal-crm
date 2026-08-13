package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.ContactTag
import com.mycorrhizal.crm.model.network.Tag

/**
 * Tag data access. Online-first: writes go to the server and the returned
 * records are mirrored into the local cache. Tags use UUID string PKs.
 */
interface TagRepository {
    /** All tags, cursor-paginated. */
    suspend fun list(cursor: String? = null, limit: Int = 100): Result<List<Tag>>

    /**
     * The tags applied to a given contact, derived from the taggings (`GET
     * /tags?include_contacts=true` filtered client-side by VCard UID). The
     * contact detail payload carries no tags, so M24's inline editor derives
     * them from the taggings; the snapshot also refreshes the local tag/tagging
     * mirror wholesale. Empty [vcardUid] short-circuits to success with no tags
     * (no network call).
     */
    suspend fun tagsForContact(vcardUid: String): Result<List<Tag>>

    /** A tag with its tagged contacts. */
    suspend fun getWithContacts(id: String): Result<TagDetail>

    /** Create a tag; returns the created tag. */
    suspend fun create(name: String): Result<Tag>

    /** Rename a tag; returns the updated tag. */
    suspend fun rename(id: String, name: String): Result<Tag>

    /** Delete a tag (hard delete). */
    suspend fun delete(id: String): Result<Unit>

    /** Tag a contact (by VCard UID). */
    suspend fun addContact(tagId: String, vcardUid: String): Result<ContactTag>

    /** Remove a contact's tagging (hard delete join row). */
    suspend fun removeContact(tagId: String, vcardUid: String): Result<Unit>
}

data class TagDetail(
    val tag: Tag,
    val contacts: List<ContactTag>,
)
