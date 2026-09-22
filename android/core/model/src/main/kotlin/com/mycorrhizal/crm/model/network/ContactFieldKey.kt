package com.mycorrhizal.crm.model.network

/**
 * Issue #832 (web parity with `frontend/src/contactFields.ts`): every extended
 * contact field the "Contact field settings" screen can toggle show/edit for.
 * [wireKey] is the exact string web/Android PATCH to `/users/enabled-contact-fields`
 * and read back from it and from `GET /users/me`'s `enabled_contact_fields` —
 * these MUST stay byte-identical to web's `ContactFieldKey` union, the same
 * hand-mirrored-list convention as `MultiValueEditor.kt`'s type-option lists
 * (`/CLAUDE.md` frontend trap #4: no dynamic type-list endpoint exists).
 *
 * `firstname`/`lastname` (givenName/surname on Android) are deliberately never
 * represented here — matching web, they are never toggleable.
 *
 * Android additionally has no UI for the standalone `anniversary` (singular)
 * scalar shortcut web offers — Android's `anniversaries` key covers its one
 * general list editor for every non-birth anniversary; `anniversary` is kept
 * here only so a stored preference that includes it round-trips without being
 * dropped, but toggling it has no visible effect on Android today.
 */
enum class ContactFieldKey(val wireKey: String) {
    EMAILS("emails"),
    PHONES("phones"),
    ADDRESSES("addresses"),
    LINKS("links"),
    IMPP_ADDRESSES("imppAddresses"),
    SOCIAL_PROFILES("socialProfiles"),
    OTHER_ONLINE_SERVICES("otherOnlineServices"),
    NICKNAME("nickname"),
    GENDER("gender"),
    BIRTHDAY("birthday"),
    ANNIVERSARY("anniversary"),
    ANNIVERSARIES("anniversaries"),
    PREFIX("prefix"),
    MIDDLE_NAME("middle_name"),
    SUFFIX("suffix"),
    ORGANIZATIONS("organizations"),
    TITLES("titles"),
    HOW_WE_MET("how_we_met"),
    WORK_INFORMATION("work_information"),
    CONTACT_INFORMATION("contact_information"),
    SPEAK_TO_AS("speakToAs"),
    PERSONAL_INFO("personalInfo"),
    KEYWORDS("keywords"),
    CARD_NOTES("cardNotes"),
    PREFERRED_LANGUAGES("preferredLanguages"),
    CARD_KIND("cardKind"),
    LANGUAGE("language"),
    ;

    companion object {
        private val byWireKey = entries.associateBy { it.wireKey }

        /** Unrecognized wire keys (future web fields Android doesn't know about yet) are dropped. */
        fun fromWireKey(wireKey: String): ContactFieldKey? = byWireKey[wireKey]
    }
}

/** Render-order grouping — mirrors web's `CONTACT_FIELD_GROUPS` (`contactFields.ts`). */
enum class ContactFieldGroup {
    COMMUNICATION,
    NAME,
    WORK,
    PERSONAL,
    MYCORRHIZAL,
}

/**
 * Each key's group, in the same section a web toggle for it appears under —
 * copied field-for-field from web's `CONTACT_FIELDS` array, not derived by
 * guessing at a "sensible" grouping. Several of these are counterintuitive
 * (`gender`/`birthday`/`anniversaries` are `personal`, not `name`;
 * `how_we_met`/`contact_information` are `mycorrhizal` but `work_information`
 * is `work`) — a prior pass here got 8 of these wrong by working from a
 * research summary instead of `frontend/src/contactFields.ts` itself.
 */
