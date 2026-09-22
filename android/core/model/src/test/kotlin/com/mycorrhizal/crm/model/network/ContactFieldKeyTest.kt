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
}
