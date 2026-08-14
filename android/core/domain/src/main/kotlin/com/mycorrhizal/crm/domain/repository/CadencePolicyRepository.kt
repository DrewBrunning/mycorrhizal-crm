package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.CadencePolicy
import com.mycorrhizal.crm.model.network.CadencePolicyInput

/**
 * Cadence/relationship-health policy data access (T19). Online-first: writes
 * go to the server and the returned record is mirrored into the local cache.
 * A contact has at most one policy — the server rejects a second with a 409.
 */
interface CadencePolicyRepository {
    /** A contact's cadence policies (0 or 1), filtered by entity_id (Contact.VCardUID). */
    suspend fun listForContact(entityId: String): Result<List<CadencePolicy>>

    /** A single policy with its server-derived health. */
    suspend fun get(id: String): Result<CadencePolicy>

    /** Create a policy on a contact; returns the created policy (with health). */
    suspend fun create(input: CadencePolicyInput): Result<CadencePolicy>

    /** Update a policy; returns the updated policy (with health). */
    suspend fun update(id: String, input: CadencePolicyInput): Result<CadencePolicy>

    /** Delete a policy (soft delete server-side). */
    suspend fun delete(id: String): Result<Unit>
}
