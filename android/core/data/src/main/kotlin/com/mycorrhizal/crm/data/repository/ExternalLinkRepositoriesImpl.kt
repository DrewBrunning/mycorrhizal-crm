package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.domain.repository.ExternalIdentityRepository
import com.mycorrhizal.crm.domain.repository.ImmichRepository
import com.mycorrhizal.crm.domain.repository.NextcloudRepository
import com.mycorrhizal.crm.domain.repository.PaperlessRepository
import com.mycorrhizal.crm.domain.repository.SeafileRepository
import com.mycorrhizal.crm.model.network.ExternalIdentity
import com.mycorrhizal.crm.model.network.ImmichAssetSummary
import com.mycorrhizal.crm.model.network.ImmichConfigInput
import com.mycorrhizal.crm.model.network.ImmichConfigResponse
import com.mycorrhizal.crm.model.network.ImmichConnectionTestResult
import com.mycorrhizal.crm.model.network.ImmichPerson
import com.mycorrhizal.crm.model.network.ImmichPersonSummary
import com.mycorrhizal.crm.model.network.NextcloudConfigInput
import com.mycorrhizal.crm.model.network.NextcloudConfigResponse
import com.mycorrhizal.crm.model.network.NextcloudConnectionTestResult
import com.mycorrhizal.crm.model.network.NextcloudLinkRequest
import com.mycorrhizal.crm.model.network.PaperlessConfigInput
import com.mycorrhizal.crm.model.network.PaperlessConfigResponse
import com.mycorrhizal.crm.model.network.PaperlessConnectionTestResult
import com.mycorrhizal.crm.model.network.PaperlessDocument
import com.mycorrhizal.crm.model.network.SeafileConfigInput
import com.mycorrhizal.crm.model.network.SeafileConfigResponse
import com.mycorrhizal.crm.model.network.SeafileConnectionTestResult
import com.mycorrhizal.crm.model.network.SeafileItem
import com.mycorrhizal.crm.model.network.SeafileLibrary
import com.mycorrhizal.crm.model.network.SeafileLinkRequest
import com.mycorrhizal.crm.model.network.WebDAVItem
import com.mycorrhizal.crm.network.ApiClient
import javax.inject.Inject

/**
 * Online-only access to the ExternalIdentity substrate — see the interface's
 * doc comment for why there is no Room mirror. The list is a full_resync page
 * per contact; deletes hard-remove the row server-side.
 */
class ExternalIdentityRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
) : ExternalIdentityRepository {

    override suspend fun listForContact(entityId: String): Result<List<ExternalIdentity>> =
        apiClient.listExternalIdentities(entityId).map { it.externalIdentities }

    override suspend fun delete(id: String): Result<Unit> =
        apiClient.deleteExternalIdentity(id)
}

/** Online-only Immich integration — delegates straight to the ApiClient. */
class ImmichRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
) : ImmichRepository {

    override suspend fun isConfigured(): Result<Boolean> =
        apiClient.getImmichConfig().map { it.hasApiKey }

    override suspend fun listPeople(): Result<List<ImmichPerson>> =
        apiClient.listImmichPeople()

    override suspend fun linkPerson(vcardUid: String, person: ImmichPerson): Result<Unit> =
        apiClient.linkImmichContact(vcardUid, person.id, person.name)

    override suspend fun unlinkPerson(vcardUid: String): Result<Unit> =
        apiClient.unlinkImmichContact(vcardUid)

    override suspend fun getContactSummary(vcardUid: String): Result<ImmichPersonSummary?> =
        apiClient.getImmichContactSummary(vcardUid)

    override suspend fun listContactAssets(vcardUid: String): Result<List<ImmichAssetSummary>> =
        apiClient.listImmichContactAssets(vcardUid)

    override suspend fun getThumbnailBytes(vcardUid: String): Result<ByteArray> =
        apiClient.getImmichThumbnailBytes(vcardUid)

    override suspend fun getAssetImageBytes(vcardUid: String, assetId: String): Result<ByteArray> =
        apiClient.getImmichAssetImageBytes(vcardUid, assetId)

    override suspend fun getConfig(): Result<ImmichConfigResponse> = apiClient.getImmichConfig()

    override suspend fun saveConfig(input: ImmichConfigInput): Result<ImmichConfigResponse> =
        apiClient.saveImmichConfig(input)

    override suspend fun deleteConfig(): Result<Unit> = apiClient.deleteImmichConfig()

    override suspend fun testConnection(): Result<ImmichConnectionTestResult> = apiClient.testImmichConnection()
}

