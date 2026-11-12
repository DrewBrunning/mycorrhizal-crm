package com.mycorrhizal.crm.model.network

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** #2513: the create-from-contacts wire models are values with safe defaults for sparse payloads. */
class EntityFromContactsModelTest {

    @Test
    fun `a group and its contacts are values and default safely`() {
        val contact = EntityValueGroupContact(contactId = 7, firstname = "Ada", lastname = "L")
        assertEquals("Ada L", contact.displayName)
        assertEquals("", EntityValueGroupContact().displayName)
        assertEquals(contact, contact.copy())
        assertNotEquals(contact, contact.copy(hasPrimary = true))
        assertTrue(contact.toString().contains("Ada"))
        val group = EntityValueGroup(key = "k", name = "Acme", contacts = listOf(contact))
        assertEquals(group, group.copy())
        assertEquals(group.hashCode(), group.copy().hashCode())
        assertTrue(group.variantsOrEmpty.isEmpty())
        assertEquals(listOf(contact), group.contactsOrEmpty)
        assertEquals(EntityKind.ORGANISATION, EntityValueGroup().kind)
        assertTrue(EntityValueGroup().contactsOrEmpty.isEmpty())
    }

    @Test
    fun `a response exposes empty groups when the server sent null`() {
        assertTrue(EntitySuggestionsResponse(groups = null).groupsOrEmpty.isEmpty())
        val response = EntitySuggestionsResponse(groups = listOf(EntityValueGroup(key = "k")))
        assertEquals(1, response.groupsOrEmpty.size)
    }

    @Test
    fun `input, skip, created and response are plain values`() {
        val group = EntityFromContactsGroup(key = "k", role = "other", contactIds = listOf(1))
        assertNull(group.name)
        assertEquals(false, group.setPrimary)
        val input = EntityFromContactsInput(kind = "place", groups = listOf(group))
        assertEquals(input, input.copy())
        assertNotEquals(input, input.copy(kind = "organisation"))
        assertEquals(group, group.copy())
        val skip = EntityFromContactsSkip()
        assertNull(skip.contactId)
        assertEquals("", skip.reason)
        assertEquals(skip, skip.copy())
        val created = EntityFromContactsCreated(key = "k")
        assertTrue(created.linksOrEmpty.isEmpty())
        assertEquals(created, created.copy())
        val response = EntityFromContactsResponse(
            created = listOf(created),
            skipped = listOf(skip)
        )
        assertEquals(response, response.copy())
        assertEquals(1, response.createdOrEmpty.size)
        assertEquals(1, response.skippedOrEmpty.size)
        assertTrue(EntityFromContactsResponse().createdOrEmpty.isEmpty())
        assertTrue(EntityFromContactsResponse().skippedOrEmpty.isEmpty())
        assertTrue(response.toString().contains("created"))
    }
}
