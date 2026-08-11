package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.domain.repository.BulkOperationRepository
import com.mycorrhizal.crm.domain.repository.MergeRepository
import com.mycorrhizal.crm.model.network.BulkContactOperationInput
import com.mycorrhizal.crm.model.network.BulkOperationResult
import com.mycorrhizal.crm.model.network.ContactMergePreviewResponse
import com.mycorrhizal.crm.model.network.ContactMergeRequest
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.network.ApiClient
import javax.inject.Inject

class MergeRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
) : MergeRepository {
    override suspend fun preview(request: ContactMergeRequest): Result<ContactMergePreviewResponse> =
        apiClient.previewMerge(request)

    override suspend fun commit(request: ContactMergeRequest): Result<ContactRecordResponse> =
        apiClient.commitMerge(request)
}

class BulkOperationRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
) : BulkOperationRepository {
    override suspend fun run(input: BulkContactOperationInput): Result<BulkOperationResult> =
        apiClient.bulkOperation(input)
}
