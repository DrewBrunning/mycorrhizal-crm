package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.ContactEntityLink
import com.mycorrhizal.crm.model.network.ContactEntityLinkAdoptResponse
import com.mycorrhizal.crm.model.network.ContactEntityLinkFromExistingInput
import com.mycorrhizal.crm.model.network.ContactEntityLinkFromExistingResponse
import com.mycorrhizal.crm.model.network.ContactEntityLinkInput
import com.mycorrhizal.crm.model.network.ContactFieldValuesInput
import com.mycorrhizal.crm.model.network.Entity
import com.mycorrhizal.crm.model.network.EntityContactLink
import com.mycorrhizal.crm.model.network.EntityDeleteResponse
import com.mycorrhizal.crm.model.network.EntityInput
import com.mycorrhizal.crm.model.network.EntityListResponse
import com.mycorrhizal.crm.model.network.EntityMergeInput
import com.mycorrhizal.crm.model.network.EntityMergeResponse
import com.mycorrhizal.crm.model.network.EntityRestoreResponse
import com.mycorrhizal.crm.model.network.FieldValue

/**
 * Reusable entities - organisations, places, pets (issue #2302, ADR 0043). Online-first and
 * deliberately without a Room mirror of the entities themselves: they are small, user-curated
 * lists read on demand. What IS cached is the contact record, which carries the server's
 * read-time projection (`ent-<linkId>` ORG/ADR), so every write here that moves a projection
 * re-fetches the affected contacts through [ContactRepository.getContact] (which upserts the
 * cache), keeping a cached contact from diverging from the server after a link change.
 */
interface EntityRepository {
    suspend fun list(
        kind: String?,
        query: String?,
        cursor: String?,
        limit: Int? = null
    ): Result<EntityListResponse>

    suspend fun get(id: String): Result<Entity>

    suspend fun create(input: EntityInput): Result<Entity>

    suspend fun update(id: String, input: EntityInput, revision: Int?): Result<Entity>

    suspend fun delete(id: String, revision: Int?): Result<EntityDeleteResponse>

    suspend fun restore(id: String): Result<EntityRestoreResponse>

    suspend fun merge(loserId: String, input: EntityMergeInput): Result<EntityMergeResponse>

    /** The live links of an entity (the "People" list). */
    suspend fun contacts(id: String): Result<List<EntityContactLink>>

    suspend fun fieldValues(id: String): Result<List<FieldValue>>

    suspend fun replaceFieldValues(
        id: String,
        input: ContactFieldValuesInput
    ): Result<List<FieldValue>>

    suspend fun linksForContact(contactId: Int): Result<List<ContactEntityLink>>

    suspend fun createLink(contactId: Int, input: ContactEntityLinkInput): Result<ContactEntityLink>

    suspend fun updateLink(
        contactId: Int,
        linkId: String,
        input: ContactEntityLinkInput,
        revision: Int?
    ): Result<ContactEntityLink>

    suspend fun deleteLink(contactId: Int, linkId: String): Result<Unit>

    suspend fun adopt(contactId: Int, linkId: String): Result<ContactEntityLinkAdoptResponse>

    suspend fun linkFromExisting(
        contactId: Int,
        input: ContactEntityLinkFromExistingInput
    ): Result<ContactEntityLinkFromExistingResponse>
}
