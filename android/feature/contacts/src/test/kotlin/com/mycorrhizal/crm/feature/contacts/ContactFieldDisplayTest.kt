package com.mycorrhizal.crm.feature.contacts

import com.mycorrhizal.crm.model.network.CRMEnvelope
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactFieldKey
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.Email
import com.mycorrhizal.crm.model.network.Name
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ContactFieldDisplayTest {

    private val empty = ContactRecordResponse(
        id = 1,
        card = Card(name = Name(full = "Dana")),
        crm = CRMEnvelope()
    )
    private val filled = ContactRecordResponse(
        id = 1,
        card = Card(
            name = Name(full = "Dana"),
            emails = listOf(Email(address = "dana@example.com"))
        ),
        crm = CRMEnvelope(howWeMet = "At work")
    )
    private val enabled = setOf(
        ContactFieldKey.EMAILS,
        ContactFieldKey.PHONES,
        ContactFieldKey.HOW_WE_MET,
        ContactFieldKey.NICKNAME
    )

    @Test
    fun `contactFieldHasValue distinguishes filled and empty fields`() {
        assertTrue(filled.contactFieldHasValue(ContactFieldKey.EMAILS))
        assertFalse(empty.contactFieldHasValue(ContactFieldKey.EMAILS))
        assertTrue(filled.contactFieldHasValue(ContactFieldKey.HOW_WE_MET))
        assertFalse(empty.contactFieldHasValue(ContactFieldKey.HOW_WE_MET))
        assertFalse(empty.contactFieldHasValue(ContactFieldKey.PHONES))
    }

    @Test
    fun `renderedFieldKeys keeps empty fields when hide-empty is off`() {
        assertEquals(
            enabled,
            empty.renderedFieldKeys(enabled, hideEmpty = false, revealed = emptySet())
        )
    }

    @Test
    fun `renderedFieldKeys suppresses empty fields when hide-empty is on`() {
        val rendered = empty.renderedFieldKeys(enabled, hideEmpty = true, revealed = emptySet())
        assertFalse(ContactFieldKey.EMAILS in rendered)
        assertFalse(ContactFieldKey.PHONES in rendered)
        // Header-only keys are outside the hide-empty scope.
        assertTrue(ContactFieldKey.NICKNAME in rendered)
    }

    @Test
    fun `renderedFieldKeys keeps filled fields and revealed empty fields`() {
        val rendered = filled.renderedFieldKeys(
            enabled,
            hideEmpty = true,
            revealed = setOf(ContactFieldKey.PHONES)
        )
        assertTrue(ContactFieldKey.EMAILS in rendered)
        assertTrue(ContactFieldKey.PHONES in rendered)
        assertTrue(ContactFieldKey.HOW_WE_MET in rendered)
    }

    @Test
    fun `hiddenEmptyFieldKeys lists only the suppressed empty fields`() {
        assertEquals(
            emptyList<ContactFieldKey>(),
            filled.hiddenEmptyFieldKeys(enabled, hideEmpty = false, revealed = emptySet())
        )
        assertEquals(
            listOf(ContactFieldKey.PHONES),
            filled.hiddenEmptyFieldKeys(enabled, hideEmpty = true, revealed = emptySet())
        )
        assertEquals(
            setOf(ContactFieldKey.PHONES, ContactFieldKey.EMAILS, ContactFieldKey.HOW_WE_MET),
            empty.hiddenEmptyFieldKeys(enabled, hideEmpty = true, revealed = emptySet()).toSet()
        )
    }

    @Test
    fun `revealing an empty field removes it from the hidden list`() {
        assertEquals(
            listOf(ContactFieldKey.HOW_WE_MET),
            empty.hiddenEmptyFieldKeys(
                enabled,
                hideEmpty = true,
                revealed = setOf(ContactFieldKey.EMAILS, ContactFieldKey.PHONES)
            )
        )
    }
}
