package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.IncomingShareGrant
import com.mycorrhizal.crm.model.network.IncomingShareGrantsPage
import com.mycorrhizal.crm.model.network.OutgoingShareGrant
import com.mycorrhizal.crm.model.network.OutgoingShareGrantsPage
import com.mycorrhizal.crm.model.network.ShareBlock
import com.mycorrhizal.crm.model.network.ShareGrantContent
import com.mycorrhizal.crm.model.network.ShareGrantInput
import com.mycorrhizal.crm.model.network.ShareGrantNotifications
import com.mycorrhizal.crm.model.network.ShareGrantPatch

/**
 * Standing-share data access (ADR 0042, issue #2293). Online-only by design: a grant stores
 * authorization, and the recipient's content is read through per request with `no-store`
 * semantics. Nothing here is mirrored into Room, so a revoked grant disappears on the next load
 * and a failed read is an error, never stale content.
 */
interface ShareGrantRepository {
    /** Grants offered to me (pending and active only). */
    suspend fun listIncoming(
        cursor: String? = null,
        limit: Int = 100
    ): Result<IncomingShareGrantsPage>

    /** Grants I have issued, any status. */
    suspend fun listOutgoing(
        cursor: String? = null,
        limit: Int = 100
    ): Result<OutgoingShareGrantsPage>

    /** Issue a standing grant; it starts `pending` until the recipient accepts. */
    suspend fun create(input: ShareGrantInput): Result<OutgoingShareGrant>

    /** Owner-only: change the expiry and/or narrow the sections. [revision] rides in `If-Match`. */
    suspend fun update(
        id: String,
        patch: ShareGrantPatch,
        revision: Int?
    ): Result<OutgoingShareGrant>

    suspend fun revoke(id: String): Result<OutgoingShareGrant>

    suspend fun reconfirm(id: String): Result<OutgoingShareGrant>

    suspend fun accept(id: String): Result<IncomingShareGrant>

    /** Decline a pending grant; [block] also blocks the sender and ends all their grants. */
    suspend fun decline(id: String, block: Boolean = false): Result<IncomingShareGrant>

    /** Stop receiving a pending or active grant. */
    suspend fun leave(id: String): Result<IncomingShareGrant>

    /** The granted entity's allowlisted fields, resolved per request. */
    suspend fun content(id: String): Result<ShareGrantContent>

    suspend fun blocks(): Result<List<ShareBlock>>

    suspend fun unblock(userId: Long): Result<Unit>

    /** The in-app pending badge and unread notices. */
    suspend fun notifications(): Result<ShareGrantNotifications>

    suspend fun markNotificationsRead(): Result<Unit>
}
