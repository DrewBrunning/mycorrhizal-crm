package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.ConversationAgenda
import com.mycorrhizal.crm.model.network.ConversationAgendaInput
import com.mycorrhizal.crm.model.network.Gift
import com.mycorrhizal.crm.model.network.GiftInput
import com.mycorrhizal.crm.model.network.LifeEvent
import com.mycorrhizal.crm.model.network.LifeEventInput
import com.mycorrhizal.crm.model.network.Preference
import com.mycorrhizal.crm.model.network.PreferenceInput

/**
 * Contact-scoped timeline entities (item 18): life events, gifts, preferences,
 * and conversation agenda. All four follow the identical online-first pattern:
 * list by entity VCardUID, create/update/delete, mirror into the Room cache.
 * All use UUID string PKs; `entity_id` is the owning Contact's VCardUID.
 */
interface LifeEventRepository {
    suspend fun listForContact(entityId: String): Result<List<LifeEvent>>
    suspend fun create(input: LifeEventInput): Result<LifeEvent>
    suspend fun update(id: String, input: LifeEventInput): Result<LifeEvent>
    suspend fun delete(id: String): Result<Unit>
}

interface GiftRepository {
    suspend fun listForContact(entityId: String): Result<List<Gift>>
    suspend fun create(input: GiftInput): Result<Gift>
    suspend fun update(id: String, input: GiftInput): Result<Gift>
    suspend fun delete(id: String): Result<Unit>
}

interface PreferenceRepository {
    suspend fun listForContact(entityId: String): Result<List<Preference>>
    suspend fun create(input: PreferenceInput): Result<Preference>
    suspend fun update(id: String, input: PreferenceInput): Result<Preference>
    suspend fun delete(id: String): Result<Unit>
}

interface ConversationAgendaRepository {
    suspend fun listForContact(entityId: String): Result<List<ConversationAgenda>>
    suspend fun create(input: ConversationAgendaInput): Result<ConversationAgenda>
    suspend fun update(id: String, input: ConversationAgendaInput): Result<ConversationAgenda>
    suspend fun delete(id: String): Result<Unit>
    /** Marks an item discussed (PATCH discuss). */
    suspend fun discuss(id: String): Result<ConversationAgenda>
}
