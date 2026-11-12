package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.GiftShoppingItem
import com.mycorrhizal.crm.model.network.OccasionObligation
import com.mycorrhizal.crm.model.network.OccasionObligationInput

/**
 * Standing occasion-obligation access (docs/adrs/0024-occasions.md, issue #2521). Online-only:
 * obligations drive server-side reminders, so every read and write goes to the server.
 */
interface OccasionObligationRepository {
    /** Every live obligation for one contact (`Contact.VCardUID`), all pages. */
    suspend fun listForContact(entityId: String): Result<List<OccasionObligation>>

    suspend fun create(input: OccasionObligationInput): Result<OccasionObligation>

    /** Full replace. */
    suspend fun update(id: String, input: OccasionObligationInput): Result<OccasionObligation>

    suspend fun delete(id: String): Result<Unit>

    /** Upcoming gift obligations within [days] (30 or 90) joined to this cycle's gift status. */
    suspend fun giftShoppingList(days: Int): Result<List<GiftShoppingItem>>

    /** The card-list CSV for obligations of [kind]; above-normal addresses are left out. */
    suspend fun cardListCsv(kind: String): Result<ByteArray>
}
