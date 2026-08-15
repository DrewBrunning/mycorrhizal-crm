package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.ContactShare
import com.mycorrhizal.crm.model.network.ContactShareInput
import com.mycorrhizal.crm.model.network.ContactSharesPage
import com.mycorrhizal.crm.model.network.ImportPreviewResponse
import com.mycorrhizal.crm.model.network.ImportResult
import com.mycorrhizal.crm.model.network.RowImportAction
import com.mycorrhizal.crm.model.network.UserDirectoryEntry

/**
 * Contact-share data access (P1). Online-first: a share is a frozen snapshot
 * between two users — the
 * inbox/outbox lists and the accept preview are inherently online data with
 * no offline value, so nothing is mirrored into the Room cache (the web
 * client doesn't cache them either).
 */
interface ContactShareRepository {
    /** Incoming shares (offered to me), cursor-paginated, with the sender's usernames. */
    suspend fun listIncoming(cursor: String? = null, limit: Int = 100): Result<ContactSharesPage>

    /** Outgoing shares (offered by me), cursor-paginated, with the recipient's usernames. */
    suspend fun listOutgoing(cursor: String? = null, limit: Int = 100): Result<ContactSharesPage>

    /** Offer a one-time filtered copy of a contact to another user. */
    suspend fun create(input: ContactShareInput): Result<ContactShare>

    /**
     * PREVIEW-ONLY: parse the share's payload through the import pipeline and
     * return the duplicate matches. Does NOT change the share's status.
     */
    suspend fun accept(id: String): Result<ImportPreviewResponse>

    /** Finalize an accepted share with the recipient's per-row choices. */
    suspend fun confirm(id: String, sessionId: String, actions: List<RowImportAction>): Result<ImportResult>

    /** Decline a pending share (flips its status; the sender's offer survives). */
    suspend fun decline(id: String): Result<Unit>

    /** Every other user on the instance (id + username), for the recipient picker. */
    suspend fun userDirectory(): Result<List<UserDirectoryEntry>>
}
