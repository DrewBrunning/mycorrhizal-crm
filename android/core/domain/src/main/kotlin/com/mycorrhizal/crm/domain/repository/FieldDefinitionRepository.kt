package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.ContactFieldValuesInput
import com.mycorrhizal.crm.model.network.FieldDefinition
import com.mycorrhizal.crm.model.network.FieldDefinitionInput
import com.mycorrhizal.crm.model.network.FieldValue

/**
 * Custom field-definition CRUD plus per-contact value read/write (T6/T7 web parity; T84 shipped
 * the Android read-only slice, issue #830 adds the write path). Own file rather than
 * `SettingsRepositories.kt`'s grab-bag, matching `TagRepository`/`CircleRepository`'s precedent:
 * field definitions are read by `feature/contacts` (custom-field value display/editing) as well
 * as written by `feature/settings` (the field-definitions management screen), so this is
 * genuinely cross-cutting, not settings-local. Thin over `ApiClient` — field definitions are
 * per-user config, not offline contact data, so unlike `TagRepository` there is no Room mirror,
 * the same posture `WebhookRepository`/`ApiTokenRepository` take.
 */
interface FieldDefinitionRepository {
    /** All of the user's field definitions. */
    suspend fun list(limit: Int? = null): Result<List<FieldDefinition>>

    /** One field definition by id — used to prefill the edit form. */
    suspend fun get(id: String): Result<FieldDefinition>

    /** Create a field definition; 409 on a duplicate (user, key) pair. */
    suspend fun create(input: FieldDefinitionInput): Result<FieldDefinition>

    /** Update a field definition's editable fields (key is immutable; see [FieldDefinitionInput]). */
    suspend fun update(id: String, input: FieldDefinitionInput): Result<FieldDefinition>

    /** Delete a field definition; its FieldValues cascade server-side. */
    suspend fun delete(id: String): Result<Unit>

    /**
     * Persist the full display order (issue #1210). [order] must be every one of the user's
     * definition ids exactly once; the backend rejects an incomplete, duplicated, or foreign set
     * with a 400. Returns the reordered definitions.
     */
    suspend fun reorder(order: List<String>): Result<List<FieldDefinition>>

    /** A contact's current custom-field values. */
    suspend fun contactValues(contactId: Int): Result<List<FieldValue>>

    /** Full-replace a contact's custom-field values — see [ContactFieldValuesInput]'s doc comment. */
    suspend fun replaceContactValues(contactId: Int, input: ContactFieldValuesInput): Result<List<FieldValue>>
}
