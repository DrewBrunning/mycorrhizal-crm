package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.data.local.PendingInteractionDao
import com.mycorrhizal.crm.domain.repository.PendingInteraction as DomainPendingInteraction
import com.mycorrhizal.crm.domain.repository.PendingInteractionRepository
import javax.inject.Inject

class PendingInteractionRepositoryImpl @Inject constructor(
    private val dao: PendingInteractionDao,
) : PendingInteractionRepository {
    override suspend fun record(interaction: DomainPendingInteraction) {
        dao.insert(
            com.mycorrhizal.crm.data.local.PendingInteraction(
                id = interaction.id,
                timestampMillis = interaction.timestampMillis,
                kind = interaction.kind,
                direction = interaction.direction,
                phoneNumber = interaction.phoneNumber,
                matchedContactId = interaction.matchedContactId,
                synced = interaction.synced,
                syncedAt = interaction.syncedAt,
            ),
        )
    }

    override suspend fun unsynced(): List<DomainPendingInteraction> =
        dao.getUnsynced().map { it.toDomain() }

    override suspend fun markSynced(id: Long, syncedAt: String) {
        dao.markSynced(id, syncedAt)
    }

    override suspend fun deleteSynced() {
        dao.deleteSynced()
    }
}

private fun com.mycorrhizal.crm.data.local.PendingInteraction.toDomain(): DomainPendingInteraction =
    DomainPendingInteraction(
        id = id,
        timestampMillis = timestampMillis,
        kind = kind,
        direction = direction,
        phoneNumber = phoneNumber,
        matchedContactId = matchedContactId,
        synced = synced,
        syncedAt = syncedAt,
    )
