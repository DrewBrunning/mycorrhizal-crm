package com.mycorrhizal.crm.feature.contacts

import com.mycorrhizal.crm.model.network.Address
import com.mycorrhizal.crm.model.network.Anniversary
import com.mycorrhizal.crm.model.network.AnniversaryDate
import com.mycorrhizal.crm.model.network.CRMEnvelope
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.Email
import com.mycorrhizal.crm.model.network.EntryPeriod
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.model.network.OrgUnit
import com.mycorrhizal.crm.model.network.Organization
import com.mycorrhizal.crm.model.network.PartialDate
import com.mycorrhizal.crm.model.network.PersonalInfo
import com.mycorrhizal.crm.model.network.PersonalInfoMeta
import com.mycorrhizal.crm.model.network.TemporalRange
import com.mycorrhizal.crm.model.network.Title
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Issue #1728: pure record-mapping tests for the contact-page drill-downs.
 * They pin the full-record PUT payload — including the entry `level`, the
 * per-entry `crm.personalInfoMeta` (sensitivity/favorite/notes), and the fact
 * that everything the drill-down does not edit round-trips unchanged.
 */
class ContactSectionMappingTest {

    private fun record(
        card: Card = Card(name = Name(full = "Dana White")),
        crm: CRMEnvelope = CRMEnvelope(),
    ) = ContactRecordResponse(id = 5, uid = "uid-5", revision = 7, gender = "female", card = card, crm = crm)

    // --- Skills & interests -------------------------------------------------

    @Test
    fun `adding a skill keeps its level and writes sensitivity favorite and notes metadata`() {
        val base = record(
            card = Card(
                name = Name(full = "Dana White"),
                emails = listOf(Email(id = "e1", address = "dana@example.com")),
            ),
            crm = CRMEnvelope(ambient = true, workInformation = "Analyst"),
        )

        val input = base.withPersonalInfo(
            editingId = null,
            form = PersonalInfoFormData(
                kind = "expertise",
                value = "  Go  ",
                level = "expert",
                label = "Backend",
                notes = "  shipped two services  ",
                sensitivity = "private",
                favorite = true,
            ),
            newId = "pi-new",
        )

        val entry = input.card!!.personalInfo!!.single()
        assertEquals("pi-new", entry.id)
        assertEquals("expertise", entry.kind)
        assertEquals("Go", entry.value)
        assertEquals("expert", entry.level)
        assertEquals("Backend", entry.label)
        assertEquals(
            PersonalInfoMeta(
                sensitivity = "private",
                favorite = true,
                notes = "shipped two services",
            ),
            input.crm!!.personalInfoMeta!!["pi-new"],
        )
        // Everything the dialog does not model rides along unchanged.
        assertEquals("female", input.gender)
        assertEquals(listOf(Email(id = "e1", address = "dana@example.com")), input.card!!.emails)
        assertEquals(true, input.crm!!.ambient)
        assertEquals("Analyst", input.crm!!.workInformation)
        assertEquals(7, base.revision!!)
    }

    @Test
    fun `editing a skill preserves its listAs and every other entry metadata`() {
        val base = record(
            card = Card(
                personalInfo = listOf(
                    PersonalInfo(id = "pi-1", kind = "hobby", value = "Climbing", level = "high", listAs = 2),
                    PersonalInfo(id = "pi-2", kind = "interest", value = "Chess"),
                ),
            ),
            crm = CRMEnvelope(
                personalInfoMeta = mapOf(
                    "pi-1" to PersonalInfoMeta(favorite = true, source = "import"),
                    "pi-2" to PersonalInfoMeta(notes = "plays weekly"),
                ),
            ),
        )

        val input = base.withPersonalInfo(
            editingId = "pi-1",
            form = PersonalInfoFormData(kind = "expertise", value = "Rust", level = "average"),
        )

        val edited = input.card!!.personalInfo!!.first { it.id == "pi-1" }
        assertEquals("Rust", edited.value)
        assertEquals("expertise", edited.kind)
        assertEquals("average", edited.level)
        assertEquals("listAs is not surfaced by the dialog and must survive", 2, edited.listAs!!)
        // The touched entry keeps its import provenance but loses the old
        // favorite/notes because the form explicitly submitted normal/no-notes.
        assertEquals(PersonalInfoMeta(source = "import"), input.crm!!.personalInfoMeta!!["pi-1"])
        assertEquals(PersonalInfoMeta(notes = "plays weekly"), input.crm!!.personalInfoMeta!!["pi-2"])
        assertEquals(2, input.card!!.personalInfo!!.size)
    }

