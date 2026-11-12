package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.domain.repository.OccasionObligationRepository
import com.mycorrhizal.crm.model.network.GiftShoppingItem
import com.mycorrhizal.crm.model.network.OccasionObligation
import com.mycorrhizal.crm.model.network.OccasionObligationInput
import com.mycorrhizal.crm.network.ApiClient
import javax.inject.Inject

/** Online-only occasion-obligation access (docs/adrs/0024-occasions.md, issue #2521). */
class OccasionObligationRepositoryImpl @Inject constructor(private val apiClient: ApiClient) :
    OccasionObligationRepository {

    override suspend fun listForContact(entityId: String): Result<List<OccasionObligation>> {
        val all = mutableListOf<OccasionObligation>()
        var cursor: String? = null
        val seen = mutableSetOf<String>()
        while (true) {
            val page = apiClient.listOccasionObligations(entityId, cursor, PAGE_LIMIT)
                .getOrElse { return Result.failure(it) }
            all += page.occasionObligations.filterNot { it.deleted }
            val next = page.nextCursor?.takeIf { it.isNotEmpty() } ?: break
            // A server echoing the same cursor would otherwise loop forever.
            if (!seen.add(next)) break
            cursor = next
        }
        return Result.success(all)
    }

    override suspend fun create(input: OccasionObligationInput): Result<OccasionObligation> =
        apiClient.createOccasionObligation(input)

    override suspend fun update(
        id: String,
        input: OccasionObligationInput
    ): Result<OccasionObligation> = apiClient.updateOccasionObligation(id, input)

    override suspend fun delete(id: String): Result<Unit> = apiClient.deleteOccasionObligation(id)

    override suspend fun giftShoppingList(days: Int): Result<List<GiftShoppingItem>> =
        apiClient.getGiftShoppingList(days).map { it.items }

    override suspend fun cardListCsv(kind: String): Result<ByteArray> =
        apiClient.exportOccasionCardListCsv(kind)

    private companion object {
        const val PAGE_LIMIT = 100
    }
}
