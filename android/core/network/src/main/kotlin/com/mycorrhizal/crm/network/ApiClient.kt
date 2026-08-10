package com.mycorrhizal.crm.network

import com.mycorrhizal.crm.model.network.ActivitiesPage
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.ActivityInput
import com.mycorrhizal.crm.model.network.AddCircleMemberResponse
import com.mycorrhizal.crm.model.network.AddContactTagResponse
import com.mycorrhizal.crm.model.network.AddHouseholdMemberResponse
import com.mycorrhizal.crm.model.network.BackendError
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.CircleDetailResponse
import com.mycorrhizal.crm.model.network.CircleInput
import com.mycorrhizal.crm.model.network.CircleMember
import com.mycorrhizal.crm.model.network.CircleMemberInput
import com.mycorrhizal.crm.model.network.CirclesPage
import com.mycorrhizal.crm.model.network.ContactActivitiesResponse
import com.mycorrhizal.crm.model.network.ContactNotesResponse
import com.mycorrhizal.crm.model.network.ContactRecordInput
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.ContactRemindersResponse
import com.mycorrhizal.crm.model.network.ContactTag
import com.mycorrhizal.crm.model.network.ContactTagInput
import com.mycorrhizal.crm.model.network.ContactsPage
import com.mycorrhizal.crm.model.network.CreateActivityResponse
import com.mycorrhizal.crm.model.network.CreateCircleResponse
import com.mycorrhizal.crm.model.network.CreateContactResponse
import com.mycorrhizal.crm.model.network.CreateHouseholdResponse
import com.mycorrhizal.crm.model.network.CreateNoteResponse
import com.mycorrhizal.crm.model.network.CreateRelationshipEdgeResponse
import com.mycorrhizal.crm.model.network.CreateReminderResponse
import com.mycorrhizal.crm.model.network.CreateTagResponse
import com.mycorrhizal.crm.model.network.Household
import com.mycorrhizal.crm.model.network.HouseholdDetailResponse
import com.mycorrhizal.crm.model.network.HouseholdInput
import com.mycorrhizal.crm.model.network.HouseholdMember
import com.mycorrhizal.crm.model.network.HouseholdMemberInput
import com.mycorrhizal.crm.model.network.HouseholdsPage
import com.mycorrhizal.crm.model.network.LoginRequest
import com.mycorrhizal.crm.model.network.LoginResponse
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.model.network.NoteInput
import com.mycorrhizal.crm.model.network.RelationshipEdge
import com.mycorrhizal.crm.model.network.RelationshipEdgeInput
import com.mycorrhizal.crm.model.network.RelationshipEdgesPage
import com.mycorrhizal.crm.model.network.Reminder
import com.mycorrhizal.crm.model.network.ReminderCompleteResponse
import com.mycorrhizal.crm.model.network.Tag
import com.mycorrhizal.crm.model.network.TagDetailResponse
import com.mycorrhizal.crm.model.network.TagInput
import com.mycorrhizal.crm.model.network.TagsPage
import com.mycorrhizal.crm.model.network.UserProfile
import com.squareup.moshi.Moshi
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
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
        executePost(LOGIN_PATH, LoginRequest(identifier = identifier, password = password)) { response, body ->
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
        executeGet("$PLACEHOLDER_ORIGIN$ME_PATH") { _, body ->
            moshi.adapter(UserProfile::class.java).fromJson(body)
        }

    /** GET /api/v1/contacts (cursor-paginated list). */
    suspend fun listContacts(
        cursor: String? = null,
        limit: Int? = null,
        search: String? = null,
        includeArchived: Boolean? = null,
    ): Result<ContactsPage> {
        val urlBuilder = "$PLACEHOLDER_ORIGIN$CONTACTS_PATH".toHttpUrl().newBuilder()
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
        executeGet("$PLACEHOLDER_ORIGIN$CONTACTS_PATH/$id") { _, body ->
            moshi.adapter(ContactRecordResponse::class.java).fromJson(body)
        }

    /**
     * POST /api/v1/contacts. The create endpoint wraps its response as
     * `{ message, contact }` (ticket §2.6 asymmetry) — this method unwraps it
     * so callers receive the bare ContactRecordResponse.
     */
    suspend fun createContact(input: ContactRecordInput): Result<ContactRecordResponse> =
        executePost(CONTACTS_PATH, input) { _, body ->
            moshi.adapter(CreateContactResponse::class.java).fromJson(body)?.contact
        }

    /** PUT /api/v1/contacts/{id} — returns the raw ContactRecordResponse (200). */
    suspend fun updateContact(id: Int, input: ContactRecordInput): Result<ContactRecordResponse> =
        executePut("$PLACEHOLDER_ORIGIN$CONTACTS_PATH/$id", input) { _, body ->
            moshi.adapter(ContactRecordResponse::class.java).fromJson(body)
        }

    /** GET /api/v1/contacts/{id}/activities — a contact's activities. */
    suspend fun listContactActivities(contactId: Int): Result<ContactActivitiesResponse> =
        executeGet("$PLACEHOLDER_ORIGIN$CONTACTS_PATH/$contactId/activities") { _, body ->
            moshi.adapter(ContactActivitiesResponse::class.java).fromJson(body)
        }

    /** POST /api/v1/activities — wrapped `{ message, activity }`, unwrapped here. */
    suspend fun createActivity(input: ActivityInput): Result<Activity> =
        executePost(ACTIVITIES_PATH, input) { _, body ->
            moshi.adapter(CreateActivityResponse::class.java).fromJson(body)?.activity
        }

    /** PUT /api/v1/activities/{id} — raw Activity response. */
    suspend fun updateActivity(id: Int, input: ActivityInput): Result<Activity> =
        executePut("$PLACEHOLDER_ORIGIN$ACTIVITIES_PATH/$id", input) { _, body ->
            moshi.adapter(Activity::class.java).fromJson(body)
        }

    /** GET /api/v1/activities/{id} — a single activity (with participants). */
    suspend fun getActivity(id: Int): Result<Activity> =
        executeGet("$PLACEHOLDER_ORIGIN$ACTIVITIES_PATH/$id") { _, body ->
            moshi.adapter(Activity::class.java).fromJson(body)
        }

    /** GET /api/v1/activities (cursor-paginated, all activities). */
    suspend fun listActivities(cursor: String? = null, limit: Int? = null): Result<ActivitiesPage> {
        val urlBuilder = "$PLACEHOLDER_ORIGIN$ACTIVITIES_PATH".toHttpUrl().newBuilder()
        cursor?.let { urlBuilder.addQueryParameter("cursor", it) }
        limit?.let { urlBuilder.addQueryParameter("limit", it.toString()) }
        return executeGet(urlBuilder.build().toString()) { _, body ->
            moshi.adapter(ActivitiesPage::class.java).fromJson(body)
        }
    }

    /** GET /api/v1/contacts/{id}/notes — a contact's notes. */
    suspend fun listContactNotes(contactId: Int): Result<ContactNotesResponse> =
        executeGet("$PLACEHOLDER_ORIGIN$CONTACTS_PATH/$contactId/notes") { _, body ->
            moshi.adapter(ContactNotesResponse::class.java).fromJson(body)
        }

    /** POST /api/v1/contacts/{id}/notes — wrapped `{ message, note }`, unwrapped here. */
    suspend fun createNote(contactId: Int, input: NoteInput): Result<Note> =
        executePost("$CONTACTS_PATH/$contactId/notes", input) { _, body ->
            moshi.adapter(CreateNoteResponse::class.java).fromJson(body)?.note
        }

    /** PUT /api/v1/notes/{id} — wrapped `{ message, note }`, unwrapped here. */
    suspend fun updateNote(id: Int, input: NoteInput): Result<Note> =
        executePut("$PLACEHOLDER_ORIGIN$NOTES_PATH/$id", input) { _, body ->
            moshi.adapter(CreateNoteResponse::class.java).fromJson(body)?.note
        }

    /** GET /api/v1/notes/{id} — a single note. */
    suspend fun getNote(id: Int): Result<Note> =
        executeGet("$PLACEHOLDER_ORIGIN$NOTES_PATH/$id") { _, body ->
            moshi.adapter(Note::class.java).fromJson(body)
        }

    /** GET /api/v1/contacts/{id}/reminders — a contact's reminders. */
    suspend fun listContactReminders(contactId: Int): Result<ContactRemindersResponse> =
        executeGet("$PLACEHOLDER_ORIGIN$CONTACTS_PATH/$contactId/reminders") { _, body ->
            moshi.adapter(ContactRemindersResponse::class.java).fromJson(body)
        }

    /** POST /api/v1/contacts/{id}/reminders — wrapped `{ message, reminder }`, unwrapped here. */
    suspend fun createReminder(contactId: Int, reminder: Reminder): Result<Reminder> =
        executePost("$CONTACTS_PATH/$contactId/reminders", reminder) { _, body ->
            moshi.adapter(CreateReminderResponse::class.java).fromJson(body)?.reminder
        }

    /** PUT /api/v1/reminders/{id} — wrapped `{ message, reminder }`, unwrapped here. */
    suspend fun updateReminder(id: Int, reminder: Reminder): Result<Reminder> =
        executePut("$PLACEHOLDER_ORIGIN$REMINDERS_PATH/$id", reminder) { _, body ->
            moshi.adapter(CreateReminderResponse::class.java).fromJson(body)?.reminder
        }

    /** POST /api/v1/reminders/{id}/complete — completes a reminder (no body). */
    suspend fun completeReminder(id: Int): Result<ReminderCompleteResponse> =
        executePostEmpty("$REMINDERS_PATH/$id/complete") { _, body ->
            moshi.adapter(ReminderCompleteResponse::class.java).fromJson(body)
        }

    /** GET /api/v1/reminders/{id} — a single reminder. */
    suspend fun getReminder(id: Int): Result<Reminder> =
        executeGet("$PLACEHOLDER_ORIGIN$REMINDERS_PATH/$id") { _, body ->
            moshi.adapter(Reminder::class.java).fromJson(body)
        }

    /** GET /api/v1/circles — cursor-paginated; members when include_members=true. */
    suspend fun listCircles(
        cursor: String? = null,
        limit: Int? = null,
        includeMembers: Boolean = false,
    ): Result<CirclesPage> {
        val urlBuilder = "$PLACEHOLDER_ORIGIN$CIRCLES_PATH".toHttpUrl().newBuilder()
        cursor?.let { urlBuilder.addQueryParameter("cursor", it) }
        limit?.let { urlBuilder.addQueryParameter("limit", it.toString()) }
        if (includeMembers) urlBuilder.addQueryParameter("include_members", "true")
        return executeGet(urlBuilder.build().toString()) { _, body ->
            moshi.adapter(CirclesPage::class.java).fromJson(body)
        }
    }

    /** GET /api/v1/circles/{id} — `{ circle, members }`. */
    suspend fun getCircle(id: String): Result<CircleDetailResponse> =
        executeGet("$PLACEHOLDER_ORIGIN$CIRCLES_PATH/$id") { _, body ->
            moshi.adapter(CircleDetailResponse::class.java).fromJson(body)
        }

    /** POST /api/v1/circles — wrapped `{ message, circle }`, unwrapped here. */
    suspend fun createCircle(input: CircleInput): Result<Circle> =
        executePost(CIRCLES_PATH, input) { _, body ->
            moshi.adapter(CreateCircleResponse::class.java).fromJson(body)?.circle
        }

    /** PUT /api/v1/circles/{id} — raw Circle response. */
    suspend fun updateCircle(id: String, input: CircleInput): Result<Circle> =
        executePut("$PLACEHOLDER_ORIGIN$CIRCLES_PATH/$id", input) { _, body ->
            moshi.adapter(Circle::class.java).fromJson(body)
        }

    /** DELETE /api/v1/circles/{id}. */
    suspend fun deleteCircle(id: String): Result<Unit> =
        executeDelete("$PLACEHOLDER_ORIGIN$CIRCLES_PATH/$id")

    /** POST /api/v1/circles/{id}/members — wrapped `{ message, member }`. */
    suspend fun addCircleMember(circleId: String, input: CircleMemberInput): Result<CircleMember> =
        executePost("$CIRCLES_PATH/$circleId/members", input) { _, body ->
            moshi.adapter(AddCircleMemberResponse::class.java).fromJson(body)?.member
        }

    /** DELETE /api/v1/circles/{id}/members/{vcard_uid}. */
    suspend fun removeCircleMember(circleId: String, vcardUid: String): Result<Unit> =
        executeDelete("$PLACEHOLDER_ORIGIN$CIRCLES_PATH/$circleId/members/$vcardUid")

    /** GET /api/v1/tags — cursor-paginated; contacts when include_contacts=true. */
    suspend fun listTags(
        cursor: String? = null,
        limit: Int? = null,
        includeContacts: Boolean = false,
    ): Result<TagsPage> {
        val urlBuilder = "$PLACEHOLDER_ORIGIN$TAGS_PATH".toHttpUrl().newBuilder()
        cursor?.let { urlBuilder.addQueryParameter("cursor", it) }
        limit?.let { urlBuilder.addQueryParameter("limit", it.toString()) }
        if (includeContacts) urlBuilder.addQueryParameter("include_contacts", "true")
        return executeGet(urlBuilder.build().toString()) { _, body ->
            moshi.adapter(TagsPage::class.java).fromJson(body)
        }
    }

    /** GET /api/v1/tags/{id} — `{ tag, contacts }`. */
    suspend fun getTag(id: String): Result<TagDetailResponse> =
        executeGet("$PLACEHOLDER_ORIGIN$TAGS_PATH/$id") { _, body ->
            moshi.adapter(TagDetailResponse::class.java).fromJson(body)
        }

    /** POST /api/v1/tags — wrapped `{ message, tag }`, unwrapped here. */
    suspend fun createTag(input: TagInput): Result<Tag> =
        executePost(TAGS_PATH, input) { _, body ->
            moshi.adapter(CreateTagResponse::class.java).fromJson(body)?.tag
        }

    /** PUT /api/v1/tags/{id} — raw Tag response. */
    suspend fun updateTag(id: String, input: TagInput): Result<Tag> =
        executePut("$PLACEHOLDER_ORIGIN$TAGS_PATH/$id", input) { _, body ->
            moshi.adapter(Tag::class.java).fromJson(body)
        }

    /** DELETE /api/v1/tags/{id}. */
    suspend fun deleteTag(id: String): Result<Unit> =
        executeDelete("$PLACEHOLDER_ORIGIN$TAGS_PATH/$id")

    /** POST /api/v1/tags/{id}/contacts — wrapped `{ message, tagging }`. */
    suspend fun addContactTag(tagId: String, input: ContactTagInput): Result<ContactTag> =
        executePost("$TAGS_PATH/$tagId/contacts", input) { _, body ->
            moshi.adapter(AddContactTagResponse::class.java).fromJson(body)?.tagging
        }

    /** DELETE /api/v1/tags/{id}/contacts/{vcard_uid}. */
    suspend fun removeContactTag(tagId: String, vcardUid: String): Result<Unit> =
        executeDelete("$PLACEHOLDER_ORIGIN$TAGS_PATH/$tagId/contacts/$vcardUid")

    /** GET /api/v1/households — cursor-paginated; members when include_members=true. */
    suspend fun listHouseholds(
        cursor: String? = null,
        limit: Int? = null,
        includeMembers: Boolean = false,
    ): Result<HouseholdsPage> {
        val urlBuilder = "$PLACEHOLDER_ORIGIN$HOUSEHOLDS_PATH".toHttpUrl().newBuilder()
        cursor?.let { urlBuilder.addQueryParameter("cursor", it) }
        limit?.let { urlBuilder.addQueryParameter("limit", it.toString()) }
        if (includeMembers) urlBuilder.addQueryParameter("include_members", "true")
        return executeGet(urlBuilder.build().toString()) { _, body ->
            moshi.adapter(HouseholdsPage::class.java).fromJson(body)
        }
    }

    /** GET /api/v1/households/{id} — `{ household, members }`. */
    suspend fun getHousehold(id: String): Result<HouseholdDetailResponse> =
        executeGet("$PLACEHOLDER_ORIGIN$HOUSEHOLDS_PATH/$id") { _, body ->
            moshi.adapter(HouseholdDetailResponse::class.java).fromJson(body)
        }

    /** POST /api/v1/households — wrapped `{ message, household }`, unwrapped here. */
    suspend fun createHousehold(input: HouseholdInput): Result<Household> =
        executePost(HOUSEHOLDS_PATH, input) { _, body ->
            moshi.adapter(CreateHouseholdResponse::class.java).fromJson(body)?.household
        }

    /** PUT /api/v1/households/{id} — raw Household response. */
    suspend fun updateHousehold(id: String, input: HouseholdInput): Result<Household> =
        executePut("$PLACEHOLDER_ORIGIN$HOUSEHOLDS_PATH/$id", input) { _, body ->
            moshi.adapter(Household::class.java).fromJson(body)
        }

    /** DELETE /api/v1/households/{id}. */
    suspend fun deleteHousehold(id: String): Result<Unit> =
        executeDelete("$PLACEHOLDER_ORIGIN$HOUSEHOLDS_PATH/$id")

    /** POST /api/v1/households/{id}/members — wrapped `{ message, member }`. */
    suspend fun addHouseholdMember(id: String, input: HouseholdMemberInput): Result<HouseholdMember> =
        executePost("$HOUSEHOLDS_PATH/$id/members", input) { _, body ->
            moshi.adapter(AddHouseholdMemberResponse::class.java).fromJson(body)?.member
        }

    /** DELETE /api/v1/households/{id}/members/{vcard_uid}. */
    suspend fun removeHouseholdMember(id: String, vcardUid: String): Result<Unit> =
        executeDelete("$PLACEHOLDER_ORIGIN$HOUSEHOLDS_PATH/$id/members/$vcardUid")

    /** PATCH /api/v1/households/{id}/members/{vcard_uid} — update role/since/until. */
    suspend fun updateHouseholdMember(id: String, vcardUid: String, input: HouseholdMemberInput): Result<Unit> =
        executePatch("$PLACEHOLDER_ORIGIN$HOUSEHOLDS_PATH/$id/members/$vcardUid", input)

    /** GET /api/v1/relationship-edges — cursor-paginated, filtered by contact. */
    suspend fun listRelationshipEdges(
        contactId: String,
        status: String? = null,
        cursor: String? = null,
        limit: Int? = null,
    ): Result<RelationshipEdgesPage> {
        val urlBuilder = "$PLACEHOLDER_ORIGIN$RELATIONSHIP_EDGES_PATH".toHttpUrl().newBuilder()
        urlBuilder.addQueryParameter("contact_id", contactId)
        status?.let { urlBuilder.addQueryParameter("status", it) }
        cursor?.let { urlBuilder.addQueryParameter("cursor", it) }
        limit?.let { urlBuilder.addQueryParameter("limit", it.toString()) }
        return executeGet(urlBuilder.build().toString()) { _, body ->
            moshi.adapter(RelationshipEdgesPage::class.java).fromJson(body)
        }
    }

    /** POST /api/v1/relationship-edges — wrapped `{ relationship_edge }`, unwrapped here. */
    suspend fun createRelationshipEdge(input: RelationshipEdgeInput): Result<RelationshipEdge> =
        executePost(RELATIONSHIP_EDGES_PATH, input) { _, body ->
            moshi.adapter(CreateRelationshipEdgeResponse::class.java).fromJson(body)?.relationshipEdge
        }

    /** PATCH /api/v1/relationship-edges/{id}/accept — promotes a suggestion; raw edge. */
    suspend fun acceptRelationshipEdge(id: String): Result<RelationshipEdge> =
        executePatchEmpty("$PLACEHOLDER_ORIGIN$RELATIONSHIP_EDGES_PATH/$id/accept") { _, body ->
            moshi.adapter(RelationshipEdge::class.java).fromJson(body)
        }

    /** DELETE /api/v1/relationship-edges/{id} — doubles as reject for suggestions. */
    suspend fun deleteRelationshipEdge(id: String): Result<Unit> =
        executeDelete("$PLACEHOLDER_ORIGIN$RELATIONSHIP_EDGES_PATH/$id")

    private suspend fun <T> executeGet(
        url: String,
        mapper: (okhttp3.Response, String) -> T?,
    ): Result<T> {
        val request = Request.Builder().url(url).get().build()
        return execute(request, mapper)
    }

    private suspend fun <T> executePost(
        path: String,
        body: Any,
        mapper: (okhttp3.Response, String) -> T?,
    ): Result<T> {
        val request = Request.Builder()
            .url("$PLACEHOLDER_ORIGIN$path".toHttpUrl())
            .post(body.toJsonBody())
            .build()
        return execute(request, mapper)
    }

    private suspend fun <T> executePostEmpty(
        path: String,
        mapper: (okhttp3.Response, String) -> T?,
    ): Result<T> {
        val request = Request.Builder()
            .url("$PLACEHOLDER_ORIGIN$path".toHttpUrl())
            .post(okhttp3.RequestBody.create(null, ByteArray(0)))
            .build()
        return execute(request, mapper)
    }

    private suspend fun <T> executePut(
        url: String,
        body: Any,
        mapper: (okhttp3.Response, String) -> T?,
    ): Result<T> {
        val request = Request.Builder()
            .url(url.toHttpUrl())
            .put(body.toJsonBody())
            .build()
        return execute(request, mapper)
    }

    private suspend fun <T> executePatch(
        url: String,
        body: Any,
        mapper: (okhttp3.Response, String) -> T? = { _, _ -> Unit as T },
    ): Result<T> {
        val request = Request.Builder()
            .url(url.toHttpUrl())
            .patch(body.toJsonBody())
            .build()
        return execute(request, mapper)
    }

    private suspend fun <T> executePatchEmpty(
        url: String,
        mapper: (okhttp3.Response, String) -> T?,
    ): Result<T> {
        val request = Request.Builder()
            .url(url.toHttpUrl())
            .patch(okhttp3.RequestBody.create(null, ByteArray(0)))
            .build()
        return execute(request, mapper)
    }

    private fun Any.toJsonBody(): okhttp3.RequestBody =
        moshi.adapter<Any>(javaClass).toJson(this).toRequestBody(jsonMediaType)

    private suspend fun <T> executeDelete(
        url: String,
        mapper: (okhttp3.Response, String) -> T? = { _, _ -> Unit as T },
    ): Result<T> {
        val request = Request.Builder()
            .url(url.toHttpUrl())
            .delete()
            .build()
        return execute(request, mapper)
    }

    private suspend fun <T> execute(
        request: Request,
        mapper: (okhttp3.Response, String) -> T?,
    ): Result<T> = withContext(Dispatchers.IO) {
        try {
            val response = okHttpClient.newCall(request).execute()
            response.use {
                val body = it.body?.string().orEmpty()
                if (!it.isSuccessful) {
                    return@withContext Result.failure(parseError(it.code, body))
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
        private const val ACTIVITIES_PATH = "$API_V1/activities"
        private const val NOTES_PATH = "$API_V1/notes"
        private const val REMINDERS_PATH = "$API_V1/reminders"
        private const val CIRCLES_PATH = "$API_V1/circles"
        private const val TAGS_PATH = "$API_V1/tags"
        private const val HOUSEHOLDS_PATH = "$API_V1/households"
        private const val RELATIONSHIP_EDGES_PATH = "$API_V1/relationship-edges"
        private const val AUTH_COOKIE = "auth_token"
    }
}

/** Successful login: the bearer JWT (captured from the httpOnly cookie) plus profile prefs. */
data class LoginResult(
    val token: String?,
    val language: String?,
    val dateFormat: String?,
)
