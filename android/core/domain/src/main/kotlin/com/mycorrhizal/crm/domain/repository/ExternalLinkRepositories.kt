package com.mycorrhizal.crm.domain.repository

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

    // --- Issue #236: the Immich connection-config settings screen ---

    /** GET /immich/config — the full config (base URL + `has_api_key` + sync status). */
    suspend fun getConfig(): Result<ImmichConfigResponse>

    /** PUT /immich/config — an empty [ImmichConfigInput.apiKey] keeps the stored key. */
    suspend fun saveConfig(input: ImmichConfigInput): Result<ImmichConfigResponse>

    /** DELETE /immich/config. */
    suspend fun deleteConfig(): Result<Unit>

    /** POST /immich/test-connection — a diagnosed failure is a success-with-`ok:false`. */
    suspend fun testConnection(): Result<ImmichConnectionTestResult>
}

/**
 * Issue #236: the Paperless-ngx document create/link surface. Config CRUD +
 * test-connection are analogous to [ImmichRepository]'s; [searchDocuments]/
 * [linkDocument] back the Paperless document picker. Online-only, like the
 * rest of this substrate.
 */
interface PaperlessRepository {
    /** GET /paperless/config — `has_api_token` gates the picker/settings entry points. */
    suspend fun isConfigured(): Result<Boolean>

    suspend fun getConfig(): Result<PaperlessConfigResponse>
    suspend fun saveConfig(input: PaperlessConfigInput): Result<PaperlessConfigResponse>
    suspend fun deleteConfig(): Result<Unit>
    suspend fun testConnection(): Result<PaperlessConnectionTestResult>

    /** GET /paperless/documents?query=… — full list when [query] is null (mirrors web). */
    suspend fun searchDocuments(query: String? = null): Result<List<PaperlessDocument>>

    /** POST /paperless/contacts/:vcard_uid/link — writes the ExternalIdentity server-side. */
    suspend fun linkDocument(vcardUid: String, documentId: String): Result<Unit>
}

/**
 * Issue #236: the Seafile file/folder create/link surface — a two-level browse
 * (libraries, then a directory tree within one) backing the Seafile picker.
 */
interface SeafileRepository {
    /** GET /seafile/config — `has_api_token` gates the picker/settings entry points. */
    suspend fun isConfigured(): Result<Boolean>

    suspend fun getConfig(): Result<SeafileConfigResponse>
    suspend fun saveConfig(input: SeafileConfigInput): Result<SeafileConfigResponse>
    suspend fun deleteConfig(): Result<Unit>
    suspend fun testConnection(): Result<SeafileConnectionTestResult>

    /** GET /seafile/libraries. */
    suspend fun listLibraries(): Result<List<SeafileLibrary>>

    /** GET /seafile/libraries/:repo_id/dir?path=… */
    suspend fun listDir(repoId: String, path: String): Result<List<SeafileItem>>

    /** POST /seafile/contacts/:vcard_uid/link — writes the ExternalIdentity server-side. */
    suspend fun linkItem(vcardUid: String, request: SeafileLinkRequest): Result<Unit>
}

/**
 * Issue #236: the Nextcloud/WebDAV file/folder create/link surface — a
 * single-level directory browse backing the Nextcloud picker. The wire name
 * and route are "nextcloud"; the backend implementation is a generic WebDAV
 * client shared with any ownCloud-compatible server.
 */
interface NextcloudRepository {
    /** GET /nextcloud/config — `has_app_password` gates the picker/settings entry points. */
    suspend fun isConfigured(): Result<Boolean>

    suspend fun getConfig(): Result<NextcloudConfigResponse>
    suspend fun saveConfig(input: NextcloudConfigInput): Result<NextcloudConfigResponse>
    suspend fun deleteConfig(): Result<Unit>
    suspend fun testConnection(): Result<NextcloudConnectionTestResult>

    /** GET /nextcloud/dir?path=… — defaults to the dav root. */
    suspend fun listDir(path: String = "/"): Result<List<WebDAVItem>>

    /** POST /nextcloud/contacts/:vcard_uid/link — writes the ExternalIdentity server-side. */
    suspend fun linkItem(vcardUid: String, item: WebDAVItem): Result<Unit>
}
