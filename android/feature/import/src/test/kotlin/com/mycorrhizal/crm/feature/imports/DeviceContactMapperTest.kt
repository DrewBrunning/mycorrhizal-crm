package com.mycorrhizal.crm.feature.imports

import android.provider.ContactsContract.CommonDataKinds.Phone
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class DeviceContactMapperTest {

    @Test
    fun `full name splits into given and surname components`() {
        val device = DeviceContact(
            contactId = 1,
            lookupKey = "lk1",
            displayName = "Dana White",
            phones = emptyList(),
            emails = emptyList(),
            addresses = emptyList(),
            organization = null,
            birthday = null,
        )

        val input = DeviceContactMapper.toInput(device)

        val name = input.card?.name
        assertEquals("Dana White", name?.full)
        assertEquals("given", name?.components?.get(0)?.kind)
        assertEquals("Dana", name?.components?.get(0)?.value)
        assertEquals("surname", name?.components?.get(1)?.kind)
        assertEquals("White", name?.components?.get(1)?.value)
    }

    @Test
    fun `mobile phone carries the cell feature`() {
        val device = DeviceContact(
            contactId = 1,
            lookupKey = "lk1",
            displayName = "Dana",
            phones = listOf("+15550100" to Phone.TYPE_MOBILE),
            emails = emptyList(),
            addresses = emptyList(),
            organization = null,
            birthday = null,
        )

        val input = DeviceContactMapper.toInput(device)

        assertEquals(listOf("cell"), input.card?.phones?.first()?.features)
    }

    @Test
    fun `home phone has no cell feature`() {
        val device = DeviceContact(
            contactId = 1,
            lookupKey = "lk1",
            displayName = "Dana",
            phones = listOf("5550100" to Phone.TYPE_HOME),
            emails = emptyList(),
            addresses = emptyList(),
            organization = null,
            birthday = null,
        )

        val input = DeviceContactMapper.toInput(device)

        assertNull(input.card?.phones?.first()?.features)
        assertEquals("home", input.card?.phones?.first()?.label)
    }

    @Test
    fun `birthday maps to a birth anniversary`() {
        val device = DeviceContact(
            contactId = 1,
            lookupKey = "lk1",
            displayName = "Dana",
            phones = emptyList(),
            emails = emptyList(),
            addresses = emptyList(),
            organization = null,
            birthday = "1990-06-15",
        )

        val input = DeviceContactMapper.toInput(device)

        val ann = input.card?.anniversaries?.first()
        assertEquals("birth", ann?.kind)
        assertEquals(1990, ann?.date?.partial?.year)
        assertEquals(6, ann?.date?.partial?.month)
        assertEquals(15, ann?.date?.partial?.day)
    }

    @Test
    fun `organization maps to an organization row`() {
        val device = DeviceContact(
            contactId = 1,
            lookupKey = "lk1",
            displayName = "Dana",
            phones = emptyList(),
            emails = emptyList(),
            addresses = emptyList(),
            organization = "Acme",
            birthday = null,
        )

        val input = DeviceContactMapper.toInput(device)

        assertEquals("Acme", input.card?.organizations?.first()?.name)
    }

    @Test
    fun `blank display name yields no name`() {
        val device = DeviceContact(1, "lk1", null, emptyList(), emptyList(), emptyList(), null, null)
        val input = DeviceContactMapper.toInput(device)
        assertTrue(input.card?.name == null)
    }
}
