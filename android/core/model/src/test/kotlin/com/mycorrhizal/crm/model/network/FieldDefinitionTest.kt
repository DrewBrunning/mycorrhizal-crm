package com.mycorrhizal.crm.model.network

import org.junit.Assert.assertEquals
import org.junit.Test

class FieldDefinitionTest {

    @Test
    fun `string value renders as-is`() {
        val def = FieldDefinition(type = "string")
        assertEquals("Latte", fieldValueDisplay(def, "Latte"))
    }

    @Test
    fun `number value renders without a trailing decimal for a whole number`() {
        val def = FieldDefinition(type = "number")
        assertEquals("5", fieldValueDisplay(def, 5.0))
    }

    @Test
    fun `number value keeps a real decimal`() {
        val def = FieldDefinition(type = "number")
        assertEquals("5.5", fieldValueDisplay(def, 5.5))
    }

    @Test
    fun `boolean true and false render as the literal words`() {
        val def = FieldDefinition(type = "boolean")
        assertEquals("true", fieldValueDisplay(def, true))
        assertEquals("false", fieldValueDisplay(def, false))
    }

    @Test
    fun `enum value renders as its plain string`() {
        val def = FieldDefinition(type = "enum", constraints = FieldConstraints(values = listOf("small", "large")))
        assertEquals("large", fieldValueDisplay(def, "large"))
    }

    @Test
    fun `a null value renders as an empty string`() {
        val def = FieldDefinition(type = "string")
        assertEquals("", fieldValueDisplay(def, null))
    }

    @Test
    fun `a multi field joins its elements with a semicolon`() {
        val def = FieldDefinition(type = "string", constraints = FieldConstraints(multi = true))
        assertEquals("oat; almond", fieldValueDisplay(def, listOf("oat", "almond")))
    }

    @Test
    fun `a multi field with no value renders as an empty string, not a crash`() {
        val def = FieldDefinition(type = "string", constraints = FieldConstraints(multi = true))
        assertEquals("", fieldValueDisplay(def, null))
    }
}
