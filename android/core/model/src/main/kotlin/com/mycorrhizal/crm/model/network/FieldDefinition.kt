package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * Custom field v2 (T6/T7 backend/web; T84 Android) — a user-defined, typed property on a
 * contact, distinct from the native Card fields. Mirrors `backend/models/field_definition.go`
 * and `backend/models/dtos.go` by hand. There is no dynamic type-list endpoint anywhere in this
 * codebase (`/CLAUDE.md` frontend trap #4) — [FIELD_TYPES] below is a hand-mirrored copy of the
 * backend's `oneof=string text number boolean date datetime uri email phone enum` validator and
 * must be kept in sync by hand if the backend ever adds a type.
 */
val FIELD_TYPES: List<String> = listOf(
    "string", "text", "number", "boolean", "date", "datetime", "uri", "email", "phone", "enum",
)

/** Type-dependent validation rules (§94.3). `multi` marks list-of-<type> rather than a scalar. */
@JsonClass(generateAdapter = true)
data class FieldConstraints(
    val min: Double? = null,
    val max: Double? = null,
    @Json(name = "maxLength") val maxLength: Int? = null,
    val pattern: String? = null,
    /** The allowed-value list for type `enum`. */
    val values: List<String>? = null,
    val multi: Boolean = false,
)

@JsonClass(generateAdapter = true)
data class FieldDefinition(
    val id: String = "",
    val label: String? = null,
    val key: String? = null,
    val target: String? = null,
    /** One of [FIELD_TYPES]. */
    val type: String? = null,
    val constraints: FieldConstraints? = null,
    /** "internal-only" (default) or "vcard:X-<NAME>". */
    val projection: String? = null,
    /** normal | private | secret (§91.13) — all sensitivities are returned on this owner-only surface. */
    val sensitivity: String? = null,
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "updated_at") val updatedAt: String? = null,
)

/**
 * GET /field-definitions. `fieldDefinitions` is nullable on the wire even though the endpoint
 * always emits the JSON key (it's wrapped in a `gin.H` map, not a struct with `omitempty`) —
 * a `Find` into a `nil` Go slice for a user with zero definitions serializes as JSON `null`,
 * which Moshi codegen rejects for a non-nullable `List<T>` even with a Kotlin default (defaults
 * only apply when the key is *absent*, not when it's present as `null`; see `/CLAUDE.md` frontend
 * trap #8 for the sibling absent-key failure mode). [definitions] normalizes all three shapes
 * (absent / null / `[]`) to a plain empty list.
 */
@JsonClass(generateAdapter = true)
data class FieldDefinitionsResponse(
    @Json(name = "field_definitions") val fieldDefinitions: List<FieldDefinition>? = null,
    val total: Int = 0,
    @Json(name = "next_cursor") val nextCursor: String? = null,
    val limit: Int = 0,
) {
    val definitions: List<FieldDefinition> get() = fieldDefinitions.orEmpty()
}

/**
 * One typed value of a [FieldDefinition] for one contact. [value] is the raw JSON payload
 * (§94.4): a scalar definition holds a bare value (string/number/boolean), a `multi` definition
 * holds a JSON array. Moshi's `Any` support decodes a JSON number as `Double` — see
 * [fieldValueDisplay] for the integer-vs-decimal rendering that follows from that.
 */
@JsonClass(generateAdapter = true)
data class FieldValue(
    val id: Int = 0,
    @Json(name = "field_definition_id") val fieldDefinitionId: String = "",
    @Json(name = "entity_id") val entityId: String? = null,
    val value: Any? = null,
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "updated_at") val updatedAt: String? = null,
)

/** GET/PUT /contacts/:id/field-values response envelope — see [FieldDefinitionsResponse]'s doc
 *  comment for why [fieldValues] is nullable and [values] is the normalized accessor. */
@JsonClass(generateAdapter = true)
data class ContactFieldValuesResponse(
    @Json(name = "field_values") val fieldValues: List<FieldValue>? = null,
    val message: String? = null,
) {
    val values: List<FieldValue> get() = fieldValues.orEmpty()
}

/** One element of [ContactFieldValuesInput] — the value to write for one definition. */
@JsonClass(generateAdapter = true)
data class FieldValueInput(
    @Json(name = "field_definition_id") val fieldDefinitionId: String,
    val value: Any?,
)

/**
 * PUT /contacts/:id/field-values request body — a full-replace of the contact's values: any
 * existing value for a definition not present in [fieldValues] is deleted server-side.
 */
@JsonClass(generateAdapter = true)
data class ContactFieldValuesInput(
    @Json(name = "field_values") val fieldValues: List<FieldValueInput>,
)

/**
 * POST/PUT /field-definitions request body (issue #830) — mirrors `backend/models/dtos.go`'s
 * `FieldDefinitionInput` exactly. [key] is required on create but ignored (never re-assigned) by
 * `UpdateFieldDefinition` server-side — the create/edit form disables the key field when editing
 * rather than relying on the server to reject a change.
 */
