package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.domain.repository.ExportRepository
import com.mycorrhizal.crm.model.network.ExportLossPreflightResponse
import com.mycorrhizal.crm.network.ApiClient
import javax.inject.Inject

class ExportRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
) : ExportRepository {
    override suspend fun exportDataCsv(): Result<ByteArray> =
        apiClient.exportDataCsv()

    override suspend fun exportContactsVcf(
        version: Int?,
        sections: List<String>?,
        includeSensitive: Boolean,
    ): Result<ByteArray> =
        apiClient.exportAllContactsVcf(version, sections, includeSensitive)

    override suspend fun exportContactsJsContact(
        sections: List<String>?,
        includeSensitive: Boolean,
    ): Result<ByteArray> =
        apiClient.exportAllContactsJsContact(sections, includeSensitive)

    override suspend fun exportAuditLogCsv(): Result<ByteArray> =
        apiClient.exportAuditLogCsv()

    override suspend fun preflight(
        format: String,
        sections: List<String>,
        includeSensitive: Boolean,
    ): Result<ExportLossPreflightResponse> =
        apiClient.exportPreflight(format, sections, includeSensitive)
}
