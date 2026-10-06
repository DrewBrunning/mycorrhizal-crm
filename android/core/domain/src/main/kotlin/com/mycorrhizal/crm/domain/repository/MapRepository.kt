package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.ContactMapResponse
import com.mycorrhizal.crm.model.network.GeocodeDraftRequest
import com.mycorrhizal.crm.model.network.GeocodeDraftResponse
import com.mycorrhizal.crm.model.network.MapConfig

/**
 * Contact map data access (ADR 0031, issue #1287). Online-first and
 * deliberately uncached: coordinates are server-owned address data and the
 * only copy this client keeps is MapLibre's own tile cache.
 */
interface MapRepository {
    /** GET /config/map — the instance's tile style URL (public). */
    suspend fun getMapConfig(): Result<MapConfig>

    /** GET /contacts/map — every plottable address of the owner's contacts. */
    suspend fun getContactMap(): Result<ContactMapResponse>

    /**
     * POST /contacts/{id}/addresses/geocode — one explicit, stateless lookup for
     * an address draft (saved or not). The coordinate is returned, never stored.
     */
    suspend fun geocodeAddressDraft(contactId: Int, draft: GeocodeDraftRequest): Result<GeocodeDraftResponse>
}
