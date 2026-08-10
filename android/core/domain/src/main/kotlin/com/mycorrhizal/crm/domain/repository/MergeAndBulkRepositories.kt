package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.BulkContactOperationInput
import com.mycorrhizal.crm.model.network.BulkOperationResult
import com.mycorrhizal.crm.model.network.ContactMergePreviewResponse
import com.mycorrhizal.crm.model.network.ContactMergeRequest
import com.mycorrhizal.crm.model.network.ContactRecordResponse

/**
 * Contact operations beyond single-entity CRUD: merge two contacts (preview
 * then commit with conflict resolutions) and bulk actions across many
 * contacts (archive/unarchive/delete/circle/tag membership).
 */
interface MergeRepository {
    suspend fun preview(request: ContactMergeRequest): Result<ContactMergePreviewResponse>
    suspend fun commit(request: ContactMergeRequest): Result<ContactRecordResponse>
}

interface BulkOperationRepository {
    suspend fun run(input: BulkContactOperationInput): Result<BulkOperationResult>
}
