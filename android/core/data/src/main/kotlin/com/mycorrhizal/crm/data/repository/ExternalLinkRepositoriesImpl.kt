package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.domain.repository.ExternalIdentityRepository
import com.mycorrhizal.crm.domain.repository.ImmichRepository
import com.mycorrhizal.crm.model.network.ExternalIdentity
import com.mycorrhizal.crm.model.network.ImmichAssetSummary
import com.mycorrhizal.crm.model.network.ImmichPerson
import com.mycorrhizal.crm.model.network.ImmichPersonSummary
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
}
