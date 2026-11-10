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

    @Test
    fun `period years validate as four digits with the end not before the start`() {
        assertTrue(entityPeriodYearsValid("", ""))
        assertTrue(entityPeriodYearsValid("2019", ""))
        assertTrue(entityPeriodYearsValid("2019", "2019"))
        assertTrue(entityPeriodYearsValid("2019", "2024"))
        assertFalse(entityPeriodYearsValid("19", ""))
        assertFalse(entityPeriodYearsValid("abcd", ""))
        assertFalse(entityPeriodYearsValid("2024", "2019"))
    }

    @Test
    fun `period from years is null when blank and keeps a finer stored date for an untouched year`() {
        assertNull(entityPeriodFromYears("", "", null))
        assertEquals(
            TemporalRange(start = PartialDate(year = 2019)),
            entityPeriodFromYears("2019", "", null)
        )
        val stored = TemporalRange(
            start = PartialDate(year = 2019, month = 4, day = 9),
            end = PartialDate(year = 2022, month = 1)
        )
        // start untouched keeps its month/day; end edited becomes a plain year.
        assertEquals(
            TemporalRange(start = stored.start, end = PartialDate(year = 2023)),
            entityPeriodFromYears("2019", "2023", stored)
        )
        // both untouched: identical to what was stored.
        assertEquals(stored, entityPeriodFromYears("2019", "2022", stored))
    }

    @Test
    fun `a period is current unless it ended before today or starts after it`() {
        val today = Triple(2026, 11, 10)
        fun current(range: TemporalRange?) = isPeriodCurrent(range, today.first, today.second, today.third)
        assertTrue(current(null))
        assertTrue(current(TemporalRange()))
        assertTrue(current(TemporalRange(start = PartialDate(year = 2019))))
        assertFalse(current(TemporalRange(end = PartialDate(year = 2024))))
        assertFalse(current(TemporalRange(start = PartialDate(year = 2027))))
        // shared precision: a year-only end of the current year still includes today.
        assertTrue(current(TemporalRange(end = PartialDate(year = 2026))))
        assertTrue(current(TemporalRange(end = PartialDate(2026, 11, 10))))
        assertFalse(current(TemporalRange(end = PartialDate(2026, 11, 9))))
        assertFalse(current(TemporalRange(start = PartialDate(2026, 12))))
        assertTrue(current(TemporalRange(start = PartialDate(2026, 11))))
        // a yearless endpoint is not comparable and counts as current (ADR 0015 rule 3).
        assertTrue(current(TemporalRange(end = PartialDate(month = 1, day = 1))))
    }

    @Test
    fun `only a primary, current link of a projecting kind is adopted before delete`() {
        fun person(primary: Boolean, period: TemporalRange? = null) =
            EntityContactLink(linkId = "l", contactId = 1, isPrimary = primary, period = period)
        assertTrue(person(true).projectsToday(2026, 11, 10))
        assertFalse(person(false).projectsToday(2026, 11, 10))
        assertFalse(
            person(true, TemporalRange(end = PartialDate(year = 2020))).projectsToday(2026, 11, 10)
        )
    }
}
