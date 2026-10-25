package com.mycorrhizal.crm.model.network

import java.util.Locale
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class FieldGroupTest {

    private fun def(id: String, group: String = "") = FieldDefinition(
        id = id,
        label = id,
        type = "string",
        group = group
    )

    @Test
    fun `buckets by exact trimmed label and keeps definition order inside a group`() {
        val groups = groupFieldDefinitions(
            listOf(def("a", "Travel"), def("b", ""), def("c", "Travel"), def("d", "  Food  ")),
            mapOf("a" to "x", "b" to "y", "c" to "z", "d" to "w"),
            Locale.ENGLISH
        )
        assertEquals(listOf("Food", "Travel", ""), groups.map { it.label })
        assertEquals(
            listOf("a", "c"),
            groups.first { it.label == "Travel" }.definitions.map { it.id }
        )
    }

    @Test
    fun `labels differing only by case are separate buckets`() {
        val values = mapOf("a" to "x", "b" to "y")
        val groups = groupFieldDefinitions(
            listOf(def("a", "Travel"), def("b", "travel")),
            values,
            Locale.ENGLISH
        )
        assertEquals(2, groups.size)
        assertEquals(setOf("Travel", "travel"), groups.map { it.label }.toSet())
        // Deterministic regardless of input order.
        val reversed = groupFieldDefinitions(
            listOf(def("b", "travel"), def("a", "Travel")),
            values,
            Locale.ENGLISH
        )
        assertEquals(groups.map { it.label }, reversed.map { it.label })
    }

    @Test
    fun `groups sort alphabetically and locale-aware with ungrouped last`() {
        val values = mapOf("1" to "v", "2" to "v", "3" to "v", "4" to "v")
        val groups = groupFieldDefinitions(
            listOf(def("1", "zeta"), def("2", ""), def("3", "Álpha"), def("4", "beta")),
            values,
            Locale.ENGLISH
        )
        assertEquals(listOf("Álpha", "beta", "zeta", ""), groups.map { it.label })
    }

    @Test
    fun `groups with no filled value come after filled ones, ungrouped last within a tier`() {
        val groups = groupFieldDefinitions(
            listOf(def("a", "Alpha"), def("b", "Beta"), def("c", ""), def("d", "Delta")),
            mapOf("b" to "x", "c" to "y"),
            Locale.ENGLISH
        )
        // Filled tier: Beta, then ungrouped. Muted tier: Alpha, Delta.
        assertEquals(listOf("Beta", "", "Alpha", "Delta"), groups.map { it.label })
        assertEquals(listOf(true, true, false, false), groups.map { it.hasFilled })
    }

    @Test
    fun `filled count counts only definitions with a value`() {
        val groups = groupFieldDefinitions(
            listOf(def("a", "G"), def("b", "G"), def("c", "G"), def("d", "G")),
            mapOf("a" to "x", "b" to "  ", "c" to null, "d" to false),
            Locale.ENGLISH
        )
        assertEquals(1, groups.size)
        assertEquals(2, groups[0].filledCount) // "x" and the explicit false
        assertEquals(4, groups[0].definitions.size)
    }

    @Test
    fun `no definitions means no groups`() {
        assertEquals(
            emptyList<FieldGroup>(),
            groupFieldDefinitions(emptyList(), emptyMap(), Locale.ENGLISH)
        )
    }

    @Test
    fun `ungrouped flag follows a blank label`() {
        val groups = groupFieldDefinitions(listOf(def("a", "   ")), emptyMap(), Locale.ENGLISH)
        assertTrue(groups.single().isUngrouped)
        assertFalse(
            groupFieldDefinitions(
                listOf(def("a", "X")),
                emptyMap(),
                Locale.ENGLISH
            ).single().isUngrouped
        )
    }

    @Test
    fun `hasFieldValue treats blank strings, empty lists and null as unfilled`() {
        assertFalse(hasFieldValue(null))
        assertFalse(hasFieldValue(""))
        assertFalse(hasFieldValue(listOf("", " ")))
        assertTrue(hasFieldValue(listOf("", "a")))
        assertTrue(hasFieldValue(0.0))
        assertTrue(hasFieldValue(false))
    }

    @Test
    fun `suggestions are distinct, non-blank, case-sensitive and sorted`() {
        val s = fieldGroupSuggestions(
            listOf(
                def("a", "Travel"),
                def("b", ""),
                def("c", "Food"),
                def("d", "Travel"),
                def("e", "travel")
            ),
            Locale.ENGLISH
        )
        assertEquals(setOf("Food", "Travel", "travel"), s.toSet())
        assertEquals(3, s.size)
        assertEquals("Food", s.first())
    }
}
