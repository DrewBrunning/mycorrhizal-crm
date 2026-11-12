package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.EntityRepository
import com.mycorrhizal.crm.model.network.ContactEntityLink
import com.mycorrhizal.crm.model.network.ContactEntityLinkAdoptResponse
import com.mycorrhizal.crm.model.network.ContactEntityLinkFromExistingInput
import com.mycorrhizal.crm.model.network.ContactEntityLinkFromExistingResponse
import com.mycorrhizal.crm.model.network.ContactEntityLinkInput
import com.mycorrhizal.crm.model.network.ContactFieldValuesInput
import com.mycorrhizal.crm.model.network.Entity
import com.mycorrhizal.crm.model.network.EntityContactLink
import com.mycorrhizal.crm.model.network.EntityDeleteResponse
import com.mycorrhizal.crm.model.network.EntityFromContactsInput
import com.mycorrhizal.crm.model.network.EntityFromContactsResponse
import com.mycorrhizal.crm.model.network.EntityInput
import com.mycorrhizal.crm.model.network.EntityListResponse
import com.mycorrhizal.crm.model.network.EntityMergeInput
import com.mycorrhizal.crm.model.network.EntityMergeResponse
import com.mycorrhizal.crm.model.network.EntityRestoreResponse
import com.mycorrhizal.crm.model.network.EntitySuggestionsResponse
import com.mycorrhizal.crm.model.network.FieldValue
import com.mycorrhizal.crm.network.ApiClient
import javax.inject.Inject

/**
 * Online-first entity access (issue #2302, ADR 0043). See [EntityRepository] for the cache
 * policy: entities are not mirrored, but every write that changes a contact's projection
 * re-fetches the affected contacts so the cached record never keeps a stale `ent-` ORG/ADR.
 */
class EntityRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
    private val contactRepository: ContactRepository
) : EntityRepository {

    override suspend fun list(
        kind: String?,
        query: String?,
        cursor: String?,
        limit: Int?
    ): Result<EntityListResponse> = apiClient.listEntities(
        kind = kind,
        q = query,
        cursor = cursor,
        limit = limit
    )

    override suspend fun get(id: String): Result<Entity> = apiClient.getEntity(id)

    override suspend fun create(input: EntityInput): Result<Entity> = apiClient.createEntity(input)

    override suspend fun update(id: String, input: EntityInput, revision: Int?): Result<Entity> {
        val affected = linkedContactIds(id)
        return apiClient.updateEntity(id, input, revision).also {
            if (it.isSuccess) refresh(affected)
        }
    }

    override suspend fun delete(id: String, revision: Int?): Result<EntityDeleteResponse> {
        val affected = linkedContactIds(id)
        return apiClient.deleteEntity(id, revision).also {
            if (it.isSuccess) refresh(affected)
        }
    }

    override suspend fun restore(id: String): Result<EntityRestoreResponse> =
        apiClient.restoreEntity(id).also {
            if (it.isSuccess) refresh(linkedContactIds(id))
        }

    override suspend fun merge(
        loserId: String,
        input: EntityMergeInput
    ): Result<EntityMergeResponse> {
        // The loser's links are re-pointed to the survivor: those contacts now project the
        // survivor's value, so their cached records are the ones that went stale.
        val affected = linkedContactIds(loserId)
        return apiClient.mergeEntity(loserId, input).also {
            if (it.isSuccess) refresh(affected)
        }
    }

    override suspend fun suggestions(kind: String): Result<EntitySuggestionsResponse> =
        apiClient.listEntitySuggestions(kind)

    override suspend fun valueGroups(kind: String, cursor: String?): Result<EntitySuggestionsResponse> =
        apiClient.listEntityValueGroups(kind, cursor)

    override suspend fun createFromContacts(
        input: EntityFromContactsInput
    ): Result<EntityFromContactsResponse> = apiClient.createEntitiesFromContacts(input).also {
        val response = it.getOrNull() ?: return@also
        val createdKeys = response.createdOrEmpty.map { c -> c.key }.toSet()
        refresh(
            input.groups.filter { g -> g.key in createdKeys }.flatMap { g -> g.contactIds }.distinct()
        )
    }

    override suspend fun dismissSuggestion(entityA: String, entityB: String): Result<Unit> =
        apiClient.dismissEntitySuggestion(entityA, entityB).map { }

    override suspend fun contacts(id: String): Result<List<EntityContactLink>> =
        apiClient.listEntityContacts(id)

    override suspend fun fieldValues(id: String): Result<List<FieldValue>> =
        apiClient.listEntityFieldValues(id).map { it.values }

    override suspend fun replaceFieldValues(
        id: String,
        input: ContactFieldValuesInput
    ): Result<List<FieldValue>> = apiClient.replaceEntityFieldValues(id, input).map { it.values }

    override suspend fun linksForContact(contactId: Int): Result<List<ContactEntityLink>> =
        apiClient.listContactEntityLinks(contactId)

    override suspend fun createLink(
        contactId: Int,
        input: ContactEntityLinkInput
    ): Result<ContactEntityLink> = apiClient.createContactEntityLink(contactId, input).also {
        if (it.isSuccess) refresh(listOf(contactId))
    }

    override suspend fun updateLink(
        contactId: Int,
        linkId: String,
        input: ContactEntityLinkInput,
        revision: Int?
    ): Result<ContactEntityLink> =
        apiClient.updateContactEntityLink(contactId, linkId, input, revision).also {
            if (it.isSuccess) refresh(listOf(contactId))
        }

    override suspend fun deleteLink(contactId: Int, linkId: String): Result<Unit> =
        apiClient.deleteContactEntityLink(contactId, linkId).also {
            if (it.isSuccess) refresh(listOf(contactId))
        }

    override suspend fun adopt(
        contactId: Int,
        linkId: String
    ): Result<ContactEntityLinkAdoptResponse> =
        apiClient.adoptContactEntityLink(contactId, linkId).also {
            if (it.isSuccess) refresh(listOf(contactId))
        }

    override suspend fun linkFromExisting(
        contactId: Int,
        input: ContactEntityLinkFromExistingInput
    ): Result<ContactEntityLinkFromExistingResponse> =
        apiClient.linkContactEntityFromExisting(contactId, input).also {
            if (it.isSuccess) refresh(listOf(contactId))
        }

    /** Contact ids currently linked to [entityId]; empty on any failure (best effort). */
    private suspend fun linkedContactIds(entityId: String): List<Int> =
        apiClient.listEntityContacts(entityId).getOrNull().orEmpty()
            .map { it.contactId }.distinct()

    /**
     * Re-fetch [contactIds] so the contact cache picks up the server's current projection.
     * Best effort and bounded: a failure leaves the (still correct) write result alone, and a
     * huge entity only refreshes the first [MAX_REFRESHED_CONTACTS]; the rest refresh the next
     * time they are opened.
     */
    private suspend fun refresh(contactIds: List<Int>) {
        contactIds.take(MAX_REFRESHED_CONTACTS).forEach { contactRepository.getContact(it) }
    }

    companion object {
        const val MAX_REFRESHED_CONTACTS = 50
    }
}
