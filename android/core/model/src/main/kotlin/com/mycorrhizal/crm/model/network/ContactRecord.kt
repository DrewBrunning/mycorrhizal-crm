package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * Full neutral detail-view response for GET/POST/PUT /contacts — matches the
 * OpenAPI ContactRecordResponse. `notes`/`activities`/`reminders` are the
 * contact's sub-resource rows (the detail endpoint preloads them), so the
 * unified timeline is a pure client-side merge of these three arrays.
 */
@JsonClass(generateAdapter = true)
data class ContactRecordResponse(
    val id: Int = 0,
    val uid: String? = null,
    val etag: String? = null,
    val gender: String? = null,
    val card: Card? = null,
    val crm: CRMEnvelope? = null,
    val passthrough: Passthrough? = null,
    val photo: String? = null,
    @Json(name = "photo_thumbnail") val photoThumbnail: String? = null,
    val archived: Boolean = false,
    val notes: List<Note>? = null,
    val activities: List<Activity>? = null,
    val reminders: List<Reminder>? = null,
)

/** Neutral superset of a contact's standardized data (RFC 9553 JSContact + vCard registry). */
@JsonClass(generateAdapter = true)
data class Card(
    val uid: String? = null,
    val kind: String? = null,
    val language: String? = null,
    @Json(name = "prodId") val prodId: String? = null,
    val created: Timestamp? = null,
    val updated: Timestamp? = null,
    val name: Name? = null,
    val nicknames: List<Nickname>? = null,
    val organizations: List<Organization>? = null,
    val titles: List<Title>? = null,
    val emails: List<Email>? = null,
    val phones: List<Phone>? = null,
    @Json(name = "imppAddresses") val imppAddresses: List<OnlineService>? = null,
    @Json(name = "socialProfiles") val socialProfiles: List<OnlineService>? = null,
    @Json(name = "otherOnlineServices") val otherOnlineServices: List<OnlineService>? = null,
    val addresses: List<Address>? = null,
    val anniversaries: List<Anniversary>? = null,
    @Json(name = "speakToAs") val speakToAs: SpeakToAs? = null,
    @Json(name = "personalInfo") val personalInfo: List<PersonalInfo>? = null,
    val notes: List<CardNote>? = null,
    val keywords: List<String>? = null,
    val media: List<Resource>? = null,
    val calendars: List<Resource>? = null,
    @Json(name = "freeBusyUrls") val freeBusyUrls: List<Resource>? = null,
    @Json(name = "schedulingAddresses") val schedulingAddresses: List<Resource>? = null,
    @Json(name = "cryptoKeys") val cryptoKeys: List<Resource>? = null,
    val directories: List<Resource>? = null,
    val links: List<Resource>? = null,
    @Json(name = "contactUris") val contactUris: List<Resource>? = null,
    @Json(name = "preferredLanguages") val preferredLanguages: List<LanguagePref>? = null,
    @Json(name = "relatedTo") val relatedTo: List<Relation>? = null,
    val members: List<String>? = null,
    val localizations: Map<String, Any?>? = null,
) {
    val displayName: String
        get() {
            // Prefer given + surname components (the web's nameComponentValue
            // pattern) — `full` is often only the given name for CRM contacts.
            val components = name?.components.orEmpty()
            val given = components.firstOrNull { it.kind == "given" }?.value.orEmpty()
            val surname = components.firstOrNull { it.kind == "surname" }?.value.orEmpty()
            val joined = listOfNotNull(given, surname).joinToString(" ").trim()
            if (joined.isNotBlank()) return joined
            return name?.full?.takeIf { it.isNotBlank() } ?: "Contact"
        }

    /** The photo media entry's URI (kind=photo), if any — the detail endpoint
     *  populates card.media via buildMedia, so this is the reliable source. */
    val photoUri: String?
        get() = media?.firstOrNull { it.kind == "photo" }?.uri?.takeIf { it.isNotBlank() }
}

/** Mycorrhizal-specific data outside any contact-exchange standard. */
@JsonClass(generateAdapter = true)
data class CRMEnvelope(
    /**
     * Envelope-side entity kind (human|animal) — distinct from [Card.kind]'s
     * standard vCard/JSContact KIND (individual|group|org|…), which has no
     * pet/animal value. Mirrors backend contactmodel.CRMEnvelope.Kind (T27).
     */
    val kind: String? = null,
    val circles: List<String>? = null,
    @Json(name = "how_we_met") val howWeMet: String? = null,
    @Json(name = "work_information") val workInformation: String? = null,
    @Json(name = "contact_information") val contactInformation: String? = null,
)

/** Unmapped data preserved verbatim (passthrough). */
@JsonClass(generateAdapter = true)
data class Passthrough(
    @Json(name = "vCardProps") val vCardProps: List<JCardProp>? = null,
    @Json(name = "jsContactProps") val jsContactProps: Map<String, Any?>? = null,
)

@JsonClass(generateAdapter = true)
data class Timestamp(
    val utc: String? = null,
)

/** Partial date: `{ year: 1990, month: 6, day: 15 }`, yearless, or year-only. */
@JsonClass(generateAdapter = true)
data class PartialDate(
    val year: Int? = null,
    val month: Int? = null,
    val day: Int? = null,
    val calendarScale: String? = null,
) {
    val hasMonthDay: Boolean get() = month != null && day != null
    val isYearOnly: Boolean get() = year != null && month == null
}

@JsonClass(generateAdapter = true)
data class AnniversaryDate(
    val partial: PartialDate? = null,
    val timestamp: String? = null,
)

@JsonClass(generateAdapter = true)
data class Anniversary(
    val id: String? = null,
    val kind: String? = null,
    val date: AnniversaryDate? = null,
    val place: Address? = null,
)
