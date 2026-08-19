package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.ExternalIdentity
import com.mycorrhizal.crm.model.network.ImmichAssetSummary
import com.mycorrhizal.crm.model.network.ImmichPerson
import com.mycorrhizal.crm.model.network.ImmichPersonSummary

/**
 * Read-only access to the ExternalIdentity substrate (T14) for one contact
 * (issue #220). Online-only by design: the rows are edge/join-shaped, hard
 * deleted, and bounded (full_resync per contact), so there is no Room mirror —
 * the contact detail fetches them fresh and a client re-pulls after a delete,
 * the same trust model as the graph/contact-shares repositories.
 */
interface ExternalIdentityRepository {
    /** GET /external-identities?contact_id=<entityId> — the contact's links, newest-first. */
    suspend fun listForContact(entityId: String): Result<List<ExternalIdentity>>

    /** DELETE /external-identities/:id — removes a link (hard delete). */
    suspend fun delete(id: String): Result<Unit>
}

/**
 * The Immich integration (T15/T16): per-contact link/summary/assets plus the
 * profile-photo "choose from Immich" flow. All calls go through the backend;
 * photo bytes are fetched here so the caller can crop then upload. Online-only,
 * like [ExternalIdentityRepository] — nothing here is cached locally.
 */
interface ImmichRepository {
    /** GET /immich/config — `has_api_key` gates whether the Immich UI entry points are shown. */
    suspend fun isConfigured(): Result<Boolean>

    /** GET /immich/people — every person in the user's instance (search is client-side). */
    suspend fun listPeople(): Result<List<ImmichPerson>>

    /** POST /immich/contacts/:vcard_uid/link — links an Immich person to the contact. */
    suspend fun linkPerson(vcardUid: String, person: ImmichPerson): Result<Unit>

    /** DELETE /immich/contacts/:vcard_uid/link — unlinks (keeps enrichment history). */
    suspend fun unlinkPerson(vcardUid: String): Result<Unit>

    /** GET /immich/contacts/:vcard_uid/summary — null when the contact has no link. */
    suspend fun getContactSummary(vcardUid: String): Result<ImmichPersonSummary?>

    /** GET /immich/contacts/:vcard_uid/assets — the linked person's recent photos. */
    suspend fun listContactAssets(vcardUid: String): Result<List<ImmichAssetSummary>>

    /** The linked person's thumbnail bytes, for the profile-photo flow. */
    suspend fun getThumbnailBytes(vcardUid: String): Result<ByteArray>

    /** One recent photo's bytes (assetId from [listContactAssets]), for the profile-photo flow. */
    suspend fun getAssetImageBytes(vcardUid: String, assetId: String): Result<ByteArray>
}
