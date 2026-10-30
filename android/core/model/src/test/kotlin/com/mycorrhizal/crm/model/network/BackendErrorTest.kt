package com.mycorrhizal.crm.model.network

import com.mycorrhizal.crm.model.MoshiProvider
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** Issue #2105: the error envelope carries per-field reasons on a VALIDATION_ERROR. */
class BackendErrorTest {

    private val adapter = MoshiProvider.get().adapter(BackendError::class.java)

    @Test
    fun `field_reasons parse with their code and params`() {
        val error = adapter.fromJson(
            """
            {
              "error": {
                "code": "VALIDATION_ERROR",
                "message": "Request validation failed",
                "details": {"Name": "Name is required"},
                "field_reasons": {
                  "Name": {"reason_code": "validation_required", "reason_params": {"field": "Name"}},
                  "Email": {"reason_code": "validation_email"}
                }
              }
            }
            """.trimIndent()
        )!!.error!!

        assertEquals("Request validation failed", error.displayMessage)
        val fieldReasons = error.fieldReasons!!
        val name = fieldReasons.getValue("Name")
        assertEquals("validation_required", name.reasonCode)
        assertEquals(mapOf("field" to "Name"), name.reasonParams)
        val email = fieldReasons.getValue("Email")
        assertEquals("validation_email", email.reasonCode)
        assertNull(email.reasonParams)
    }

    @Test
    fun `field_reasons are null for an older server or a non-validation error`() {
        val error = adapter.fromJson(
            """{"error":{"code":"NOT_FOUND","message":"Contact not found"}}"""
        )!!.error!!

        assertNull(error.fieldReasons)
        assertEquals("Contact not found", error.displayMessage)
    }
}
