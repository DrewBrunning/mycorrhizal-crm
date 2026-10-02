package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

@JsonClass(generateAdapter = true)
data class NameComponent(
    val kind: String? = null,
    val value: String? = null,
    val phonetic: String? = null,
)

@JsonClass(generateAdapter = true)
data class Name(
    val components: List<NameComponent>? = null,
    val full: String? = null,
    @Json(name = "sortAs") val sortAs: Map<String, String>? = null,
    val isOrdered: Boolean? = null,
    val defaultSeparator: String? = null,
    val phoneticSystem: String? = null,
    val phoneticScript: String? = null,
)

@JsonClass(generateAdapter = true)
data class Nickname(
    val id: String? = null,
    val name: String? = null,
    val contexts: List<String>? = null,
    val pref: Int? = null,
)

@JsonClass(generateAdapter = true)
data class OrgUnit(
    val name: String? = null,
    val sortAs: String? = null,
)

@JsonClass(generateAdapter = true)
data class Organization(
    val id: String? = null,
    val name: String? = null,
    val units: List<OrgUnit>? = null,
    val sortAs: String? = null,
)

@JsonClass(generateAdapter = true)
data class Title(
    val id: String? = null,
    val name: String? = null,
    val kind: String? = null,
    val organizationId: String? = null,
)

@JsonClass(generateAdapter = true)
data class Email(
    val id: String? = null,
    val address: String? = null,
    val contexts: List<String>? = null,
    val pref: Int? = null,
    val label: String? = null,
)

@JsonClass(generateAdapter = true)
data class Phone(
    val id: String? = null,
    val number: String? = null,
    val features: List<String>? = null,
    val contexts: List<String>? = null,
    val pref: Int? = null,
    val label: String? = null,
)

@JsonClass(generateAdapter = true)
data class OnlineService(
    val id: String? = null,
    val service: String? = null,
    val uri: String? = null,
    val user: String? = null,
    val contexts: List<String>? = null,
    val pref: Int? = null,
    val label: String? = null,
)

@JsonClass(generateAdapter = true)
data class AddressComponent(
    val kind: String? = null,
    val value: String? = null,
    val phonetic: String? = null,
)

@JsonClass(generateAdapter = true)
data class Address(
    val id: String? = null,
    val components: List<AddressComponent>? = null,
    val countryCode: String? = null,
    val coordinates: String? = null,
    /**
     * ADR 0031: `normal` | `private` | `secret` (null = normal). A CRM-side
     * classification with no RFC 9553 home. Never shown or edited here, but it
     * MUST round-trip: Moshi drops unknown keys, so omitting it would erase it
     * on the next save. It gates the geocode lookup ([isGeocodable]).
     */
    val sensitivity: String? = null,
    val timeZone: String? = null,
    val contexts: List<String>? = null,
    val pref: Int? = null,
    val full: String? = null,
    val isOrdered: Boolean? = null,
    val defaultSeparator: String? = null,
    val phoneticSystem: String? = null,
    val phoneticScript: String? = null,
)

/**
 * True unless the address is `private`/`secret`: those are never sent to the
 * geocoder (the backend answers 400), so the client blocks the action up front
 * and says why instead of letting it fail (ADR 0031).
 */
val Address.isGeocodable: Boolean
    get() = sensitivity.isNullOrBlank() || sensitivity == "normal"

@JsonClass(generateAdapter = true)
data class GrammaticalGender(
    val id: String? = null,
    val value: String? = null,
    val language: String? = null,
)

@JsonClass(generateAdapter = true)
data class Pronouns(
    val id: String? = null,
    val pronouns: String? = null,
    val contexts: List<String>? = null,
    val pref: Int? = null,
)

@JsonClass(generateAdapter = true)
data class SpeakToAs(
    @Json(name = "grammaticalGenders") val grammaticalGenders: List<GrammaticalGender>? = null,
    val pronouns: List<Pronouns>? = null,
)

@JsonClass(generateAdapter = true)
data class PersonalInfo(
    val id: String? = null,
    val kind: String? = null,
    val value: String? = null,
    val level: String? = null,
    val listAs: Int? = null,
    val label: String? = null,
)

@JsonClass(generateAdapter = true)
data class Author(
    val name: String? = null,
    val uri: String? = null,
)

@JsonClass(generateAdapter = true)
data class CardNote(
    val id: String? = null,
    val note: String? = null,
    val author: Author? = null,
    val created: Timestamp? = null,
)

@JsonClass(generateAdapter = true)
data class Resource(
    val id: String? = null,
    val kind: String? = null,
    val uri: String? = null,
    val mediaType: String? = null,
    val label: String? = null,
    val contexts: List<String>? = null,
    val pref: Int? = null,
    val listAs: Int? = null,
)

@JsonClass(generateAdapter = true)
data class LanguagePref(
    val id: String? = null,
    val language: String? = null,
    val contexts: List<String>? = null,
    val pref: Int? = null,
)

@JsonClass(generateAdapter = true)
data class Relation(
    val target: String? = null,
    val relations: List<String>? = null,
)

@JsonClass(generateAdapter = true)
data class JCardProp(
    val name: String? = null,
    val params: Map<String, Any?>? = null,
    val type: String? = null,
    val value: Any? = null,
)
