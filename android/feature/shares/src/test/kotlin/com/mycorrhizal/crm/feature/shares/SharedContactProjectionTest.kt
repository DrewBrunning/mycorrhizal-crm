package com.mycorrhizal.crm.feature.shares

import com.squareup.moshi.Moshi
import com.squareup.moshi.Types
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * ADR 0042 decisions 4-5: the recipient view renders only an allowlisted set of JSContact
 * properties, and only the sections the grant names. Anything else in the payload is ignored.
 */
class SharedContactProjectionTest {

    private val adapter = Moshi.Builder().build().adapter<Map<String, Any?>>(
        Types.newParameterizedType(Map::class.java, String::class.java, Any::class.java)
    )

    private fun card(json: String): Map<String, Any?> = adapter.fromJson(json)!!

    private val full = card(
        """{"@type":"Card","version":"1.0","uid":"secret-uid-123","kind":"individual",
        "name":{"components":[{"kind":"given","value":"Dana"},{"kind":"surname","value":"White"}],
        "isOrdered":true},
        "nicknames":{"n1":{"name":"Dee"}},
        "emails":{"e1":{"address":"dana@example.com","label":"home"}},
        "phones":{"p1":{"number":"+1 555 0100"}},
        "addresses":{"a1":{"full":"1 Main St, Springfield"}},
        "organizations":{"o1":{"name":"Acme","units":[{"name":"R&D"}]}},
        "anniversaries":{"b1":{"kind":"birth","date":{"@type":"PartialDate","year":1990,"month":3,"day":7}}},
        "onlineServices":{"s1":{"service":"Mastodon","user":"dana"}},
        "links":{"l1":{"uri":"https://example.com/dana","label":"Site"}},
        "notes":{"n1":{"note":"Met at the fair"}},
        "keywords":{"friend":true,"neighbour":true},
        "relatedTo":{"other-uid":{"relation":{"friend":true}}},
        "personalInfo":{"i1":{"kind":"hobby","value":"Chess"}},
        "speakToAs":{"pronouns":{"x1":{"pronouns":"she/her"}}},
        "preferredLanguages":{"fr":{"pref":1}},
        "vCardProps":[["x-nickname-extra",{},"text","Bee"]],
        "mycorrhizal:private":"LEAK","crm":{"how_we_met":"LEAK"}}"""
    )

    @Test
    fun `identity is always shown and the uid is never`() {
        val view = projectSharedContact(full, grantedSections = emptySet())
        assertEquals("Dana White", view.displayName)
        assertEquals("Dee", view.nickname)
        assertTrue(view.sections.isEmpty())
    }

    @Test
    fun `only granted sections render`() {
        val view = projectSharedContact(full, grantedSections = setOf("emails", "notes"))
        assertEquals(listOf("emails", "notes"), view.sections.map { it.token })
        assertEquals(listOf("dana@example.com (home)"), view.sections[0].lines)
        assertEquals(listOf("Met at the fair"), view.sections[1].lines)
    }

    @Test
    fun `every section projects its allowlisted fields in canonical order`() {
        val all = projectSharedContact(full, grantedSections = null)
        val byToken = all.sections.associate { it.token to it.lines }
        assertEquals(listOf("+1 555 0100"), byToken["phones"])
        assertEquals(listOf("1 Main St, Springfield"), byToken["addresses"])
        assertEquals(listOf("Acme (R&D)"), byToken["organizations"])
        assertEquals(listOf("birth: 1990-03-07"), byToken["anniversaries"])
        assertEquals(listOf("Mastodon: dana"), byToken["online_services"])
        assertEquals(listOf("Site: https://example.com/dana"), byToken["links"])
        assertEquals(listOf("friend", "neighbour"), byToken["keywords"])
        assertEquals(listOf("friend"), byToken["related_to"])
        assertEquals(listOf("hobby: Chess"), byToken["personal_info"])
        assertEquals(listOf("she/her"), byToken["speak_to_as"])
        assertEquals(listOf("fr"), byToken["languages"])
        assertEquals(listOf("x-nickname-extra: Bee"), byToken["custom_fields"])
        assertEquals(
            listOf(
                "emails", "phones", "addresses", "organizations", "anniversaries",
                "online_services", "links", "notes", "keywords", "related_to", "personal_info",
                "speak_to_as", "languages", "custom_fields"
            ),
            all.sections.map { it.token }
        )
    }

    @Test
    fun `unknown and internal properties are never rendered`() {
        val view = projectSharedContact(full, grantedSections = null)
        val everything = buildString {
            append(view.displayName).append(view.nickname)
            view.sections.forEach { append(it.lines.joinToString()) }
        }
        assertFalse(everything.contains("secret-uid-123"))
        assertFalse(everything.contains("LEAK"))
        assertFalse(everything.contains("other-uid"))
    }

    @Test
    fun `an unknown granted token is ignored rather than widened`() {
        val view = projectSharedContact(full, grantedSections = setOf("bogus"))
        assertTrue(view.sections.isEmpty())
    }

    @Test
    fun `a granted section with no data is omitted`() {
        val view = projectSharedContact(
            card("""{"name":{"full":"Solo"}}"""),
            grantedSections = setOf("emails")
        )
        assertEquals("Solo", view.displayName)
        assertTrue(view.sections.isEmpty())
        assertNull(view.nickname)
    }

    @Test
    fun `a missing name falls back to an empty display name for the caller to localize`() {
        assertEquals("", projectSharedContact(card("{}"), null).displayName)
    }

    @Test
    fun `a list shaped section is read like a map shaped one and malformed entries are skipped`() {
        val view = projectSharedContact(
            card(
                """{"name":{"full":"X"},"emails":[{"address":"a@b.c"},"junk",{"address":""}],
                "phones":"not-a-collection"}"""
            ),
            grantedSections = null
        )
        assertEquals(listOf("emails"), view.sections.map { it.token })
        assertEquals(listOf("a@b.c"), view.sections[0].lines)
    }

    @Test
    fun `address components and string dates render`() {
        val view = projectSharedContact(
            card(
                """{"name":{"full":"X"},
                "addresses":{"a":{"components":[{"kind":"name","value":"5 Oak"},{"kind":"locality","value":"Town"}]}},
                "anniversaries":{"b":{"kind":"wedding","date":"2010-06-01"}}}"""
            ),
            grantedSections = null
        )
        val byToken = view.sections.associate { it.token to it.lines }
        assertEquals(listOf("5 Oak, Town"), byToken["addresses"])
        assertEquals(listOf("wedding: 2010-06-01"), byToken["anniversaries"])
    }
}
