package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/** GET /contacts/birthdays — `{ birthdays: [...] }`. */
@JsonClass(generateAdapter = true)
data class Birthday(
    val type: String = "contact",
    val name: String = "",
    val birthday: String? = null,
    @Json(name = "photo_thumbnail") val photoThumbnail: String? = null,
    @Json(name = "contact_id") val contactId: Long = 0,
)

@JsonClass(generateAdapter = true)
data class BirthdaysResponse(
    val birthdays: List<Birthday> = emptyList(),
)

/** GET /cadence-policies/overdue — `{ overdue: [...] }`. */
@JsonClass(generateAdapter = true)
data class OverdueCadence(
    val policy: CadencePolicy? = null,
    val health: CadenceHealth? = null,
    @Json(name = "contact_id") val contactId: Long = 0,
    @Json(name = "contact_name") val contactName: String = "",
    @Json(name = "photo_thumbnail") val photoThumbnail: String? = null,
)

@JsonClass(generateAdapter = true)
data class OverdueCadencesResponse(
    val overdue: List<OverdueCadence> = emptyList(),
)
