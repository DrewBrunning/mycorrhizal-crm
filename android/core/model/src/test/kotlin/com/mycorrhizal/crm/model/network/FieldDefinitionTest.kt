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

    // --- isMulti ---

    @Test
    fun `isMulti reflects the multi constraint`() {
        assertEquals(true, isMulti(FieldDefinition(type = "string", constraints = FieldConstraints(multi = true))))
        assertEquals(false, isMulti(FieldDefinition(type = "string", constraints = FieldConstraints(multi = false))))
        assertEquals(false, isMulti(FieldDefinition(type = "string", constraints = null)))
    }

    // --- emptyEditorValue ---

    @Test
    fun `emptyEditorValue is an empty string for a plain scalar`() {
        assertEquals("", emptyEditorValue(FieldDefinition(type = "string")))
        assertEquals("", emptyEditorValue(FieldDefinition(type = "enum")))
    }

    @Test
    fun `emptyEditorValue is false for boolean`() {
        assertEquals(false, emptyEditorValue(FieldDefinition(type = "boolean")))
    }

    @Test
    fun `emptyEditorValue is an empty list for multi`() {
        assertEquals(
            emptyList<String>(),
            emptyEditorValue(FieldDefinition(type = "string", constraints = FieldConstraints(multi = true))),
        )
    }

    // --- isEditorValueEmpty ---

    @Test
    fun `isEditorValueEmpty is true for a blank string scalar`() {
        val def = FieldDefinition(type = "string")
        assertEquals(true, isEditorValueEmpty(def, ""))
        assertEquals(true, isEditorValueEmpty(def, "   "))
        assertEquals(true, isEditorValueEmpty(def, null))
    }

    @Test
    fun `isEditorValueEmpty is false for a non-blank string scalar`() {
        assertEquals(false, isEditorValueEmpty(FieldDefinition(type = "string"), "Latte"))
    }

    @Test
    fun `isEditorValueEmpty is never true for boolean since false is a real value`() {
        val def = FieldDefinition(type = "boolean")
        assertEquals(false, isEditorValueEmpty(def, false))
        assertEquals(false, isEditorValueEmpty(def, true))
    }

    @Test
    fun `isEditorValueEmpty for multi is true only when every row is blank`() {
        val def = FieldDefinition(type = "string", constraints = FieldConstraints(multi = true))
        assertEquals(true, isEditorValueEmpty(def, emptyList<String>()))
        assertEquals(true, isEditorValueEmpty(def, listOf("", "  ")))
        assertEquals(false, isEditorValueEmpty(def, listOf("", "oat")))
    }

    // --- wireToEditorValue ---

    @Test
    fun `wireToEditorValue renders a scalar wire value as its display string`() {
        assertEquals("Latte", wireToEditorValue(FieldDefinition(type = "string"), "Latte"))
        assertEquals("5", wireToEditorValue(FieldDefinition(type = "number"), 5.0))
        assertEquals("5.5", wireToEditorValue(FieldDefinition(type = "number"), 5.5))
    }

    @Test
    fun `wireToEditorValue renders a missing scalar value as an empty string`() {
        assertEquals("", wireToEditorValue(FieldDefinition(type = "string"), null))
    }

    @Test
    fun `wireToEditorValue renders boolean as a real Boolean, defaulting false when absent`() {
        val def = FieldDefinition(type = "boolean")
        assertEquals(true, wireToEditorValue(def, true))
        assertEquals(false, wireToEditorValue(def, false))
        assertEquals(false, wireToEditorValue(def, null))
    }

    @Test
    fun `wireToEditorValue renders a multi wire array as a list of display strings`() {
        val def = FieldDefinition(type = "string", constraints = FieldConstraints(multi = true))
        assertEquals(listOf("oat", "almond"), wireToEditorValue(def, listOf("oat", "almond")))
    }

    @Test
    fun `wireToEditorValue renders a missing multi value as an empty list, not a crash`() {
        val def = FieldDefinition(type = "string", constraints = FieldConstraints(multi = true))
        assertEquals(emptyList<String>(), wireToEditorValue(def, null))
    }

    // --- editorToWireValue ---

    @Test
    fun `editorToWireValue passes a string editor value through unchanged`() {
        assertEquals("Latte", editorToWireValue(FieldDefinition(type = "string"), "Latte"))
    }

    @Test
    fun `editorToWireValue parses a number editor string into a Double`() {
        assertEquals(5.5, editorToWireValue(FieldDefinition(type = "number"), "5.5"))
    }

    @Test
    fun `editorToWireValue for an unparsable number editor string is null, not a crash`() {
        assertEquals(null, editorToWireValue(FieldDefinition(type = "number"), "not a number"))
    }

    @Test
    fun `editorToWireValue converts a boolean editor value`() {
        val def = FieldDefinition(type = "boolean")
        assertEquals(true, editorToWireValue(def, true))
        assertEquals(false, editorToWireValue(def, false))
    }

    @Test
    fun `editorToWireValue converts every row of a multi editor list per the scalar type`() {
        val def = FieldDefinition(type = "number", constraints = FieldConstraints(multi = true))
        assertEquals(listOf(1.0, 2.5), editorToWireValue(def, listOf("1", "2.5")))
    }

    @Test
    fun `editorToWireValue for an empty multi editor list is an empty list, not null`() {
        val def = FieldDefinition(type = "string", constraints = FieldConstraints(multi = true))
        assertEquals(emptyList<String>(), editorToWireValue(def, emptyList<String>()))
    }

    @Test
    fun `wireToEditorValue then editorToWireValue round-trips a multi number field`() {
        val def = FieldDefinition(type = "number", constraints = FieldConstraints(multi = true))
        val wire = listOf(1.0, 2.5)
        val editor = wireToEditorValue(def, wire)
        assertEquals(wire, editorToWireValue(def, editor))
    }
}
