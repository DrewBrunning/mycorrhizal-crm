package com.mycorrhizal.crm.model.network

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class ContactFieldKeyTest {

    @Test
    fun `null stored value resolves to the default enabled set`() {
        assertEquals(DEFAULT_ENABLED_CONTACT_FIELDS, resolveEnabledFields(null))
    }

    @Test
    fun `explicit empty list resolves to an empty set, distinct from null`() {
        val resolved = resolveEnabledFields(emptyList())
        assertTrue(resolved.isEmpty())
        assertTrue(resolved != DEFAULT_ENABLED_CONTACT_FIELDS)
    }

    @Test
    fun `a populated list maps recognized wire keys back to their enum entries`() {
        val resolved = resolveEnabledFields(listOf("emails", "speakToAs", "cardNotes"))
        assertEquals(
            setOf(ContactFieldKey.EMAILS, ContactFieldKey.SPEAK_TO_AS, ContactFieldKey.CARD_NOTES),
            resolved,
        )
    }

    @Test
    fun `an unrecognized wire key is silently dropped rather than failing`() {
        val resolved = resolveEnabledFields(listOf("emails", "some_future_web_only_key"))
        assertEquals(setOf(ContactFieldKey.EMAILS), resolved)
    }

    @Test
    fun `fromWireKey round-trips every enum entry's own wireKey`() {
        ContactFieldKey.entries.forEach { key ->
            assertEquals(key, ContactFieldKey.fromWireKey(key.wireKey))
        }
    }

    @Test
    fun `fromWireKey returns null for an unknown token`() {
        assertNull(ContactFieldKey.fromWireKey("not-a-real-key"))
    }

    @Test
    fun `every key has exactly one group and groups cover the full render order`() {
        ContactFieldKey.entries.forEach { key ->
            assertTrue("$key must have a group", CONTACT_FIELD_GROUP.containsKey(key))
            assertTrue("$key's group must be in the render order", CONTACT_FIELD_GROUP[key] in CONTACT_FIELD_GROUP_ORDER)
        }
    }

    @Test
    fun `group assignments match web's CONTACT_FIELDS array field for field`() {
        // frontend/src/contactFields.ts is the source of truth — several of these are
        // counterintuitive (gender/birthday/anniversaries are personal, not name;
        // how_we_met/contact_information are mycorrhizal but work_information is work).
        val expected = mapOf(
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
        assertEquals(expected, CONTACT_FIELD_GROUP)
    }
}
