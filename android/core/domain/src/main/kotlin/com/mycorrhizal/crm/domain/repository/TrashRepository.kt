package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.TrashListResponse
import com.mycorrhizal.crm.model.network.TrashRestoreResponse

/**
 * Contact trash access (issue #2002, ADR 0037, mirroring web's useTrash).
 * Online-first and deliberately uncached: the trash is server state the user
 * acts on directly, and a restore must run against the live snapshot.
 */
interface TrashRepository {
    /** GET /trash/contacts — one page of restorable deleted contacts. */
    suspend fun list(page: Int = 1, limit: Int = 25): Result<TrashListResponse>

    /** POST /trash/contacts/:id/restore — 409 when a live contact already uses the UID. */
    suspend fun restore(id: Int): Result<TrashRestoreResponse>

    /** DELETE /trash/contacts/:id — permanently deletes one trashed contact. */
    suspend fun purge(id: Int): Result<Unit>

    /** DELETE /trash/contacts — empties the trash; returns the number purged. */
    suspend fun empty(): Result<Int>
}
