package com.mycorrhizal.crm.model.network

import com.mycorrhizal.crm.model.MoshiProvider
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** Issue #2604: the merge preview must decode when the server sends null lists. */
class ContactMergePreviewDecodeTest {

    private val adapter = MoshiProvider.get().adapter(ContactMergePreviewResponse::class.java)

    @Test
    fun `preview with explicit null lists decodes and exposes no conflicts`() {
        val json = """
            {"keep_id":1,"merge_id":2,"resolution":{
              "emails":[],"phones":null,"addresses":null,"urls":null,"impps":null,
              "circles":null,"resolved_scalars":null,"conflicts":null,"field_value_conflicts":null},
             "association_counts":{}}
        """.trimIndent()

        val preview = adapter.fromJson(json)

        assertNotNull(preview)
        assertNull(preview!!.resolution.addresses)
        assertNull(preview.resolution.circles)
        assertTrue(preview.resolution.allConflicts.isEmpty())
    }

    @Test
    fun `allConflicts returns scalar conflicts then field value conflicts`() {
        val json = """
            {"keep_id":1,"merge_id":2,"resolution":{
              "addresses":null,
              "conflicts":[{"field":"lastname","label":"Last Name","keeper_value":"A","loser_value":"B"}],
              "field_value_conflicts":[{"field":"fd1","label":"Custom","keeper_value":"x","loser_value":"y"}]}}
        """.trimIndent()

        val all = adapter.fromJson(json)!!.resolution.allConflicts

        assertEquals(listOf("lastname", "fd1"), all.map { it.field })
    }
}
