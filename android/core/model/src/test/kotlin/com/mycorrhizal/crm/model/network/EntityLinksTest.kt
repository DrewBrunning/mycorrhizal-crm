package com.mycorrhizal.crm.model.network

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Test

/** Issue #2302 / ADR 0043 decision 3: projected `ent-` entries are read-only, never written back. */
class EntityLinksTest {

    @Test
    fun `projected element ids carry the reserved ent- prefix`() {
        assertTrue(isProjectedElementId("ent-1234"))
        assertFalse(isProjectedElementId("1234"))
        assertFalse(isProjectedElementId(null))
        assertFalse(isProjectedElementId(""))
        assertEquals("1234", entityLinkIdFromElementId("ent-1234"))
        assertNull(entityLinkIdFromElementId("org-1"))
    }

    @Test
    fun `stripProjectedEntries removes only ent- organizations and addresses`() {
        val input = ContactRecordInput(
            card = Card(
                organizations = listOf(
                    Organization(id = "o1", name = "Own"),
                    Organization(id = "ent-L1", name = "Projected")
                ),
                addresses = listOf(Address(id = "ent-L2"), Address(id = "a1")),
                titles = listOf(Title(id = "ent-keep", name = "t"))
            )
        )
        val stripped = input.stripProjectedEntries()
        assertEquals(listOf("o1"), stripped.card?.organizations?.map { it.id })
        assertEquals(listOf("a1"), stripped.card?.addresses?.map { it.id })
        assertEquals(1, stripped.card?.titles?.size)
    }

    @Test
    fun `stripProjectedEntries returns the same instance when nothing is projected`() {
        val input = ContactRecordInput(card = Card(organizations = listOf(Organization(id = "o1"))))
        assertSame(input, input.stripProjectedEntries())
        val none = ContactRecordInput()
        assertSame(none, none.stripProjectedEntries())
    }

    @Test
    fun `stripping every projected entry leaves a null list, not an empty one`() {
        val input = ContactRecordInput(
            card = Card(organizations = listOf(Organization(id = "ent-L1", name = "P")))
        )
        assertNull(input.stripProjectedEntries().card?.organizations)
    }

    @Test
    fun `stripProjectedEntries also drops periods keyed by a projected id`() {
        val input = ContactRecordInput(
            card = Card(organizations = listOf(Organization(id = "ent-L1"))),
            crm = CRMEnvelope(
                periods = listOf(
                    EntryPeriod("organization", "ent-L1", TemporalRange()),
                    EntryPeriod("organization", "o1", TemporalRange())
                )
            )
        )
        assertEquals(listOf("o1"), input.stripProjectedEntries().crm?.periods?.map { it.entryId })
    }

    @Test
    fun `own views hide projected entries from the editors`() {
        val card = Card(
            organizations = listOf(Organization(id = "ent-L1"), Organization(id = "o1")),
            addresses = listOf(Address(id = "ent-L2"), Address(id = "a1"))
        )
        assertEquals(listOf("o1"), card.ownOrganizations().map { it.id })
        assertEquals(listOf("a1"), card.ownAddresses().map { it.id })
        assertTrue(Card().ownOrganizations().isEmpty())
    }

    @Test
    fun `link roles are the closed vocabulary per kind`() {
        assertEquals(
            listOf("employee", "owner", "contractor", "volunteer", "member", "other"),
            EntityLinkRoles.forKind(EntityKind.ORGANISATION)
        )
        assertEquals(
            listOf("resident", "owner", "visitor", "other"),
            EntityLinkRoles.forKind(EntityKind.PLACE)
        )
        assertEquals(listOf("owner", "carer", "other"), EntityLinkRoles.forKind(EntityKind.PET))
        assertTrue(EntityLinkRoles.forKind("x").isEmpty())
    }

    @Test
    fun `only organisations and places project, so only they can be adopted`() {
        assertTrue(EntityKind.projects(EntityKind.ORGANISATION))
        assertTrue(EntityKind.projects(EntityKind.PLACE))
        assertFalse(EntityKind.projects(EntityKind.PET))
    }
}