    @Test
    fun `normal sensitivity with no notes or favorite leaves no metadata entry`() {
        val base = record(crm = CRMEnvelope(personalInfoMeta = mapOf("pi-1" to PersonalInfoMeta(notes = "old"))))

        val input = base.withPersonalInfo(
            editingId = null,
            form = PersonalInfoFormData(kind = "interest", value = "Chess", sensitivity = "normal"),
            newId = "pi-2",
        )

        assertFalse(input.crm!!.personalInfoMeta.orEmpty().containsKey("pi-2"))
        assertEquals(PersonalInfoMeta(notes = "old"), input.crm!!.personalInfoMeta!!["pi-1"])
    }

    @Test
    fun `deleting a skill removes the entry and only its metadata`() {
        val base = record(
            card = Card(
                personalInfo = listOf(
                    PersonalInfo(id = "pi-1", kind = "hobby", value = "Climbing"),
                    PersonalInfo(id = "pi-2", kind = "interest", value = "Chess"),
                ),
            ),
            crm = CRMEnvelope(
                personalInfoMeta = mapOf(
                    "pi-1" to PersonalInfoMeta(sensitivity = "secret"),
                    "pi-2" to PersonalInfoMeta(favorite = true),
                ),
            ),
        )

        val input = base.withoutPersonalInfo("pi-1")

        assertEquals(listOf("pi-2"), input.card!!.personalInfo!!.map { it.id })
        assertEquals(setOf("pi-2"), input.crm!!.personalInfoMeta!!.keys)
        assertEquals(PersonalInfoMeta(favorite = true), input.crm!!.personalInfoMeta!!["pi-2"])
    }

    // --- Work ---------------------------------------------------------------

    @Test
    fun `adding an organization writes its department and whole-year period`() {
        val base = record(
            card = Card(organizations = listOf(Organization(id = "org-1", name = "Acme", sortAs = "acme"))),
            crm = CRMEnvelope(periods = listOf(EntryPeriod(kind = "organization", entryId = "org-1"))),
        )

        val input = base.withOrganization(
            editingId = null,
            form = OrganizationFormData(name = "Globex", department = "R&D", startYear = "2019", endYear = "2024"),
            newId = "org-2",
        )

        assertEquals(
            listOf(
                Organization(id = "org-1", name = "Acme", sortAs = "acme"),
                Organization(id = "org-2", name = "Globex", units = listOf(OrgUnit(name = "R&D"))),
            ),
            input.card!!.organizations,
        )
        assertEquals(
            TemporalRange(start = PartialDate(year = 2019), end = PartialDate(year = 2024)),
            input.crm!!.periods!!.first { it.entryId == "org-2" }.range,
        )
        // The untouched organization's period survives.
        assertTrue(input.crm!!.periods!!.any { it.entryId == "org-1" })
    }

    @Test
    fun `editing an organization preserves extra fields the form does not show`() {
        val original = Organization(id = "org-1", name = "Acme", sortAs = "acme", units = listOf(OrgUnit(name = "Ops")))
        val base = record(card = Card(organizations = listOf(original)))

        val input = base.withOrganization(
            editingId = "org-1",
            form = OrganizationFormData(name = "Acme Corp"),
        )

        val edited = input.card!!.organizations!!.single()
        assertEquals("Acme Corp", edited.name)
        assertEquals("acme", edited.sortAs)
        assertNull(edited.units)
        assertNull(input.crm!!.periods)
    }

    @Test
    fun `deleting an organization removes its period but leaves other periods`() {
        val base = record(
            card = Card(
                organizations = listOf(
                    Organization(id = "org-1", name = "Acme"),
                    Organization(id = "org-2", name = "Globex"),
                ),
            ),
            crm = CRMEnvelope(
                periods = listOf(
                    EntryPeriod(kind = "organization", entryId = "org-1", range = TemporalRange(start = PartialDate(year = 2000))),
                    EntryPeriod(kind = "title", entryId = "title-1", range = TemporalRange(start = PartialDate(year = 2001))),
                ),
            ),
        )

        val input = base.withoutOrganization("org-1")

        assertEquals(listOf("org-2"), input.card!!.organizations!!.map { it.id })
        assertEquals(listOf("title-1"), input.crm!!.periods!!.map { it.entryId })
    }

    @Test
    fun `adding a title writes its kind and period`() {
        val base = record()

        val input = base.withTitle(
            editingId = null,
            form = TitleFormData(name = "Engineer", kind = "role", startYear = "2020"),
            newId = "title-1",
        )

        assertEquals(Title(id = "title-1", name = "Engineer", kind = "role"), input.card!!.titles!!.single())
        assertEquals(
            TemporalRange(start = PartialDate(year = 2020)),
            input.crm!!.periods!!.single().range,
        )
        assertEquals("title", input.crm!!.periods!!.single().kind)
    }