@JsonClass(generateAdapter = true)
data class FieldDefinitionInput(
    val label: String,
    val key: String,
    val target: String? = null,
    val type: String,
    val constraints: FieldConstraints? = null,
    val projection: String? = null,
    val sensitivity: String? = null,
)

/** POST /field-definitions response — wrapped `{ message, field_definition }`, unlike
 *  Get/Update's raw-object responses (see `field_definition_controller.go`'s CreateFieldDefinition). */
@JsonClass(generateAdapter = true)
data class CreateFieldDefinitionResponse(
    val message: String? = null,
    @Json(name = "field_definition") val fieldDefinition: FieldDefinition? = null,
)

/** Whether [definition] is a multi-valued (list-of-scalar) field — `FieldConstraints.Multi`. */
fun isMulti(definition: FieldDefinition): Boolean = definition.constraints?.multi == true

/**
 * The empty/"no value yet" editor state for [definition] — an empty string for a scalar,
 * `false` for boolean, an empty list for multi. Mirrors web's `emptyEditorValue`
 * (`frontend/src/api/fieldDefinitions.ts`).
 */
fun emptyEditorValue(definition: FieldDefinition): Any = when {
    isMulti(definition) -> emptyList<String>()
    definition.type == "boolean" -> false
    else -> ""
}

/**
 * True when [editor] (editor-state shape, not a raw wire value) represents "no value" and
 * should be omitted from the save payload entirely rather than sent. Mirrors web's
 * `isEditorValueEmpty`. A multi list is empty when it has no non-blank rows; boolean is never
 * "empty" (false is a real value, matching [fieldValueDisplay]'s own boolean-is-always-set
 * rendering); every other scalar is empty exactly when its string editor state is blank.
 */
fun isEditorValueEmpty(definition: FieldDefinition, editor: Any?): Boolean = when {
    isMulti(definition) -> (editor as? List<*>).orEmpty().all { (it as? String).isNullOrBlank() }
    definition.type == "boolean" -> false
    else -> (editor as? String).isNullOrBlank()
}

/**
 * Converts a raw wire [value] (as decoded from JSON — `Double`/`Boolean`/`String`/`List<*>`, or
 * null when the contact has no value for [definition] yet) into editor state: a `String` for
 * every scalar type except boolean, `Boolean` for boolean, `List<String>` for multi. Mirrors
 * web's `wireToEditorValue`.
 */
fun wireToEditorValue(definition: FieldDefinition, value: Any?): Any = when {
    isMulti(definition) -> (value as? List<*>)?.map { scalarWireToEditorString(it) } ?: emptyList<String>()
    definition.type == "boolean" -> value == true
    else -> scalarWireToEditorString(value)
}

/**
 * Converts editor state (see [wireToEditorValue]) back into a raw wire value ready for
 * `FieldValueInput.value` — a `Double` for number, `Boolean` for boolean, `String` otherwise, or
 * a `List<Any?>` of those for multi. Mirrors web's `editorToWireValue`. Callers should check
 * [isEditorValueEmpty] first and send `null` (omit the definition from the payload) instead of
 * calling this for an empty editor value.
 */
fun editorToWireValue(definition: FieldDefinition, editor: Any?): Any? = when {
    isMulti(definition) -> (editor as? List<*>).orEmpty().map { scalarEditorToWireValue(definition, it) }
    else -> scalarEditorToWireValue(definition, editor)
}

private fun scalarEditorToWireValue(definition: FieldDefinition, editor: Any?): Any? = when (definition.type) {
    "number" -> (editor as? String)?.toDoubleOrNull()
    "boolean" -> editor == true
    else -> editor as? String ?: ""
}

private fun scalarWireToEditorString(value: Any?): String = scalarValueDisplay(value)

/**
 * Human-readable rendering of a [FieldValue.value], mirroring web's `fieldValueToDisplay`
 * (`frontend/src/api/fieldDefinitions.ts`) so the same value reads the same way on both
 * platforms. A `multi` field joins with "; ", matching the CSV export's own separator.
 */
fun fieldValueDisplay(definition: FieldDefinition, value: Any?): String {
    if (definition.constraints?.multi == true) {
        val list = value as? List<*> ?: return ""
        return list.mapNotNull { scalarValueDisplay(it).takeIf(String::isNotEmpty) }.joinToString("; ")
    }
    if (definition.type == "boolean") return if (value == true) "true" else "false"
    return scalarValueDisplay(value)
}

private fun scalarValueDisplay(value: Any?): String = when (value) {
    null -> ""
    is Boolean -> if (value) "true" else "false"
    // JSON numbers decode as Double via Moshi's Any support; render a whole number without
    // a trailing ".0" (matching how a user typed "5", not "5.0").
    is Double -> if (value == Math.floor(value) && !value.isInfinite()) {
        value.toLong().toString()
    } else {
        value.toString()
    }
    else -> value.toString()
}
