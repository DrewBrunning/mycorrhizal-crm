package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.data.local.CachedConversationAgenda
import com.mycorrhizal.crm.data.local.CachedConversationAgendaDao
import com.mycorrhizal.crm.data.local.CachedGift
import com.mycorrhizal.crm.data.local.CachedGiftDao
import com.mycorrhizal.crm.data.local.CachedLifeEvent
import com.mycorrhizal.crm.data.local.CachedLifeEventDao
import com.mycorrhizal.crm.data.local.CachedPreference
import com.mycorrhizal.crm.data.local.CachedPreferenceDao
import com.mycorrhizal.crm.domain.repository.ConversationAgendaRepository
import com.mycorrhizal.crm.domain.repository.GiftRepository
import com.mycorrhizal.crm.domain.repository.LifeEventRepository
import com.mycorrhizal.crm.domain.repository.PreferenceRepository
import com.mycorrhizal.crm.model.network.ConversationAgenda
import com.mycorrhizal.crm.model.network.ConversationAgendaInput
import com.mycorrhizal.crm.model.network.Gift
import com.mycorrhizal.crm.model.network.GiftInput
import com.mycorrhizal.crm.model.network.LifeEvent
import com.mycorrhizal.crm.model.network.LifeEventInput
import com.mycorrhizal.crm.model.network.Preference
import com.mycorrhizal.crm.model.network.PreferenceInput
import com.mycorrhizal.crm.network.ApiClient
import javax.inject.Inject

/**
 * Online-first access for the four contact-scoped timeline entities. Each list
 * is full-resync for the contact, so fetching replaces the cached rows; writes
 * mirror the returned record. All are user-authored content (soft delete
 * server-side), so the cache keeps tombstones for the `?since=` feed.
 */
class LifeEventRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
    private val dao: CachedLifeEventDao,
) : LifeEventRepository {
    override suspend fun listForContact(entityId: String): Result<List<LifeEvent>> {
        val result = apiClient.listLifeEvents(entityId = entityId)
        val page = result.getOrNull()
        if (page != null) {
            dao.deleteForContact(entityId)
            dao.upsertAll(page.lifeEvents.filterNot { it.deleted == true }.map { it.toCached(entityId) })
        }
        return result.map { page -> page.lifeEvents }
    }

    override suspend fun create(input: LifeEventInput): Result<LifeEvent> {
        val result = apiClient.createLifeEvent(input)
        result.getOrNull()?.let { dao.upsert(it.toCached(input.entityId)) }
        return result
    }

    override suspend fun update(id: String, input: LifeEventInput): Result<LifeEvent> {
        val result = apiClient.updateLifeEvent(id, input)
        result.getOrNull()?.let { dao.upsert(it.toCached(input.entityId)) }
        return result
    }

    override suspend fun delete(id: String): Result<Unit> {
        val result = apiClient.deleteLifeEvent(id)
        if (result.isSuccess) dao.deleteById(id)
        return result
    }

    private fun LifeEvent.toCached(entityId: String): CachedLifeEvent = CachedLifeEvent(
        id = id, entityId = entityId, type = type, category = category,
        date = date?.let { listOfNotNull(it.year, it.month, it.day).joinToString("-") },
        description = description, remind = remind, updatedAt = updatedAt,
    )
}

class GiftRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
    private val dao: CachedGiftDao,
) : GiftRepository {
    override suspend fun listForContact(entityId: String): Result<List<Gift>> {
        val result = apiClient.listGifts(entityId = entityId)
        val page = result.getOrNull()
        if (page != null) {
            dao.deleteForContact(entityId)
            dao.upsertAll(page.gifts.filterNot { it.deleted == true }.map { it.toCached(entityId) })
        }
        return result.map { page -> page.gifts }
    }

    override suspend fun create(input: GiftInput): Result<Gift> {
        val result = apiClient.createGift(input)
        result.getOrNull()?.let { dao.upsert(it.toCached(input.entityId)) }
        return result
    }

    override suspend fun update(id: String, input: GiftInput): Result<Gift> {
        val result = apiClient.updateGift(id, input)
        result.getOrNull()?.let { dao.upsert(it.toCached(input.entityId)) }
        return result
    }

    override suspend fun delete(id: String): Result<Unit> {
        val result = apiClient.deleteGift(id)
        if (result.isSuccess) dao.deleteById(id)
        return result
    }

    private fun Gift.toCached(entityId: String): CachedGift = CachedGift(
        id = id, entityId = entityId, status = status, occasion = occasion,
        description = description, updatedAt = updatedAt,
    )
}

class PreferenceRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
    private val dao: CachedPreferenceDao,
) : PreferenceRepository {
    override suspend fun listForContact(entityId: String): Result<List<Preference>> {
        val result = apiClient.listPreferences(entityId = entityId)
        val page = result.getOrNull()
        if (page != null) {
            dao.deleteForContact(entityId)
            dao.upsertAll(page.preferences.filterNot { it.deleted == true }.map { it.toCached(entityId) })
        }
        return result.map { page -> page.preferences }
    }

    override suspend fun create(input: PreferenceInput): Result<Preference> {
        val result = apiClient.createPreference(input)
        result.getOrNull()?.let { dao.upsert(it.toCached(input.entityId)) }
        return result
    }

    override suspend fun update(id: String, input: PreferenceInput): Result<Preference> {
        val result = apiClient.updatePreference(id, input)
        result.getOrNull()?.let { dao.upsert(it.toCached(input.entityId)) }
        return result
    }

    override suspend fun delete(id: String): Result<Unit> {
        val result = apiClient.deletePreference(id)
        if (result.isSuccess) dao.deleteById(id)
        return result
    }

    private fun Preference.toCached(entityId: String): CachedPreference = CachedPreference(
        id = id, entityId = entityId, category = category, key = key,
        value = value, sensitivity = sensitivity, updatedAt = updatedAt,
    )
}

class ConversationAgendaRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
    private val dao: CachedConversationAgendaDao,
) : ConversationAgendaRepository {
    override suspend fun listForContact(entityId: String): Result<List<ConversationAgenda>> {
        val result = apiClient.listConversationAgenda(entityId = entityId)
        val page = result.getOrNull()
        if (page != null) {
            dao.deleteForContact(entityId)
            dao.upsertAll(page.conversationAgenda.filterNot { it.deleted == true }.map { it.toCached(entityId) })
        }
        return result.map { page -> page.conversationAgenda }
    }

    override suspend fun create(input: ConversationAgendaInput): Result<ConversationAgenda> {
        val result = apiClient.createConversationAgenda(input)
        result.getOrNull()?.let { dao.upsert(it.toCached(input.entityId)) }
        return result
    }

    override suspend fun update(id: String, input: ConversationAgendaInput): Result<ConversationAgenda> {
        val result = apiClient.updateConversationAgenda(id, input)
        result.getOrNull()?.let { dao.upsert(it.toCached(input.entityId)) }
        return result
    }

    override suspend fun delete(id: String): Result<Unit> {
        val result = apiClient.deleteConversationAgenda(id)
        if (result.isSuccess) dao.deleteById(id)
        return result
    }

    override suspend fun discuss(id: String, activityId: Int?): Result<ConversationAgenda> {
        val result = apiClient.discussConversationAgenda(id, activityId)
        result.getOrNull()?.let { dao.upsert(it.toCached(it.entityId)) }
        return result
    }

    private fun ConversationAgenda.toCached(entityId: String): CachedConversationAgenda =
        CachedConversationAgenda(
            id = id, entityId = entityId, content = content,
            referenceUrl = referenceUrl, discussedAt = discussedAt, updatedAt = updatedAt,
        )
}
