package com.mycorrhizal.crm.feature.imports

import android.provider.ContactsContract.CommonDataKinds.Phone
import android.provider.ContactsContract.CommonDataKinds.StructuredPostal
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
    fun `structured address maps to the real registry kinds, not street or city`() {
        // Regression for T67 Bug B: the registry has no "street"/"city" kind — it's
        // "name"/"locality" (contactmodel/model.go:167-176).
        val device = DeviceContact(
            contactId = 1,
            lookupKey = "lk1",
            displayName = "Dana",
            phones = emptyList(),
            emails = emptyList(),
            addresses = listOf(
                DeviceAddress(
                    street = "742 Evergreen Terrace",
                    city = "Springfield",
                    region = "IL",
                    postcode = "55555",
                    country = "USA",
                    formattedAddress = "742 Evergreen Terrace, Springfield, IL 55555, USA",
                    customLabel = null,
                    type = StructuredPostal.TYPE_HOME,
                ),
            ),
            organization = null,
            birthday = null,
        )

        val input = DeviceContactMapper.toInput(device)

        val components = input.card?.addresses?.first()?.components.orEmpty()
        assertEquals("name", components[0].kind)
        assertEquals("742 Evergreen Terrace", components[0].value)
        assertEquals("locality", components[1].kind)
        assertEquals("Springfield", components[1].value)
        assertEquals("region", components[2].kind)
        assertEquals("postcode", components[3].kind)
        assertEquals("country", components[4].kind)
        assertTrue(components.none { it.kind == "street" || it.kind == "city" })
    }

    @Test
    fun `postcode never lands as the only address content`() {
        // Regression for T67 Bug A: a device row where postcode is the sole populated
        // field must produce a "postcode" component, never masquerade as the street.
        val device = DeviceContact(
            contactId = 1,
            lookupKey = "lk1",
            displayName = "Dana",
            phones = emptyList(),
            emails = emptyList(),
            addresses = listOf(
                DeviceAddress(
                    street = null,
                    city = null,
                    region = null,
                    postcode = "55555",
                    country = null,
                    formattedAddress = null,
                    customLabel = null,
                    type = StructuredPostal.TYPE_HOME,
                ),
            ),
            organization = null,
            birthday = null,
        )

        val input = DeviceContactMapper.toInput(device)

        val components = input.card?.addresses?.first()?.components.orEmpty()
        assertEquals(listOf("postcode"), components.map { it.kind })
        assertEquals("55555", components.first().value)
    }

    @Test
    fun `home address carries the private context, work address the work context`() {
        val home = DeviceAddress(
            street = "1 Main St", city = null, region = null, postcode = null, country = null,
            formattedAddress = null, customLabel = null, type = StructuredPostal.TYPE_HOME,
        )
        val work = DeviceAddress(
            street = "2 Main St", city = null, region = null, postcode = null, country = null,
            formattedAddress = null, customLabel = null, type = StructuredPostal.TYPE_WORK,
        )
        val other = DeviceAddress(
            street = "3 Main St", city = null, region = null, postcode = null, country = null,
            formattedAddress = null, customLabel = null, type = StructuredPostal.TYPE_OTHER,
        )
        val device = DeviceContact(
            contactId = 1,
            lookupKey = "lk1",
            displayName = "Dana",
            phones = emptyList(),
            emails = emptyList(),
            addresses = listOf(home, work, other),
            organization = null,
            birthday = null,
        )

        val addresses = DeviceContactMapper.toInput(device).card?.addresses.orEmpty()

        assertEquals(listOf("private"), addresses[0].contexts)
        assertEquals(listOf("work"), addresses[1].contexts)
        assertNull(addresses[2].contexts)
    }

    @Test
    fun `blank display name yields no name`() {
        val device = DeviceContact(1, "lk1", null, emptyList(), emptyList(), emptyList(), null, null)
        val input = DeviceContactMapper.toInput(device)
        assertTrue(input.card?.name == null)
    }
}