/** Online-only Paperless-ngx integration (issue #236) — delegates straight to the ApiClient. */
class PaperlessRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
) : PaperlessRepository {

    override suspend fun isConfigured(): Result<Boolean> =
        apiClient.getPaperlessConfig().map { it.hasApiToken }

    override suspend fun getConfig(): Result<PaperlessConfigResponse> = apiClient.getPaperlessConfig()

    override suspend fun saveConfig(input: PaperlessConfigInput): Result<PaperlessConfigResponse> =
        apiClient.savePaperlessConfig(input)

    override suspend fun deleteConfig(): Result<Unit> = apiClient.deletePaperlessConfig()

    override suspend fun testConnection(): Result<PaperlessConnectionTestResult> =
        apiClient.testPaperlessConnection()

    override suspend fun searchDocuments(query: String?): Result<List<PaperlessDocument>> =
        apiClient.searchPaperlessDocuments(query)

    override suspend fun linkDocument(vcardUid: String, documentId: String): Result<Unit> =
        apiClient.linkPaperlessContact(vcardUid, documentId)
}

/** Online-only Seafile integration (issue #236) — delegates straight to the ApiClient. */
class SeafileRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
) : SeafileRepository {

    override suspend fun isConfigured(): Result<Boolean> =
        apiClient.getSeafileConfig().map { it.hasApiToken }

    override suspend fun getConfig(): Result<SeafileConfigResponse> = apiClient.getSeafileConfig()

    override suspend fun saveConfig(input: SeafileConfigInput): Result<SeafileConfigResponse> =
        apiClient.saveSeafileConfig(input)

    override suspend fun deleteConfig(): Result<Unit> = apiClient.deleteSeafileConfig()

    override suspend fun testConnection(): Result<SeafileConnectionTestResult> =
        apiClient.testSeafileConnection()

    override suspend fun listLibraries(): Result<List<SeafileLibrary>> = apiClient.listSeafileLibraries()

    override suspend fun listDir(repoId: String, path: String): Result<List<SeafileItem>> =
        apiClient.listSeafileDir(repoId, path)

    override suspend fun linkItem(vcardUid: String, request: SeafileLinkRequest): Result<Unit> =
        apiClient.linkSeafileContact(vcardUid, request)
}

/** Online-only Nextcloud/WebDAV integration (issue #236) — delegates straight to the ApiClient. */
class NextcloudRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
) : NextcloudRepository {

    override suspend fun isConfigured(): Result<Boolean> =
        apiClient.getNextcloudConfig().map { it.hasAppPassword }

    override suspend fun getConfig(): Result<NextcloudConfigResponse> = apiClient.getNextcloudConfig()

    override suspend fun saveConfig(input: NextcloudConfigInput): Result<NextcloudConfigResponse> =
        apiClient.saveNextcloudConfig(input)

    override suspend fun deleteConfig(): Result<Unit> = apiClient.deleteNextcloudConfig()

    override suspend fun testConnection(): Result<NextcloudConnectionTestResult> =
        apiClient.testNextcloudConnection()

    override suspend fun listDir(path: String): Result<List<WebDAVItem>> = apiClient.listNextcloudDir(path)

    override suspend fun linkItem(vcardUid: String, item: WebDAVItem): Result<Unit> =
        apiClient.linkNextcloudContact(
            vcardUid,
            NextcloudLinkRequest(
                path = item.path,
                name = item.name,
                type = item.type,
                size = item.size,
                modifiedAt = item.modifiedAt,
                fileId = item.fileId,
            ),
        )
}