    @Test
    fun `deleting a title removes only its own period`() {
        val base = record(
            card = Card(titles = listOf(Title(id = "title-1", name = "Engineer"), Title(id = "title-2", name = "Advisor"))),
            crm = CRMEnvelope(
                periods = listOf(
                    EntryPeriod(kind = "title", entryId = "title-1"),
                    EntryPeriod(kind = "title", entryId = "title-2"),
                ),
            ),
        )

        val input = base.withoutTitle("title-1")

        assertEquals(listOf("title-2"), input.card!!.titles!!.map { it.id })
        assertEquals(listOf("title-2"), input.crm!!.periods!!.map { it.entryId })
    }

    @Test
    fun `work information is replaceable and an explicitly blank value clears it`() {
        val base = record(crm = CRMEnvelope(workInformation = "Old", contactInformation = "Prefers email"))

        assertEquals("New role", base.withWorkInformation("  New role  ").crm!!.workInformation)
        val cleared = base.withWorkInformation("   ")
        assertNull(cleared.crm!!.workInformation)
        assertEquals("Prefers email", cleared.crm!!.contactInformation)
    }

    // --- Anniversaries ------------------------------------------------------

    @Test
    fun `adding an anniversary keeps the birth entry and preserves other fields`() {
        val place = Address(full = "City Hall")
        val base = record(
            card = Card(
                anniversaries = listOf(
                    Anniversary(id = "birth-1", kind = "birth", date = AnniversaryDate(partial = PartialDate(year = 1990, month = 6, day = 15))),
                    Anniversary(id = "a-1", kind = "wedding", date = AnniversaryDate(partial = PartialDate(year = 2015)), place = place),
                ),
            ),
        )

        val input = base.withAnniversary(
            editingId = null,
            form = AnniversaryFormData(kind = "death", date = "2021-03-04"),
            newId = "a-2",
        )

        val anniversaries = input.card!!.anniversaries!!
        assertEquals(listOf("birth-1", "a-1", "a-2"), anniversaries.map { it.id })
        assertEquals(AnniversaryDate(partial = PartialDate(year = 1990, month = 6, day = 15)), anniversaries.first().date)
        assertEquals(place, anniversaries.first { it.id == "a-1" }.place)
        assertEquals("death", anniversaries.last().kind)
        assertEquals(PartialDate(year = 2021, month = 3, day = 4), anniversaries.last().date!!.partial)
    }

    @Test
    fun `editing a yearless anniversary preserves its place and id`() {
        val place = Address(full = "Garden")
        val base = record(
            card = Card(
                anniversaries = listOf(
                    Anniversary(id = "a-1", kind = "wedding", date = AnniversaryDate(partial = PartialDate(month = 5, day = 1)), place = place),
                ),
            ),
        )

        val input = base.withAnniversary(
            editingId = "a-1",
            form = AnniversaryFormData(kind = "wedding", date = "--06-02"),
        )

        val edited = input.card!!.anniversaries!!.single()
        assertEquals("a-1", edited.id)
        assertEquals(place, edited.place)
        assertEquals(PartialDate(month = 6, day = 2), edited.date!!.partial)
    }

    @Test
    fun `deleting an anniversary leaves the others and never touches birth`() {
        val base = record(
            card = Card(
                anniversaries = listOf(
                    Anniversary(id = "birth-1", kind = "birth"),
                    Anniversary(id = "a-1", kind = "wedding"),
                ),
            ),
        )

        val input = base.withoutAnniversary("a-1")

        assertEquals(listOf("birth-1"), input.card!!.anniversaries!!.map { it.id })
    }

    @Test
    fun `non-birth anniversaries filters the header birthday out`() {
        val base = record(
            card = Card(
                anniversaries = listOf(
                    Anniversary(id = "birth-1", kind = "birth"),
                    Anniversary(id = "a-1", kind = "wedding"),
                    Anniversary(id = "a-2", kind = "death"),
                ),
            ),
        )

        assertEquals(listOf("a-1", "a-2"), base.nonBirthAnniversaries().map { it.id })
        assertFalse(base.nonBirthAnniversaries().any { it.kind == "birth" })
    }

    @Test
    fun `level options are kind-specific`() {
        assertEquals(listOf("beginner", "average", "expert"), personalInfoLevelOptions("expertise"))
        assertEquals(listOf("high", "medium", "low"), personalInfoLevelOptions("hobby"))
        assertEquals(listOf("high", "medium", "low"), personalInfoLevelOptions("interest"))
    }
}
