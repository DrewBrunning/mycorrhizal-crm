package com.mycorrhizal.crm.network

import com.mycorrhizal.crm.model.network.AcceptHouseholdSuggestionInput
import com.mycorrhizal.crm.model.network.AcceptHouseholdSuggestionResponse
import com.mycorrhizal.crm.model.network.AddressSuggestionsResponse
import com.mycorrhizal.crm.model.network.DismissHouseholdSuggestionInput
import com.mycorrhizal.crm.model.network.SuggestRelationshipsResponse
import com.mycorrhizal.crm.model.network.ActivitiesPage
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.ActivityInput
import com.mycorrhizal.crm.model.network.AddCircleMemberResponse
import com.mycorrhizal.crm.model.network.AddContactTagResponse
import com.mycorrhizal.crm.model.network.AddHouseholdMemberResponse
import com.mycorrhizal.crm.model.network.BackendError
import com.mycorrhizal.crm.model.network.BirthdaysResponse
import com.mycorrhizal.crm.model.network.CadencePoliciesResponse
import com.mycorrhizal.crm.model.network.CadencePolicy
import com.mycorrhizal.crm.model.network.CadencePolicyInput
import com.mycorrhizal.crm.model.network.ContactBriefing
import com.mycorrhizal.crm.model.network.CreateCadencePolicyResponse
import com.mycorrhizal.crm.model.network.OverdueCadencesResponse
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.CircleDetailResponse
import com.mycorrhizal.crm.model.network.CircleInput
import com.mycorrhizal.crm.model.network.CircleMember
import com.mycorrhizal.crm.model.network.CircleMemberInput
import com.mycorrhizal.crm.model.network.CirclesPage
import com.mycorrhizal.crm.model.network.ContactActivitiesResponse
import com.mycorrhizal.crm.model.network.ContactFieldValuesInput
import com.mycorrhizal.crm.model.network.ContactFieldValuesResponse
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
import com.mycorrhizal.crm.model.network.CreateConversationAgendaResponse
import com.mycorrhizal.crm.model.network.FieldDefinitionsResponse
import com.mycorrhizal.crm.model.network.CreateGiftResponse
import com.mycorrhizal.crm.model.network.CreateHouseholdResponse
import com.mycorrhizal.crm.model.network.CreateLifeEventResponse
import com.mycorrhizal.crm.model.network.CreateNoteResponse
import com.mycorrhizal.crm.model.network.CreatePreferenceResponse
import com.mycorrhizal.crm.model.network.CreateRelationshipEdgeResponse
import com.mycorrhizal.crm.model.network.CompletionsResponse
import com.mycorrhizal.crm.model.network.CreateReminderResponse
import com.mycorrhizal.crm.model.network.CreateTagResponse
import com.mycorrhizal.crm.model.network.ConversationAgenda
import com.mycorrhizal.crm.model.network.ConversationAgendaInput
import com.mycorrhizal.crm.model.network.ConversationAgendaPage
import com.mycorrhizal.crm.model.network.BulkContactOperationInput
import com.mycorrhizal.crm.model.network.BulkOperationResult
import com.mycorrhizal.crm.model.network.ContactMergeCommitResponse
import com.mycorrhizal.crm.model.network.ContactMergePreviewResponse
import com.mycorrhizal.crm.model.network.ContactMergeRequest
import com.mycorrhizal.crm.model.network.DashboardResponse
import com.mycorrhizal.crm.model.network.Gift
import com.mycorrhizal.crm.model.network.GiftInput
import com.mycorrhizal.crm.model.network.GiftsPage
import com.mycorrhizal.crm.model.network.Household
import com.mycorrhizal.crm.model.network.HouseholdDetailResponse
import com.mycorrhizal.crm.model.network.HouseholdInput
import com.mycorrhizal.crm.model.network.HouseholdMember
import com.mycorrhizal.crm.model.network.HouseholdMemberInput
import com.mycorrhizal.crm.model.network.HouseholdsPage
import com.mycorrhizal.crm.model.network.ImportConfirmRequest
import com.mycorrhizal.crm.model.network.ImportPreviewResponse
import com.mycorrhizal.crm.model.network.ImportRecordsRequest
import com.mycorrhizal.crm.model.network.ImportResult
import com.mycorrhizal.crm.model.network.ImportUploadResponse
import com.mycorrhizal.crm.model.network.LifeEvent
import com.mycorrhizal.crm.model.network.LifeEventInput
import com.mycorrhizal.crm.model.network.LifeEventsPage
import com.mycorrhizal.crm.model.network.LoginRequest
import com.mycorrhizal.crm.model.network.LoginResponse
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.model.network.NoteInput
import com.mycorrhizal.crm.model.network.NotesPage
import com.mycorrhizal.crm.model.network.Preference
import com.mycorrhizal.crm.model.network.PreferenceInput
import com.mycorrhizal.crm.model.network.PreferencesPage
import com.mycorrhizal.crm.model.network.RelationshipEdge
import com.mycorrhizal.crm.model.network.RelationshipEdgeInput
import com.mycorrhizal.crm.model.network.RelationshipEdgesPage
import com.mycorrhizal.crm.model.network.Reminder
import com.mycorrhizal.crm.model.network.ReminderCompleteResponse
import com.mycorrhizal.crm.model.network.SearchResult
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
import okhttp3.MultipartBody
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
        vcardUids: List<String>? = null,
    ): Result<ContactsPage> {
        val urlBuilder = "$PLACEHOLDER_ORIGIN$CONTACTS_PATH".toHttpUrl().newBuilder()
        if (!vcardUids.isNullOrEmpty()) {
            // ?vcard_uid= (repeatable) short-circuits the backend's whole
            // search/sort/pagination path (contact_controller.go), so don't
            // send cursor/limit/search alongside it -- they'd be silently
            // ignored server-side, and sending them here would be misleading.
            vcardUids.forEach { urlBuilder.addQueryParameter("vcard_uid", it) }
            includeArchived?.let { urlBuilder.addQueryParameter("include_archived", it.toString()) }
        } else {
            cursor?.let { urlBuilder.addQueryParameter("cursor", it) }
            limit?.let { urlBuilder.addQueryParameter("limit", it.toString()) }
            search?.let { urlBuilder.addQueryParameter("search", it) }
            includeArchived?.let { urlBuilder.addQueryParameter("include_archived", it.toString()) }
        }
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
     * GET /api/v1/contacts/{id}/briefing — the N2 prep-view composite (M11):
     * everything the user wants to remember before seeing a person in one
     * response. N2's backend does all the assembly; this is a read. The six
     * collection blocks are normalized in [ContactBriefing] (absent/null/[]
     * all decode to an empty list), so the screen can dereference `.size`
     * unconditionally — the exact contract regression that crashed web's prep
     * view into its ErrorBoundary (`/CLAUDE.md` frontend trap #8).
     */
    suspend fun getBriefing(contactId: Int): Result<ContactBriefing> =
        executeGet("$PLACEHOLDER_ORIGIN$CONTACTS_PATH/$contactId/briefing") { _, body ->
            moshi.adapter(ContactBriefing::class.java).fromJson(body)
        }

    /**
     * GET /api/v1/dashboard — the M3 "today/overview" composite (M10): one
     * call replacing the four-request fan-out the dashboard used to fire
     * (`listUpcomingBirthdays`, `listUpcomingReminders`,
     * `listOverdueCadences`, plus a per-reminder contact lookup). The
     * backend embeds each reminder's contact display name (M3 design
     * decision 2), so no second fetch is needed. All four blocks are
     * normalized to `[]` server-side; see [DashboardResponse]'s doc comment.
     */
    suspend fun getDashboard(): Result<DashboardResponse> =
        executeGet("$PLACEHOLDER_ORIGIN$DASHBOARD_PATH") { _, body ->
            moshi.adapter(DashboardResponse::class.java).fromJson(body)
        }

    /**
     * GET /api/v1/search — cross-entity FTS across notes and activities (T87: folded into the
     * contact list rather than a dedicated search screen). `q`'s two-character gate is the
     * backend's own; callers should apply it too rather than firing a request destined to
     * return empty. The response's `contacts` group is deliberately unmodeled — see
     * [SearchResult]'s doc comment.
     */
    suspend fun search(q: String, limit: Int? = null, householdId: String? = null): Result<SearchResult> {
        val urlBuilder = "$PLACEHOLDER_ORIGIN$SEARCH_PATH".toHttpUrl().newBuilder()
        urlBuilder.addQueryParameter("q", q)
        limit?.let { urlBuilder.addQueryParameter("limit", it.toString()) }
        householdId?.let { urlBuilder.addQueryParameter("household_id", it) }
        return executeGet(urlBuilder.build().toString()) { _, body ->
            moshi.adapter(SearchResult::class.java).fromJson(body)
        }
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

    // M24: top-level contact actions (delete/archive/unarchive/export). The
    // endpoints all pre-date the Android client (route table in
    // backend/routes/routes.go) — the gap this ticket closes is the missing
    // client surface, not the backend.

    /** DELETE /api/v1/contacts/{id} — soft-delete; the row stays for undo (T60/M16). */
    suspend fun deleteContact(id: Int): Result<Unit> =
        executeDelete("$PLACEHOLDER_ORIGIN$CONTACTS_PATH/$id")

    /** POST /api/v1/contacts/{id}/archive — archives the contact and retires its reminders. */
    suspend fun archiveContact(id: Int): Result<Unit> =
        executePostEmpty("$CONTACTS_PATH/$id/archive") { _, _ -> Unit }

    /** POST /api/v1/contacts/{id}/unarchive — restores an archived contact. */
    suspend fun unarchiveContact(id: Int): Result<Unit> =
        executePostEmpty("$CONTACTS_PATH/$id/unarchive") { _, _ -> Unit }

    /**
     * GET /api/v1/export/vcf?vcard_uid=… — exports a single contact as vCard
     * (4.0 default, 3.0 when [version] == 3), honoring the backend's default
     * field selection (all sections, private/secret sensitivity excluded).
     * Returns the raw file bytes; see `executeGetBytes`.
     */
    suspend fun exportContactVcf(vcardUid: String, version: Int? = null): Result<ByteArray> {
        val urlBuilder = "$PLACEHOLDER_ORIGIN$EXPORT_VCF_PATH".toHttpUrl().newBuilder()
        urlBuilder.addQueryParameter("vcard_uid", vcardUid)
        if (version == 3) urlBuilder.addQueryParameter("version", "3")
        return executeGetBytes(urlBuilder.build().toString())
    }

    /** GET /api/v1/field-definitions (T84). */
    suspend fun listFieldDefinitions(limit: Int? = null): Result<FieldDefinitionsResponse> {
        val urlBuilder = "$PLACEHOLDER_ORIGIN$FIELD_DEFINITIONS_PATH".toHttpUrl().newBuilder()
        limit?.let { urlBuilder.addQueryParameter("limit", it.toString()) }
        return executeGet(urlBuilder.build().toString()) { _, body ->
            moshi.adapter(FieldDefinitionsResponse::class.java).fromJson(body)
        }
    }

    /** GET /api/v1/contacts/{id}/field-values (T84). */
    suspend fun listContactFieldValues(contactId: Int): Result<ContactFieldValuesResponse> =
        executeGet("$PLACEHOLDER_ORIGIN$CONTACTS_PATH/$contactId/field-values") { _, body ->
            moshi.adapter(ContactFieldValuesResponse::class.java).fromJson(body)
        }

    /**
     * PUT /api/v1/contacts/{id}/field-values (T84) — full-replace; see
     * [ContactFieldValuesInput]'s doc comment. No UI calls this yet (T84 ships the read-only
     * slice); it exists for the round-trip test and so the write path isn't a second ticket.
     */
    suspend fun replaceContactFieldValues(
        contactId: Int,
        input: ContactFieldValuesInput,
    ): Result<ContactFieldValuesResponse> =
        executePut("$PLACEHOLDER_ORIGIN$CONTACTS_PATH/$contactId/field-values", input) { _, body ->
            moshi.adapter(ContactFieldValuesResponse::class.java).fromJson(body)
        }

    /**
     * GET /api/v1/contacts/{id}/activities — a contact's activities (M19:
     * T17 cursor-paginated; [search]/[fromDate]/[toDate] filter server-side).
     */
    suspend fun listContactActivities(
        contactId: Int,
        cursor: String? = null,
        limit: Int? = null,
        search: String? = null,
        fromDate: String? = null,
        toDate: String? = null,
    ): Result<ContactActivitiesResponse> {
        val urlBuilder = "$PLACEHOLDER_ORIGIN$CONTACTS_PATH/$contactId/activities".toHttpUrl().newBuilder()
        cursor?.let { urlBuilder.addQueryParameter("cursor", it) }
        limit?.let { urlBuilder.addQueryParameter("limit", it.toString()) }
        search?.takeIf { it.isNotBlank() }?.let { urlBuilder.addQueryParameter("search", it) }
        fromDate?.takeIf { it.isNotBlank() }?.let { urlBuilder.addQueryParameter("fromDate", it) }
        toDate?.takeIf { it.isNotBlank() }?.let { urlBuilder.addQueryParameter("toDate", it) }
        return executeGet(urlBuilder.build().toString()) { _, body ->
            moshi.adapter(ContactActivitiesResponse::class.java).fromJson(body)
        }
    }

    /** DELETE /api/v1/activities/{id} — soft-deletes an activity (M19). */
    suspend fun deleteActivity(id: Int): Result<Unit> =
        executeDelete("$PLACEHOLDER_ORIGIN$ACTIVITIES_PATH/$id")

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

    /**
     * GET /api/v1/activities (cursor-paginated, all activities). [includeContacts] appends
     * `?include=contacts` — matches `GetActivities`' `c.DefaultQuery("include", "")` check —
     * so the M9 Activities inbox can show each activity's participants.
     */
    suspend fun listActivities(
        cursor: String? = null,
        limit: Int? = null,
        includeContacts: Boolean = false,
    ): Result<ActivitiesPage> {
        val urlBuilder = "$PLACEHOLDER_ORIGIN$ACTIVITIES_PATH".toHttpUrl().newBuilder()
        cursor?.let { urlBuilder.addQueryParameter("cursor", it) }
        limit?.let { urlBuilder.addQueryParameter("limit", it.toString()) }
        if (includeContacts) urlBuilder.addQueryParameter("include", "contacts")
        return executeGet(urlBuilder.build().toString()) { _, body ->
            moshi.adapter(ActivitiesPage::class.java).fromJson(body)
        }
    }

    /** GET /api/v1/notes — the N4 unfiled-notes inbox (M9 Notes drawer entry), cursor-paginated. */
    suspend fun listNotes(cursor: String? = null, limit: Int? = null): Result<NotesPage> {
        val urlBuilder = "$PLACEHOLDER_ORIGIN$NOTES_PATH".toHttpUrl().newBuilder()
        cursor?.let { urlBuilder.addQueryParameter("cursor", it) }
        limit?.let { urlBuilder.addQueryParameter("limit", it.toString()) }
        return executeGet(urlBuilder.build().toString()) { _, body ->
            moshi.adapter(NotesPage::class.java).fromJson(body)
        }
    }

    /**
     * GET /api/v1/contacts/{id}/notes — a contact's notes (M19: T17
     * cursor-paginated; [search]/[fromDate]/[toDate] filter server-side).
     */
    suspend fun listContactNotes(
        contactId: Int,
        cursor: String? = null,
        limit: Int? = null,
        search: String? = null,
        fromDate: String? = null,
        toDate: String? = null,
    ): Result<ContactNotesResponse> {
        val urlBuilder = "$PLACEHOLDER_ORIGIN$CONTACTS_PATH/$contactId/notes".toHttpUrl().newBuilder()
        cursor?.let { urlBuilder.addQueryParameter("cursor", it) }
        limit?.let { urlBuilder.addQueryParameter("limit", it.toString()) }
        search?.takeIf { it.isNotBlank() }?.let { urlBuilder.addQueryParameter("search", it) }
        fromDate?.takeIf { it.isNotBlank() }?.let { urlBuilder.addQueryParameter("fromDate", it) }
        toDate?.takeIf { it.isNotBlank() }?.let { urlBuilder.addQueryParameter("toDate", it) }
        return executeGet(urlBuilder.build().toString()) { _, body ->
            moshi.adapter(ContactNotesResponse::class.java).fromJson(body)
        }
    }

    /** DELETE /api/v1/notes/{id} — soft-deletes a note (M19). */
    suspend fun deleteNote(id: Int): Result<Unit> =
        executeDelete("$PLACEHOLDER_ORIGIN$NOTES_PATH/$id")

    /** POST /api/v1/contacts/{id}/notes — wrapped `{ message, note }`, unwrapped here. */
    suspend fun createNote(contactId: Int, input: NoteInput): Result<Note> =
        executePost("$CONTACTS_PATH/$contactId/notes", input) { _, body ->
            moshi.adapter(CreateNoteResponse::class.java).fromJson(body)?.note
        }

    /** POST /api/v1/notes — create an unassigned note (contact_id honored, unlike the nested route). */
    suspend fun createUnassignedNote(input: NoteInput): Result<Note> =
        executePost(NOTES_PATH, input) { _, body ->
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

    /**
     * POST /api/v1/reminders/{id}/complete — completes a reminder (no body).
     * [skip] is the M10 skip path: `?skip=true` reschedules recurring
     * reminders without recording completion in the timeline (the web
     * confirms before calling it — match that at the call site).
     */
    suspend fun completeReminder(id: Int, skip: Boolean = false): Result<ReminderCompleteResponse> {
        val path = if (skip) "$REMINDERS_PATH/$id/complete?skip=true" else "$REMINDERS_PATH/$id/complete"
        return executePostEmpty(path) { _, body ->
            moshi.adapter(ReminderCompleteResponse::class.java).fromJson(body)
        }
    }

    /** GET /api/v1/reminders — all reminders for the user. */
    suspend fun listReminders(): Result<ContactRemindersResponse> =
        executeGet("$PLACEHOLDER_ORIGIN$REMINDERS_PATH") { _, body ->
            moshi.adapter(ContactRemindersResponse::class.java).fromJson(body)
        }

    /** GET /api/v1/reminders/upcoming — reminders due within ~7 days. */
    suspend fun listUpcomingReminders(): Result<ContactRemindersResponse> =
        executeGet("$PLACEHOLDER_ORIGIN$REMINDERS_PATH/upcoming") { _, body ->
            moshi.adapter(ContactRemindersResponse::class.java).fromJson(body)
        }

    /** GET /api/v1/reminders/{id} — a single reminder. */
    suspend fun getReminder(id: Int): Result<Reminder> =
        executeGet("$PLACEHOLDER_ORIGIN$REMINDERS_PATH/$id") { _, body ->
            moshi.adapter(Reminder::class.java).fromJson(body)
        }

    /** DELETE /api/v1/reminders/{id} — delete a reminder. */
    suspend fun deleteReminder(id: Int): Result<Unit> =
        executeDelete("$PLACEHOLDER_ORIGIN$REMINDERS_PATH/$id")

    /** GET /api/v1/contacts/{id}/reminder-completions — a contact's completion timeline. */
    suspend fun listContactReminderCompletions(contactId: Int): Result<CompletionsResponse> =
        executeGet("$PLACEHOLDER_ORIGIN$CONTACTS_PATH/$contactId/reminder-completions") { _, body ->
            moshi.adapter(CompletionsResponse::class.java).fromJson(body)
        }

    /** DELETE /api/v1/reminder-completions/{id} — remove a completion (undo). */
    suspend fun deleteReminderCompletion(id: Int): Result<Unit> =
        executeDelete("$PLACEHOLDER_ORIGIN$REMINDER_COMPLETIONS_PATH/$id")

    /** GET /api/v1/contacts/birthdays — upcoming birthdays. */
    suspend fun listUpcomingBirthdays(): Result<BirthdaysResponse> =
        executeGet("$PLACEHOLDER_ORIGIN$CONTACTS_PATH/birthdays") { _, body ->
            moshi.adapter(BirthdaysResponse::class.java).fromJson(body)
        }

    /** GET /api/v1/cadence-policies/overdue — overdue cadences. */
    suspend fun listOverdueCadences(): Result<OverdueCadencesResponse> =
        executeGet("$PLACEHOLDER_ORIGIN$CADENCE_POLICIES_PATH/overdue") { _, body ->
            moshi.adapter(OverdueCadencesResponse::class.java).fromJson(body)
        }

    /**
     * GET /api/v1/cadence-policies?entity_id=… — a contact's policies
     * (0 or 1, server-enforced). `entity_id` is the Contact.VCardUID.
     */
    suspend fun listCadencePolicies(entityId: String): Result<CadencePoliciesResponse> {
        val url = "$PLACEHOLDER_ORIGIN$CADENCE_POLICIES_PATH".toHttpUrl().newBuilder()
            .addQueryParameter("entity_id", entityId)
            .build()
        return executeGet(url.toString()) { _, body ->
            moshi.adapter(CadencePoliciesResponse::class.java).fromJson(body)
        }
    }

    /** GET /api/v1/cadence-policies/{id} — raw (unwrapped) policy with health. */
    suspend fun getCadencePolicy(id: String): Result<CadencePolicy> =
        executeGet("$PLACEHOLDER_ORIGIN$CADENCE_POLICIES_PATH/$id") { _, body ->
            moshi.adapter(CadencePolicy::class.java).fromJson(body)
        }

    /** POST /api/v1/cadence-policies — wrapped `{ cadence_policy }`, unwrapped here. */
    suspend fun createCadencePolicy(input: CadencePolicyInput): Result<CadencePolicy> =
        executePost(CADENCE_POLICIES_PATH, input) { _, body ->
            moshi.adapter(CreateCadencePolicyResponse::class.java).fromJson(body)?.cadencePolicy
        }

    /**
     * PUT /api/v1/cadence-policies/{id} — raw (unwrapped) updated policy,
     * unlike create's wrapped response (deliberate backend asymmetry).
     */
    suspend fun updateCadencePolicy(id: String, input: CadencePolicyInput): Result<CadencePolicy> =
        executePut("$PLACEHOLDER_ORIGIN$CADENCE_POLICIES_PATH/$id", input) { _, body ->
            moshi.adapter(CadencePolicy::class.java).fromJson(body)
        }

    /** DELETE /api/v1/cadence-policies/{id} — `{ message }`. */
    suspend fun deleteCadencePolicy(id: String): Result<Unit> =
        executeDelete("$PLACEHOLDER_ORIGIN$CADENCE_POLICIES_PATH/$id")

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

    /** POST /api/v1/households/{id}/suggest-relationships — trigger the relationship-suggestion engine. */
    suspend fun suggestHouseholdRelationships(id: String): Result<SuggestRelationshipsResponse> =
        executePostEmpty("$HOUSEHOLDS_PATH/$id/suggest-relationships") { _, body ->
            moshi.adapter(SuggestRelationshipsResponse::class.java).fromJson(body)
        }

    /** POST /api/v1/households/suggest-addresses — T40 shared-address scan (read-only, idempotent). */
    suspend fun suggestAddressHouseholds(): Result<AddressSuggestionsResponse> =
        executePostEmpty("$HOUSEHOLDS_PATH/suggest-addresses") { _, body ->
            moshi.adapter(AddressSuggestionsResponse::class.java).fromJson(body)
        }

    /** POST /api/v1/households/suggestions/accept — materialize a household from a suggested group; unwrapped `{ household }`. */
    suspend fun acceptHouseholdSuggestion(input: AcceptHouseholdSuggestionInput): Result<Household> =
        executePost("$HOUSEHOLDS_PATH/suggestions/accept", input) { _, body ->
            moshi.adapter(AcceptHouseholdSuggestionResponse::class.java).fromJson(body)?.household
        }

    /** POST /api/v1/households/suggestions/dismiss — permanently dismiss a suggested group. */
    suspend fun dismissHouseholdSuggestion(input: DismissHouseholdSuggestionInput): Result<Unit> =
        executePost("$HOUSEHOLDS_PATH/suggestions/dismiss", input) { _, _ -> Unit }

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

    /** PUT /api/v1/relationship-edges/{id} — raw (unwrapped) updated edge, unlike create's wrapped response. */
    suspend fun updateRelationshipEdge(id: String, input: RelationshipEdgeInput): Result<RelationshipEdge> =
        executePut("$PLACEHOLDER_ORIGIN$RELATIONSHIP_EDGES_PATH/$id", input) { _, body ->
            moshi.adapter(RelationshipEdge::class.java).fromJson(body)
        }

    /** PATCH /api/v1/relationship-edges/{id}/accept — promotes a suggestion; raw edge. */
    suspend fun acceptRelationshipEdge(id: String): Result<RelationshipEdge> =
        executePatchEmpty("$PLACEHOLDER_ORIGIN$RELATIONSHIP_EDGES_PATH/$id/accept") { _, body ->
            moshi.adapter(RelationshipEdge::class.java).fromJson(body)
        }

    /** DELETE /api/v1/relationship-edges/{id} — doubles as reject for suggestions. */
    suspend fun deleteRelationshipEdge(id: String): Result<Unit> =
        executeDelete("$PLACEHOLDER_ORIGIN$RELATIONSHIP_EDGES_PATH/$id")

    // --- Life events ---

    suspend fun listLifeEvents(
        entityId: String? = null,
        cursor: String? = null,
        limit: Int? = null,
    ): Result<LifeEventsPage> {
        val urlBuilder = "$PLACEHOLDER_ORIGIN$LIFE_EVENTS_PATH".toHttpUrl().newBuilder()
        entityId?.let { urlBuilder.addQueryParameter("entity_id", it) }
        cursor?.let { urlBuilder.addQueryParameter("cursor", it) }
        limit?.let { urlBuilder.addQueryParameter("limit", it.toString()) }
        return executeGet(urlBuilder.build().toString()) { _, body ->
            moshi.adapter(LifeEventsPage::class.java).fromJson(body)
        }
    }

    suspend fun createLifeEvent(input: LifeEventInput): Result<LifeEvent> =
        executePost(LIFE_EVENTS_PATH, input) { _, body ->
            moshi.adapter(CreateLifeEventResponse::class.java).fromJson(body)?.lifeEvent
        }

    suspend fun updateLifeEvent(id: String, input: LifeEventInput): Result<LifeEvent> =
        executePut("$PLACEHOLDER_ORIGIN$LIFE_EVENTS_PATH/$id", input) { _, body ->
            moshi.adapter(LifeEvent::class.java).fromJson(body)
        }

    suspend fun deleteLifeEvent(id: String): Result<Unit> =
        executeDelete("$PLACEHOLDER_ORIGIN$LIFE_EVENTS_PATH/$id")

    // --- Gifts ---

    suspend fun listGifts(
        entityId: String? = null,
        cursor: String? = null,
        limit: Int? = null,
    ): Result<GiftsPage> {
        val urlBuilder = "$PLACEHOLDER_ORIGIN$GIFTS_PATH".toHttpUrl().newBuilder()
        entityId?.let { urlBuilder.addQueryParameter("entity_id", it) }
        cursor?.let { urlBuilder.addQueryParameter("cursor", it) }
        limit?.let { urlBuilder.addQueryParameter("limit", it.toString()) }
        return executeGet(urlBuilder.build().toString()) { _, body ->
            moshi.adapter(GiftsPage::class.java).fromJson(body)
        }
    }

    suspend fun createGift(input: GiftInput): Result<Gift> =
        executePost(GIFTS_PATH, input) { _, body ->
            moshi.adapter(CreateGiftResponse::class.java).fromJson(body)?.gift
        }

    suspend fun updateGift(id: String, input: GiftInput): Result<Gift> =
        executePut("$PLACEHOLDER_ORIGIN$GIFTS_PATH/$id", input) { _, body ->
            moshi.adapter(Gift::class.java).fromJson(body)
        }

    suspend fun deleteGift(id: String): Result<Unit> =
        executeDelete("$PLACEHOLDER_ORIGIN$GIFTS_PATH/$id")

    // --- Preferences ---

    suspend fun listPreferences(
        entityId: String? = null,
        cursor: String? = null,
        limit: Int? = null,
    ): Result<PreferencesPage> {
        val urlBuilder = "$PLACEHOLDER_ORIGIN$PREFERENCES_PATH".toHttpUrl().newBuilder()
        entityId?.let { urlBuilder.addQueryParameter("entity_id", it) }
        cursor?.let { urlBuilder.addQueryParameter("cursor", it) }
        limit?.let { urlBuilder.addQueryParameter("limit", it.toString()) }
        return executeGet(urlBuilder.build().toString()) { _, body ->
            moshi.adapter(PreferencesPage::class.java).fromJson(body)
        }
    }

    suspend fun createPreference(input: PreferenceInput): Result<Preference> =
        executePost(PREFERENCES_PATH, input) { _, body ->
            moshi.adapter(CreatePreferenceResponse::class.java).fromJson(body)?.preference
        }

    suspend fun updatePreference(id: String, input: PreferenceInput): Result<Preference> =
        executePut("$PLACEHOLDER_ORIGIN$PREFERENCES_PATH/$id", input) { _, body ->
            moshi.adapter(Preference::class.java).fromJson(body)
        }

    suspend fun deletePreference(id: String): Result<Unit> =
        executeDelete("$PLACEHOLDER_ORIGIN$PREFERENCES_PATH/$id")

    // --- Conversation agenda ---

    suspend fun listConversationAgenda(
        entityId: String? = null,
        cursor: String? = null,
        limit: Int? = null,
    ): Result<ConversationAgendaPage> {
        val urlBuilder = "$PLACEHOLDER_ORIGIN$CONVERSATION_AGENDA_PATH".toHttpUrl().newBuilder()
        entityId?.let { urlBuilder.addQueryParameter("entity_id", it) }
        cursor?.let { urlBuilder.addQueryParameter("cursor", it) }
        limit?.let { urlBuilder.addQueryParameter("limit", it.toString()) }
        return executeGet(urlBuilder.build().toString()) { _, body ->
            moshi.adapter(ConversationAgendaPage::class.java).fromJson(body)
        }
    }

    suspend fun createConversationAgenda(input: ConversationAgendaInput): Result<ConversationAgenda> =
        executePost(CONVERSATION_AGENDA_PATH, input) { _, body ->
            moshi.adapter(CreateConversationAgendaResponse::class.java).fromJson(body)?.conversationAgenda
        }

    suspend fun updateConversationAgenda(id: String, input: ConversationAgendaInput): Result<ConversationAgenda> =
        executePut("$PLACEHOLDER_ORIGIN$CONVERSATION_AGENDA_PATH/$id", input) { _, body ->
            moshi.adapter(ConversationAgenda::class.java).fromJson(body)
        }

    suspend fun deleteConversationAgenda(id: String): Result<Unit> =
        executeDelete("$PLACEHOLDER_ORIGIN$CONVERSATION_AGENDA_PATH/$id")

    /** PATCH /api/v1/conversation-agenda/{id}/discuss — marks an item discussed. */
    suspend fun discussConversationAgenda(id: String): Result<ConversationAgenda> =
        executePatchEmpty("$PLACEHOLDER_ORIGIN$CONVERSATION_AGENDA_PATH/$id/discuss") { _, body ->
            moshi.adapter(ConversationAgenda::class.java).fromJson(body)
        }

    // --- Contact merge ---

    suspend fun previewMerge(request: ContactMergeRequest): Result<ContactMergePreviewResponse> =
        executePost("$CONTACTS_PATH/merge/preview", request) { _, body ->
            moshi.adapter(ContactMergePreviewResponse::class.java).fromJson(body)
        }

    suspend fun commitMerge(request: ContactMergeRequest): Result<ContactRecordResponse> =
        executePost("$CONTACTS_PATH/merge", request) { _, body ->
            moshi.adapter(ContactMergeCommitResponse::class.java).fromJson(body)?.contact
        }

    // --- Bulk operations ---

    suspend fun bulkOperation(input: BulkContactOperationInput): Result<BulkOperationResult> =
        executePost("$CONTACTS_PATH/bulk", input) { _, body ->
            moshi.adapter(BulkOperationResult::class.java).fromJson(body)
        }

    // --- Import (CSV / VCF / JSContact) ---

    suspend fun uploadCsvImport(fileBytes: ByteArray, fileName: String): Result<ImportUploadResponse> =
        executeMultipartUpload("$CONTACTS_PATH/import/upload", fileBytes, fileName) { _, body ->
            moshi.adapter(ImportUploadResponse::class.java).fromJson(body)
        }

    suspend fun uploadVcfImport(fileBytes: ByteArray, fileName: String): Result<ImportPreviewResponse> =
        executeMultipartUpload("$CONTACTS_PATH/import/vcf/upload", fileBytes, fileName) { _, body ->
            moshi.adapter(ImportPreviewResponse::class.java).fromJson(body)
        }

    suspend fun previewCsvImport(request: ImportConfirmRequest): Result<ImportPreviewResponse> =
        executePost("$CONTACTS_PATH/import/preview", request) { _, body ->
            moshi.adapter(ImportPreviewResponse::class.java).fromJson(body)
        }

    suspend fun confirmImport(request: ImportConfirmRequest): Result<ImportResult> =
        executePost("$CONTACTS_PATH/import/confirm", request) { _, body ->
            moshi.adapter(ImportResult::class.java).fromJson(body)
        }

    /** M9 item 4: confirms a VCF import session — same request/response shape as [confirmImport]. */
    suspend fun confirmVcfImport(request: ImportConfirmRequest): Result<ImportResult> =
        executePost("$CONTACTS_PATH/import/vcf/confirm", request) { _, body ->
            moshi.adapter(ImportResult::class.java).fromJson(body)
        }

    /**
     * T96: starts a records-based import session from a batch of neutral
     * Card/CRM records (the device-contacts import path produces these via
     * DeviceContactMapper.toInput). Runs the same preview pipeline as a VCF
     * upload — validation, server-side duplicate detection with a merge diff,
     * within-batch duplicate detection — and is confirmed via
     * [confirmVcfImport].
     */
    suspend fun uploadImportRecords(records: List<ContactRecordInput>): Result<ImportPreviewResponse> =
        executePost("$CONTACTS_PATH/import/records", ImportRecordsRequest(records)) { _, body ->
            moshi.adapter(ImportPreviewResponse::class.java).fromJson(body)
        }

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

    private suspend fun <T> executeMultipartUpload(
        path: String,
        fileBytes: ByteArray,
        fileName: String,
        mapper: (okhttp3.Response, String) -> T?,
    ): Result<T> {
        val body = MultipartBody.Builder()
            .setType(MultipartBody.FORM)
            .addFormDataPart("file", fileName, fileBytes.toRequestBody("application/octet-stream".toMediaType()))
            .build()
        val request = Request.Builder()
            .url("$PLACEHOLDER_ORIGIN$path".toHttpUrl())
            .post(body)
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

    /**
     * GET for a binary/download response: reads the body as raw bytes rather
     * than a decoded string (a VCF export is a file, not JSON). Non-2xx still
     * parses the JSON error body via the normal path.
     */
    private suspend fun executeGetBytes(url: String): Result<ByteArray> =
        withContext(Dispatchers.IO) {
            try {
                val request = Request.Builder().url(url.toHttpUrl()).get().build()
                val response = okHttpClient.newCall(request).execute()
                response.use {
                    val body = it.body
                    if (!it.isSuccessful) {
                        val errorBody = body?.string().orEmpty()
                        return@withContext Result.failure(parseError(it.code, errorBody))
                    }
                    val bytes = body?.bytes()
                    if (bytes == null || bytes.isEmpty()) {
                        Result.failure(ApiError.Parse("Empty response body"))
                    } else {
                        Result.success(bytes)
                    }
                }
            } catch (e: Exception) {
                Result.failure(e.toApiError())
            }
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
        private const val SEARCH_PATH = "$API_V1/search"
        private const val FIELD_DEFINITIONS_PATH = "$API_V1/field-definitions"
        private const val ACTIVITIES_PATH = "$API_V1/activities"
        private const val NOTES_PATH = "$API_V1/notes"
        private const val REMINDERS_PATH = "$API_V1/reminders"
        private const val REMINDER_COMPLETIONS_PATH = "$API_V1/reminder-completions"
        private const val CIRCLES_PATH = "$API_V1/circles"
        private const val TAGS_PATH = "$API_V1/tags"
        private const val HOUSEHOLDS_PATH = "$API_V1/households"
        private const val RELATIONSHIP_EDGES_PATH = "$API_V1/relationship-edges"
        private const val LIFE_EVENTS_PATH = "$API_V1/life-events"
        private const val GIFTS_PATH = "$API_V1/gifts"
        private const val PREFERENCES_PATH = "$API_V1/preferences"
        private const val CONVERSATION_AGENDA_PATH = "$API_V1/conversation-agenda"
        private const val CADENCE_POLICIES_PATH = "$API_V1/cadence-policies"
        private const val DASHBOARD_PATH = "$API_V1/dashboard"
        private const val EXPORT_VCF_PATH = "$API_V1/export/vcf"
        private const val AUTH_COOKIE = "auth_token"    }
}

/** Successful login: the bearer JWT (captured from the httpOnly cookie) plus profile prefs. */
data class LoginResult(
    val token: String?,
    val language: String?,
    val dateFormat: String?,
)