val CONTACT_FIELD_GROUP: Map<ContactFieldKey, ContactFieldGroup> = mapOf(
    ContactFieldKey.EMAILS to ContactFieldGroup.COMMUNICATION,
    ContactFieldKey.PHONES to ContactFieldGroup.COMMUNICATION,
    ContactFieldKey.ADDRESSES to ContactFieldGroup.COMMUNICATION,
    ContactFieldKey.LINKS to ContactFieldGroup.COMMUNICATION,
    ContactFieldKey.IMPP_ADDRESSES to ContactFieldGroup.COMMUNICATION,
    ContactFieldKey.SOCIAL_PROFILES to ContactFieldGroup.COMMUNICATION,
    ContactFieldKey.OTHER_ONLINE_SERVICES to ContactFieldGroup.COMMUNICATION,
    ContactFieldKey.PREFIX to ContactFieldGroup.NAME,
    ContactFieldKey.MIDDLE_NAME to ContactFieldGroup.NAME,
    ContactFieldKey.SUFFIX to ContactFieldGroup.NAME,
    ContactFieldKey.NICKNAME to ContactFieldGroup.NAME,
    ContactFieldKey.ORGANIZATIONS to ContactFieldGroup.WORK,
    ContactFieldKey.TITLES to ContactFieldGroup.WORK,
    ContactFieldKey.WORK_INFORMATION to ContactFieldGroup.WORK,
    ContactFieldKey.GENDER to ContactFieldGroup.PERSONAL,
    ContactFieldKey.BIRTHDAY to ContactFieldGroup.PERSONAL,
    ContactFieldKey.ANNIVERSARY to ContactFieldGroup.PERSONAL,
    ContactFieldKey.ANNIVERSARIES to ContactFieldGroup.PERSONAL,
    ContactFieldKey.SPEAK_TO_AS to ContactFieldGroup.PERSONAL,
    ContactFieldKey.PERSONAL_INFO to ContactFieldGroup.PERSONAL,
    ContactFieldKey.KEYWORDS to ContactFieldGroup.PERSONAL,
    ContactFieldKey.CARD_NOTES to ContactFieldGroup.PERSONAL,
    ContactFieldKey.PREFERRED_LANGUAGES to ContactFieldGroup.PERSONAL,
    ContactFieldKey.CARD_KIND to ContactFieldGroup.PERSONAL,
    ContactFieldKey.LANGUAGE to ContactFieldGroup.PERSONAL,
    ContactFieldKey.HOW_WE_MET to ContactFieldGroup.MYCORRHIZAL,
    ContactFieldKey.CONTACT_INFORMATION to ContactFieldGroup.MYCORRHIZAL,
)

/** Section render order — mirrors web's `CONTACT_FIELD_GROUPS` array order. */
val CONTACT_FIELD_GROUP_ORDER: List<ContactFieldGroup> = listOf(
    ContactFieldGroup.COMMUNICATION,
    ContactFieldGroup.NAME,
    ContactFieldGroup.WORK,
    ContactFieldGroup.PERSONAL,
    ContactFieldGroup.MYCORRHIZAL,
)

/**
 * The fields enabled by default — what existing users already saw before this
 * feature shipped. Everything else is opt-in. Verbatim port of web's
 * `DEFAULT_ENABLED_CONTACT_FIELDS`.
 */
val DEFAULT_ENABLED_CONTACT_FIELDS: Set<ContactFieldKey> = setOf(
    ContactFieldKey.EMAILS,
    ContactFieldKey.PHONES,
    ContactFieldKey.ADDRESSES,
    ContactFieldKey.NICKNAME,
    ContactFieldKey.GENDER,
    ContactFieldKey.BIRTHDAY,
    ContactFieldKey.SPEAK_TO_AS,
    ContactFieldKey.PERSONAL_INFO,
    ContactFieldKey.HOW_WE_MET,
    ContactFieldKey.WORK_INFORMATION,
    ContactFieldKey.CONTACT_INFORMATION,
)

/**
 * Resolve a stored `enabled_contact_fields` value (from `GET /users/me` or
 * `GET /users/enabled-contact-fields`) into the set of keys the UI should
 * show. Null (never configured) applies [DEFAULT_ENABLED_CONTACT_FIELDS];
 * a non-null list (including an explicitly empty one — "show nothing") maps
 * only its recognized wire keys, silently dropping ones Android doesn't (yet)
 * know about.
 */
fun resolveEnabledFields(stored: List<String>?): Set<ContactFieldKey> =
    if (stored == null) {
        DEFAULT_ENABLED_CONTACT_FIELDS
    } else {
        stored.mapNotNull { ContactFieldKey.fromWireKey(it) }.toSet()
    }
