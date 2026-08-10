package com.mycorrhizal.crm.network

import com.mycorrhizal.crm.model.network.BackendError
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.ContactsPage
import com.mycorrhizal.crm.model.network.LoginRequest
import com.mycorrhizal.crm.model.network.LoginResponse
import com.mycorrhizal.crm.model.network.UserProfile
import com.squareup.moshi.Moshi
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import java.io.IOException

/**
 * Hand-written typed client for the Phase-1 backend surface (login, current
 * user, contacts list/detail). Request URLs use a placeholder origin; the
 * [BaseUrlInterceptor] rewrites them onto the user's configured server.
 *
 * Generated openapi-generator output is the long-term replacement once the
 * spec's allOf/oneOf schemas are flattened (ticket §1.3); this client is
 * deliberately small and endpoint-focused so the swap stays local to
 * :core:network.
 */
class ApiClient(
    private val okHttpClient: OkHttpClient,
    private val moshi: Moshi,
) {
    private val jsonMediaType = "application/json".toMediaType()

    /** POST /api/v1/login — captures the auth_token JWT from the Set-Cookie header. */
    suspend fun login(identifier: String, password: String): Result<LoginResult> =
        execute(LOGIN_PATH, LoginRequest(identifier = identifier, password = password)) { response, body ->
            val loginResponse = moshi.adapter(LoginResponse::class.java).fromJson(body)
            val token = extractCookie(response.headers("Set-Cookie"), AUTH_COOKIE)
            LoginResult(
                token = token,
                language = loginResponse?.language,
                dateFormat = loginResponse?.dateFormat,
            )
        }

    /** GET /api/v1/users/me. */
    suspend fun currentUser(): Result<UserProfile> =
        execute(ME_PATH, null) { _, body ->
            moshi.adapter(UserProfile::class.java).fromJson(body)
        }

    /** GET /api/v1/contacts (cursor-paginated list). */
    suspend fun listContacts(
        cursor: String? = null,
        limit: Int? = null,
        search: String? = null,
        includeArchived: Boolean? = null,
    ): Result<ContactsPage> {
        val urlBuilder = CONTACTS_PATH.toHttpUrl().newBuilder()
        cursor?.let { urlBuilder.addQueryParameter("cursor", it) }
        limit?.let { urlBuilder.addQueryParameter("limit", it.toString()) }
        search?.let { urlBuilder.addQueryParameter("search", it) }
        includeArchived?.let { urlBuilder.addQueryParameter("include_archived", it.toString()) }
        return executeGet(urlBuilder.build().toString()) { _, body ->
            moshi.adapter(ContactsPage::class.java).fromJson(body)
        }
    }

    /** GET /api/v1/contacts/{id} (full neutral Record/Card). */
    suspend fun getContact(id: Int): Result<ContactRecordResponse> =
        executeGet("$CONTACTS_PATH/$id") { _, body ->
            moshi.adapter(ContactRecordResponse::class.java).fromJson(body)
        }

    private suspend fun <T> execute(
        path: String,
        body: Any?,
        mapper: (okhttp3.Response, String) -> T?,
    ): Result<T> {
        val request = Request.Builder()
            .url(path.toHttpUrl())
            .apply {
                if (body != null) {
                    val json = moshi.adapter<Any>(body.javaClass).toJson(body)
                    post(json.toRequestBody(jsonMediaType))
                } else {
                    get()
                }
            }
            .build()
        return execute(request, mapper)
    }

    private suspend fun <T> executeGet(
        url: String,
        mapper: (okhttp3.Response, String) -> T?,
    ): Result<T> {
        val request = Request.Builder().url(url).get().build()
        return execute(request, mapper)
    }

    private suspend fun <T> execute(
        request: Request,
        mapper: (okhttp3.Response, String) -> T?,
    ): Result<T> {
        return try {
            val response = okHttpClient.newCall(request).execute()
            response.use {
                val body = it.body?.string().orEmpty()
                if (!it.isSuccessful) {
                    return Result.failure(parseError(it.code, body))
                }
                val mapped = mapper(it, body)
                if (mapped == null) {
                    Result.failure(ApiError.Parse("Empty response body"))
                } else {
                    Result.success(mapped)
                }
            }
        } catch (e: Exception) {
            Result.failure(e.toApiError())
        }
    }

    private fun parseError(code: Int, body: String): ApiError {
        val parsed = try {
            moshi.adapter(BackendError::class.java).fromJson(body)?.error?.displayMessage
        } catch (_: Exception) {
            null
        }
        val message = parsed?.takeIf { it.isNotBlank() } ?: body.ifBlank { "HTTP $code" }
        return if (code in 400..499) ApiError.Client(code, message) else ApiError.Server(code, message)
    }

    private fun extractCookie(setCookieHeaders: List<String>, name: String): String? {
        for (header in setCookieHeaders) {
            val cookie = header.substringBefore(';').trim()
            val eq = cookie.indexOf('=')
            if (eq > 0 && cookie.substring(0, eq) == name) {
                return cookie.substring(eq + 1)
            }
        }
        return null
    }

    companion object {
        private const val PLACEHOLDER_ORIGIN = "http://mycorrhizal.invalid"
        private const val API_V1 = "/api/v1"
        private const val LOGIN_PATH = "$API_V1/login"
        private const val ME_PATH = "$API_V1/users/me"
        private const val CONTACTS_PATH = "$API_V1/contacts"
        private const val AUTH_COOKIE = "auth_token"
    }
}

/** Successful login: the bearer JWT (captured from the httpOnly cookie) plus profile prefs. */
data class LoginResult(
    val token: String?,
    val language: String?,
    val dateFormat: String?,
)
