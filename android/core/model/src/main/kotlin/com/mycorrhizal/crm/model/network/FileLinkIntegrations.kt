package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * Issue #236: the create/link surfaces for the three file/document integrations on the
 * ExternalIdentity substrate (T14). Unlike Immich, none of these write the ExternalIdentity
 * row through the generic `/external-identities` endpoint — each system resolves authoritative
 * metadata server-side and creates the row itself inside its own `POST /<system>/contacts/:uid/link`
 * (mirrors `frontend/src/api/paperless.ts`/`seafile.ts`/`nextcloud.ts`). Config/test-connection/
 * browse endpoints pre-date this client; only Android's own surface was missing.
 */

// ---------------------------------------------------------------------------
// Paperless-ngx
// ---------------------------------------------------------------------------

/** GET /paperless/config — `has_api_token` gates the Paperless UI entry points. */
@JsonClass(generateAdapter = true)
data class PaperlessConfigResponse(
    @Json(name = "base_url") val baseUrl: String? = null,
    @Json(name = "has_api_token") val hasApiToken: Boolean = false,
)

/** PUT /paperless/config body — an empty [apiToken] keeps the stored token unchanged. */
@JsonClass(generateAdapter = true)
data class PaperlessConfigInput(
    @Json(name = "base_url") val baseUrl: String,
    @Json(name = "api_token") val apiToken: String = "",
)

/** POST /paperless/test-connection — 200 even when [ok] is false (diagnostic, not exceptional). */
@JsonClass(generateAdapter = true)
data class PaperlessConnectionTestResult(
    val ok: Boolean = false,
    val stage: String? = null,
    val message: String? = null,
)

@JsonClass(generateAdapter = true)
data class PaperlessDocument(
    val id: Int = 0,
    val title: String? = null,
    @Json(name = "file_name") val fileName: String? = null,
    val created: String? = null,
    val added: String? = null,
)

/** GET /paperless/documents?query=… — full list when [query] is omitted; unwrapped. */
@JsonClass(generateAdapter = true)
data class PaperlessDocumentsResponse(
    val documents: List<PaperlessDocument> = emptyList(),
)

/** POST /paperless/contacts/:vcard_uid/link body. */
@JsonClass(generateAdapter = true)
data class PaperlessLinkRequest(
    @Json(name = "document_id") val documentId: String,
)

// ---------------------------------------------------------------------------
// Seafile
// ---------------------------------------------------------------------------

/** GET /seafile/config — `has_api_token` gates the Seafile UI entry points. */
@JsonClass(generateAdapter = true)
data class SeafileConfigResponse(
    @Json(name = "base_url") val baseUrl: String? = null,
    @Json(name = "has_api_token") val hasApiToken: Boolean = false,
)

/** PUT /seafile/config body — an empty [apiToken] keeps the stored token unchanged. */
@JsonClass(generateAdapter = true)
data class SeafileConfigInput(
    @Json(name = "base_url") val baseUrl: String,
    @Json(name = "api_token") val apiToken: String = "",
)

/** POST /seafile/test-connection — 200 even when [ok] is false. */
@JsonClass(generateAdapter = true)
data class SeafileConnectionTestResult(
    val ok: Boolean = false,
    val stage: String? = null,
    val message: String? = null,
)

@JsonClass(generateAdapter = true)
data class SeafileLibrary(
    val id: String = "",
    val name: String = "",
    val type: String? = null,
)

/** GET /seafile/libraries — unwrapped. */
@JsonClass(generateAdapter = true)
data class SeafileLibrariesResponse(
    val libraries: List<SeafileLibrary> = emptyList(),
)

@JsonClass(generateAdapter = true)
data class SeafileItem(
    val id: String = "",
    val name: String = "",
    val type: String = "file",
    val size: Long = 0,
    val mtime: Long = 0,
    @Json(name = "parent_dir") val parentDir: String? = null,
)

/** GET /seafile/libraries/:repo_id/dir?path=… — unwrapped. */
@JsonClass(generateAdapter = true)
data class SeafileItemsResponse(
    val items: List<SeafileItem> = emptyList(),
)

/** POST /seafile/contacts/:vcard_uid/link body. */
@JsonClass(generateAdapter = true)
data class SeafileLinkRequest(
    @Json(name = "repo_id") val repoId: String,
    val path: String,
    val name: String,
    val type: String,
    val size: Long? = null,
    val mtime: Long? = null,
)

// ---------------------------------------------------------------------------
// Nextcloud (backend route/wire name "nextcloud"; the Go implementation is a
// generic WebDAV client shared with any ownCloud-compatible server)
// ---------------------------------------------------------------------------

/** GET /nextcloud/config — `has_app_password` gates the Nextcloud UI entry points. */
@JsonClass(generateAdapter = true)
data class NextcloudConfigResponse(
    @Json(name = "base_url") val baseUrl: String? = null,
    val username: String? = null,
    @Json(name = "has_app_password") val hasAppPassword: Boolean = false,
)

/**
 * PUT /nextcloud/config body — an empty [appPassword] keeps the stored password unchanged.
 * Unlike the other three integrations this one is basic-auth (base URL + username + an app
 * password, never the account password), so [username] is required up front.
 */
@JsonClass(generateAdapter = true)
data class NextcloudConfigInput(
    @Json(name = "base_url") val baseUrl: String,
    val username: String,
    @Json(name = "app_password") val appPassword: String = "",
)

/**
 * POST /nextcloud/test-connection — 200 even when [ok] is false. [stage] is only ever
 * "reachability" or "ok" here: WebDAV/PROPFIND can't distinguish a transport failure from
 * an auth failure the way the token-based integrations' `/auth`-probing stage can.
 */
@JsonClass(generateAdapter = true)
data class NextcloudConnectionTestResult(
    val ok: Boolean = false,
    val stage: String? = null,
    val message: String? = null,
)

@JsonClass(generateAdapter = true)
data class WebDAVItem(
    val name: String = "",
    val path: String = "",
    val type: String = "file",
    val size: Long? = null,
    @Json(name = "modified_at") val modifiedAt: String? = null,
    @Json(name = "file_id") val fileId: String? = null,
)

/** GET /nextcloud/dir?path=… — unwrapped. */
@JsonClass(generateAdapter = true)
data class NextcloudItemsResponse(
    val items: List<WebDAVItem> = emptyList(),
)

/** POST /nextcloud/contacts/:vcard_uid/link body. */
@JsonClass(generateAdapter = true)
data class NextcloudLinkRequest(
    val path: String,
    val name: String,
    val type: String,
    val size: Long? = null,
    @Json(name = "modified_at") val modifiedAt: String? = null,
    @Json(name = "file_id") val fileId: String? = null,
)
