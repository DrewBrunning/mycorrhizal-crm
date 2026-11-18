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

    // Issues #2653 / #2733: an invalid birthday is a warn diagnostic, never a validation error.
    @Test
    fun `a birthday warning row decodes with no validation errors`() {
        val json = """
            {"row_index":0,"parsed_contact":{"firstname":"Ada"},"validation_errors":[],
             "suggested_action":"add",
             "diagnostics":[{"severity":"warn","concept":"birthday","message":"birthday not imported: invalid value"}]}
        """.trimIndent()

        val row = adapter.fromJson(json)!!

        assertEquals(emptyList<String>(), row.validationErrors)
        assertEquals(
            ImportDiagnostic("warn", "birthday", "birthday not imported: invalid value"),
            row.diagnostics!!.single()
        )
    }

    // Issue #2731: file-level notices ride on the upload and preview responses.
    @Test
    fun `a preview carries file diagnostics and an old server's omission decodes as empty`() {
        val adapterP = MoshiProvider.get().adapter(ImportPreviewResponse::class.java)
        val with = adapterP.fromJson(
            """{"session_id":"s","rows":[],"file_diagnostics":[{"severity":"warn","concept":"encoding","message":"file was not UTF-8; read as Windows-1252"}]}"""
        )!!
        val without = adapterP.fromJson("""{"session_id":"s","rows":[]}""")!!
        val none = adapterP.fromJson("""{"session_id":"s","rows":[],"file_diagnostics":[]}""")!!

        assertEquals(
            listOf(
                ImportDiagnostic(
                    "warn",
                    "encoding",
                    "file was not UTF-8; read as Windows-1252"
                )
            ),
            with.fileDiagnostics
        )
        assertEquals(emptyList<ImportDiagnostic>(), without.fileDiagnostics)
        assertEquals(emptyList<ImportDiagnostic>(), none.fileDiagnostics)
    }

    @Test
    fun `a CSV upload response carries file diagnostics`() {
        val upload = MoshiProvider.get().adapter(ImportUploadResponse::class.java).fromJson(
            """{"session_id":"s","headers":["a"],"suggested_mappings":[],"row_count":1,"sample_data":[["x"]],"file_diagnostics":[{"severity":"warn","concept":"encoding","message":"m"}]}"""
        )!!

        assertEquals("encoding", upload.fileDiagnostics.single().concept)
    }

    // Issue #2746: warning issues, and a status this client does not know must not throw.
    @Test
    fun `a result with warning and unknown status issues decodes`() {
        val result = MoshiProvider.get().adapter(ImportResult::class.java).fromJson(
            """{"total_processed":3,"created":2,"updated":1,"skipped":0,"errors":[],"issues":[
              {"row":1,"name":"Ada","email":"a@x.io","phone":"","status":"warning","concept":"birthday","reason":"birthday not imported: invalid value"},
              {"row":2,"name":"Bo","email":"","phone":"","status":"skipped","reason":"dup"},
              {"row":3,"name":"Cy","email":"","phone":"","status":"quarantined","concept":"future","reason":"r"}]}"""
        )!!

        assertEquals(listOf("warning", "skipped", "quarantined"), result.issues.map { it.status })
        assertEquals("birthday", result.issues[0].concept)
        assertEquals("", result.issues[1].concept)
        assertEquals("future", result.issues[2].concept)
    }

    @Test
    fun `a history run detail keeps the warning concept in its report`() {
        val detail = MoshiProvider.get().adapter(ImportRunDetail::class.java).fromJson(
            """{"id":4,"format":"vcf","total_processed":1,"created":1,"updated":0,"skipped":0,"error_count":0,"issue_count":1,
              "created_at":"2026-11-10T10:00:00Z",
              "issues":[{"row":1,"name":"Ada","email":"","phone":"","status":"warning","concept":"anniversary","reason":"anniversary not imported: invalid value"}]}"""
        )!!

        assertEquals("anniversary", detail.toImportResult().issues.single().concept)
    }

    // Issue #2735 / #2727: a failed Meerkat fetch reports a reason code beside the English text.
    @Test
    fun `a failed source status carries the coded reason and its params`() {
        val adapterS = MoshiProvider.get().adapter(SourceImportStatus::class.java)
        val coded = adapterS.fromJson(
            """{"session_id":"s","phase":"failed","error":"Could not reach Meerkat","error_reason_code":"meerkat_api_error","error_reason_params":{"reason":"Meerkat rejected the credentials"}}"""
        )!!
        val plain = adapterS.fromJson("""{"session_id":"s","phase":"failed","error":"boom"}""")!!

        assertEquals(
            ErrorReason("meerkat_api_error", mapOf("reason" to "Meerkat rejected the credentials")),
            coded.errorReason
        )
        assertNull(plain.errorReason)
    }

    @Test
    fun `a coded failure without params has empty params`() {
        val status = MoshiProvider.get().adapter(SourceImportStatus::class.java).fromJson(
            """{"phase":"failed","error":"x","error_reason_code":"meerkat_fetch_unreachable"}"""
        )!!

        assertEquals(ErrorReason("meerkat_fetch_unreachable", emptyMap()), status.errorReason)
    }
}
