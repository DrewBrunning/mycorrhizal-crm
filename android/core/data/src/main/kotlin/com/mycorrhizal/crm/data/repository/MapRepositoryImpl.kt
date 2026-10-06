package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.domain.repository.MapRepository
import com.mycorrhizal.crm.model.network.ContactMapResponse
import com.mycorrhizal.crm.model.network.GeocodeDraftRequest
import com.mycorrhizal.crm.model.network.GeocodeDraftResponse
import com.mycorrhizal.crm.model.network.MapConfig
import com.mycorrhizal.crm.network.ApiClient
import javax.inject.Inject

/** Stateless pass-through over [ApiClient]; see [MapRepository]. */
class MapRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
) : MapRepository {
    override suspend fun getMapConfig(): Result<MapConfig> = apiClient.getMapConfig()

    override suspend fun getContactMap(): Result<ContactMapResponse> = apiClient.getContactMap()

    override suspend fun geocodeAddressDraft(contactId: Int, draft: GeocodeDraftRequest): Result<GeocodeDraftResponse> =
        apiClient.geocodeAddressDraft(contactId, draft)
}
