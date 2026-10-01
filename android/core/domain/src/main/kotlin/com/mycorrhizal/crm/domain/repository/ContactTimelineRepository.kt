package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.ContactTimelinePage

/**
 * Online-only access to a contact's merged, cursor-paginated timeline
 * (`GET /contacts/{id}/timeline`, T66) — the full "View all" explorer behind
 * the contact page's bounded 5-event preview (issue #1401, web T78 parity).
 * No Room mirror: it is a filtered, server-ranked read view over six tables.
 */
interface ContactTimelineRepository {
    /**
     * One page. [types] empty (or all six) means no type filter; [bucket] is a recency token
     * (`null`/`all` means no filter); [cursor] is the previous page's `nextCursor`.
     */
    suspend fun page(
        contactId: Int,
        types: Collection<String> = emptyList(),
        bucket: String? = null,
        cursor: String? = null,
        limit: Int = 25,
    ): Result<ContactTimelinePage>
}
