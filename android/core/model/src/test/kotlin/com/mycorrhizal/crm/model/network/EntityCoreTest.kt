package com.mycorrhizal.crm.model.network

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Test

/** Issue #2302 / ADR 0043 decision 1: the per-kind `core` <-> form mapping. */
class EntityCoreTest {

    private fun ok(result: EntityCoreResult): Map<String, Any?> =
        (result as EntityCoreResult.Ok).core

    @Test
    fun `organisation form reads sort-as, one unit per line, url and notes`() {
        val core = mapOf(
            "organization" to mapOf(
                "name" to "Acme",
                "sortAs" to "ACME",
                "units" to listOf(mapOf("name" to "R&D"), mapOf("name" to "Sales"))
            ),
            "url" to "https://acme.example",
            "notes" to "n"
        )
        val form = EntityCore.toForm(EntityKind.ORGANISATION, core)
        assertEquals("ACME", form.sortAs)
        assertEquals("R&D\nSales", form.unitsText)
        assertEquals("https://acme.example", form.url)
        assertEquals("n", form.notes)
    }

    @Test
    fun `organisation core carries the name, drops blank keys and keeps unknown keys`() {
        val existing = mapOf(
            "organization" to mapOf("name" to "Old", "id" to "o1", "sortAs" to "x"),
            "extra" to 1L
        )
        val form = EntityCore.toForm(EntityKind.ORGANISATION, existing).copy(
            sortAs = "",
            unitsText = " A \n\nB",
            url = " https://a.example ",
            notes = ""
        )
        val core = ok(EntityCore.toCore(EntityKind.ORGANISATION, "Acme", form, existing))
        val org = core["organization"] as Map<*, *>
        assertEquals("Acme", org["name"])
        assertEquals("o1", org["id"])
        assertFalse(org.containsKey("sortAs"))
        assertEquals(listOf(mapOf("name" to "A"), mapOf("name" to "B")), org["units"])
        assertEquals("https://a.example", core["url"])
        assertFalse(core.containsKey("notes"))
        assertEquals(1L, core["extra"])
    }

    @Test
    fun `place form maps postal components and keeps components it does not edit`() {
        val existing = mapOf(
            "address" to mapOf(
                "components" to listOf(
                    mapOf("kind" to "name", "value" to "1 High St"),
                    mapOf("kind" to "locality", "value" to "Springfield"),
                    mapOf("kind" to "district", "value" to "North")
                ),
                "coordinates" to "geo:1,2"
            )
        )
        val form = EntityCore.toForm(EntityKind.PLACE, existing)
        assertEquals("1 High St", form.street)
        assertEquals("Springfield", form.city)

        val core = ok(
            EntityCore.toCore(
                EntityKind.PLACE,
                "Cabin",
                form.copy(city = "Shelbyville", postal = "12345", country = ""),
                existing
            )
        )
        val address = core["address"] as Map<*, *>
        assertEquals("geo:1,2", address["coordinates"])
        val kinds = (address["components"] as List<*>).map { (it as Map<*, *>)["kind"] }
        assertEquals(listOf("name", "district", "locality", "postcode"), kinds)
    }

    @Test
    fun `pet form parses partial dates and reports the invalid one`() {
        val form = EntityCore.toForm(EntityKind.PET, emptyMap()).copy(
            species = "Dog",
            birth = "2019-04",
            death = ""
        )
        val core = ok(EntityCore.toCore(EntityKind.PET, "Rex", form, null))
        assertEquals("Dog", core["species"])
        assertEquals(mapOf("year" to 2019L, "month" to 4L), core["birth"])
        assertFalse(core.containsKey("death"))

        val bad = EntityCore.toCore(EntityKind.PET, "Rex", form.copy(death = "2020-13"), null)
        assertEquals(EntityCoreResult.InvalidDate("death"), bad)
        assertEquals(
            EntityCoreResult.InvalidDate("birth"),
            EntityCore.toCore(EntityKind.PET, "Rex", form.copy(birth = "19"), null)
        )
        assertEquals(
            EntityCoreResult.InvalidDate("birth"),
            EntityCore.toCore(EntityKind.PET, "Rex", form.copy(birth = "2021-02-30"), null)
        )
    }

    @Test
    fun `whole-number doubles from the wire are written back as integers`() {
        val existing = mapOf("birth" to mapOf("year" to 2019.0, "month" to 4.0))
        val form = EntityCore.toForm(EntityKind.PET, existing)
        assertEquals("2019-04", form.birth)
        val core = ok(
            EntityCore.toCore(EntityKind.PET, "Rex", form.copy(species = "Cat"), existing)
        )
        assertEquals(mapOf("year" to 2019L, "month" to 4L), core["birth"])
    }

    @Test
    fun `partial date round trip`() {
        assertEquals("", EntityCore.formatPartial(null))
        assertEquals("2019", EntityCore.formatPartial(PartialDate(year = 2019)))
        assertEquals("2019-04-09", EntityCore.formatPartial(PartialDate(2019, 4, 9)))
        assertEquals(PartialParse.Blank, EntityCore.parsePartial("  "))
        assertEquals(
            PartialParse.Value(PartialDate(2019, 4, 9)),
            EntityCore.parsePartial("2019-04-09")
        )
        assertEquals(PartialParse.Value(PartialDate(year = 2019)), EntityCore.parsePartial("2019"))
        assertEquals(PartialParse.Invalid, EntityCore.parsePartial("20x9"))
        assertEquals(PartialParse.Invalid, EntityCore.parsePartial("2019-4"))
        assertEquals(PartialParse.Invalid, EntityCore.parsePartial("2019-04-09-01"))
    }

    @Test
    fun `summary line is the url, the locality and country, or the species and breed`() {
        assertEquals(
            "https://a.example",
            EntityCore.summary(EntityKind.ORGANISATION, mapOf("url" to "https://a.example"))
        )
        val place = mapOf(
            "address" to mapOf(
                "components" to listOf(
                    mapOf("kind" to "locality", "value" to "Paris"),
                    mapOf("kind" to "country", "value" to "France")
                )
            )
        )
        assertEquals("Paris, France", EntityCore.summary(EntityKind.PLACE, place))
        assertEquals(
            "Dog, Beagle",
            EntityCore.summary(EntityKind.PET, mapOf("species" to "Dog", "breed" to "Beagle"))
        )
        assertEquals("", EntityCore.summary(EntityKind.PET, null))
    }

    @Test
    fun `merge fills only fields the survivor leaves empty and the loser has`() {
        val survivor = Entity(
            id = "s",
            kind = EntityKind.ORGANISATION,
            name = "Acme",
            core = mapOf("url" to "https://a.example", "organization" to mapOf("name" to "Acme"))
        )
        val loser = Entity(
            id = "l",
            kind = EntityKind.ORGANISATION,
            name = "Acme Inc",
            core = mapOf(
                "url" to "https://other.example",
                "notes" to "hello",
                "organization" to mapOf("name" to "Acme Inc", "sortAs" to "ACME")
            )
        )
        assertEquals(
            listOf("organization.sortAs", "notes"),
            EntityCore.fillableMergeFields(survivor, loser)
        )
        val pets = Entity(kind = EntityKind.PET, core = mapOf("species" to "Dog"))
        assertEquals(
            listOf("species"),
            EntityCore.fillableMergeFields(Entity(kind = EntityKind.PET), pets)
        )
    }
}
