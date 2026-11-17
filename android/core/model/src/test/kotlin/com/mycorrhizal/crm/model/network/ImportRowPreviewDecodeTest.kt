package com.mycorrhizal.crm.model.network

import com.mycorrhizal.crm.model.MoshiProvider
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * Issue #2641: the import preview carries `diagnostics` as objects
 * (`contactmodel.Diagnostic`: severity, concept, message), not strings. A
 * nameless vCard used to reject the whole file with "Unexpected server response".
 */
class ImportRowPreviewDecodeTest {

    private val adapter = MoshiProvider.get().adapter(ImportRowPreview::class.java)

    @Test
    fun `a row with the real server diagnostics objects decodes`() {
        val json = """
            {"row_index":1,"parsed_contact":{"email":"a@example.com"},"validation_errors":[],
             "duplicate_match":null,"suggested_action":"add","merge_diff":null,
             "batch_duplicate_of":null,
             "diagnostics":[{"severity":"info","concept":"name","message":"The name was empty; the first email or phone number is used as the name"}]}
        """.trimIndent()

        val row = adapter.fromJson(json)

        assertNotNull(row)
        assertEquals(
            listOf(
                ImportDiagnostic(
                    severity = "info",
                    concept = "name",
                    message = "The name was empty; the first email or phone number is used as the name"
                )
            ),
            row!!.diagnostics
        )
    }

    @Test
    fun `a diagnostic without concept decodes (the server omits an empty concept)`() {
        val json = """{"row_index":0,"diagnostics":[{"severity":"warn","message":"dropped"}]}"""

        val diagnostic = adapter.fromJson(json)!!.diagnostics!!.single()

        assertEquals("warn", diagnostic.severity)
        assertEquals("", diagnostic.concept)
        assertEquals("dropped", diagnostic.message)
    }

    @Test
    fun `rows without diagnostics decode as null or empty`() {
        val absent = adapter.fromJson("""{"row_index":0,"suggested_action":"add"}""")!!
        val nulled = adapter.fromJson("""{"row_index":0,"diagnostics":null}""")!!
        val empty = adapter.fromJson("""{"row_index":0,"diagnostics":[]}""")!!

        assertNull(absent.diagnostics)
        assertNull(nulled.diagnostics)
        assertEquals(emptyList<ImportDiagnostic>(), empty.diagnostics)
    }

    @Test
    fun `a whole preview with a nameless middle card decodes all three rows`() {
        val json = """
            {"rows":[
              {"row_index":0,"parsed_contact":{"firstname":"Ann"},"suggested_action":"add"},
              {"row_index":1,"parsed_contact":{"email":"x@example.com"},"suggested_action":"add",
               "diagnostics":[{"severity":"info","concept":"name","message":"name from email"}]},
              {"row_index":2,"parsed_contact":{"firstname":"Cy"},"suggested_action":"add"}]}
        """.trimIndent()

        val preview = MoshiProvider.get().adapter(ImportPreviewResponse::class.java).fromJson(json)

        assertEquals(3, preview!!.rows.size)
        assertEquals("name from email", preview.rows[1].diagnostics!!.single().message)
    }
}
