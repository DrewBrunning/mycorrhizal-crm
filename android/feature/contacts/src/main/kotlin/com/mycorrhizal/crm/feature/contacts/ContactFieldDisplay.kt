package com.mycorrhizal.crm.feature.contacts

import com.mycorrhizal.crm.model.network.ContactFieldKey
import com.mycorrhizal.crm.model.network.ContactRecordResponse

/**
 * Issue #1997: detail-view display rules. A field is enabled by the resolved
 * visibility (per-contact override > account default > built-in default, all
 * resolved server-side). Hide-empty then suppresses an enabled field that has
 * no value; the "Add a field" menu reveals one for the current visit.
 *
 * Cosmetic only — never an access boundary. Mirrors web's
 * `frontend/src/contactFieldDisplay.ts`.
 */
val HIDE_EMPTY_FIELD_KEYS: Set<ContactFieldKey> = setOf(
    ContactFieldKey.BIRTHDAY,
    ContactFieldKey.PREFERRED_LANGUAGES,
    ContactFieldKey.KEYWORDS,
    ContactFieldKey.PHONES,
    ContactFieldKey.ADDRESSES,
    ContactFieldKey.EMAILS,
    ContactFieldKey.SOCIAL_PROFILES,
    ContactFieldKey.OTHER_ONLINE_SERVICES,
    ContactFieldKey.IMPP_ADDRESSES,
    ContactFieldKey.LINKS,
    ContactFieldKey.GENDER,
    ContactFieldKey.SPEAK_TO_AS,
    ContactFieldKey.CARD_NOTES,
    ContactFieldKey.HOW_WE_MET,
    ContactFieldKey.CONTACT_INFORMATION,
    ContactFieldKey.PERSONAL_INFO,
    ContactFieldKey.ORGANIZATIONS,
    ContactFieldKey.TITLES,
    ContactFieldKey.WORK_INFORMATION,
    ContactFieldKey.ANNIVERSARIES,
    ContactFieldKey.ANNIVERSARY
)

/** Whether this contact carries a value for the display field [key]. */
fun ContactRecordResponse.contactFieldHasValue(key: ContactFieldKey): Boolean {
    val card = card
    val crm = crm
    return when (key) {
        ContactFieldKey.GENDER -> !crm?.gender.isNullOrBlank()

        ContactFieldKey.BIRTHDAY -> card?.anniversaries?.any { it.kind == "birth" } == true

        ContactFieldKey.KEYWORDS -> !card?.keywords.isNullOrEmpty()

        ContactFieldKey.PREFERRED_LANGUAGES -> !card?.preferredLanguages.isNullOrEmpty()

        ContactFieldKey.PHONES -> !card?.phones.isNullOrEmpty()

        ContactFieldKey.ADDRESSES -> !card?.addresses.isNullOrEmpty()

        ContactFieldKey.EMAILS -> !card?.emails.isNullOrEmpty()

        ContactFieldKey.SOCIAL_PROFILES -> !card?.socialProfiles.isNullOrEmpty()

        ContactFieldKey.OTHER_ONLINE_SERVICES -> !card?.otherOnlineServices.isNullOrEmpty()

        ContactFieldKey.IMPP_ADDRESSES -> !card?.imppAddresses.isNullOrEmpty()

        ContactFieldKey.LINKS -> !card?.links.isNullOrEmpty()

        ContactFieldKey.CARD_NOTES -> !card?.notes.isNullOrEmpty()

        ContactFieldKey.HOW_WE_MET -> !crm?.howWeMet.isNullOrBlank()

        ContactFieldKey.CONTACT_INFORMATION -> !crm?.contactInformation.isNullOrBlank()

        ContactFieldKey.SPEAK_TO_AS -> {
            val speakToAs = card?.speakToAs
            !speakToAs?.pronouns.isNullOrEmpty() ||
                !speakToAs?.grammaticalGenders.isNullOrEmpty()
        }

        ContactFieldKey.PERSONAL_INFO -> !card?.personalInfo.isNullOrEmpty()

        ContactFieldKey.ORGANIZATIONS -> !card?.organizations.isNullOrEmpty()

        ContactFieldKey.TITLES -> !card?.titles.isNullOrEmpty()

        ContactFieldKey.WORK_INFORMATION -> !crm?.workInformation.isNullOrBlank()

        ContactFieldKey.ANNIVERSARIES, ContactFieldKey.ANNIVERSARY ->
            card?.anniversaries?.any { it.kind != "birth" } == true

        // Header-only keys are outside the hide-empty scope (web parity).
        else -> true
    }
}

/** Whether an enabled field is rendered right now. */
fun isContactFieldRendered(
    contact: ContactRecordResponse,
    key: ContactFieldKey,
    enabled: Set<ContactFieldKey>,
    hideEmpty: Boolean,
    revealed: Set<ContactFieldKey>
): Boolean {
    if (key !in enabled) return false
    if (!hideEmpty) return true
    if (key in revealed) return true
    if (key !in HIDE_EMPTY_FIELD_KEYS) return true
    return contact.contactFieldHasValue(key)
}

/** The enabled set after hide-empty (what the detail view renders). */
fun ContactRecordResponse.renderedFieldKeys(
    enabled: Set<ContactFieldKey>,
    hideEmpty: Boolean,
    revealed: Set<ContactFieldKey>
): Set<ContactFieldKey> = enabled.filterTo(mutableSetOf()) {
    isContactFieldRendered(this, it, enabled, hideEmpty, revealed)
}

/**
 * The enabled fields hide-empty is currently suppressing, in display order.
 * This is the "Add a field" menu's contents; empty when hide-empty is off.
 */
fun ContactRecordResponse.hiddenEmptyFieldKeys(
    enabled: Set<ContactFieldKey>,
    hideEmpty: Boolean,
    revealed: Set<ContactFieldKey>
): List<ContactFieldKey> {
    if (!hideEmpty) return emptyList()
    return HIDE_EMPTY_FIELD_KEYS.filter { key ->
        key in enabled && key !in revealed && !contactFieldHasValue(key)
    }
}
