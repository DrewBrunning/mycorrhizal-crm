package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.ContactFieldVisibility
import com.mycorrhizal.crm.model.network.ContactFieldVisibilityInput

/**
 * Issue #1997: one contact's cosmetic field-visibility overrides and hide-empty
 * mode. Resolution order is applied server-side (per-contact override > account
 * default > built-in default). Nothing here touches the contact record, so no
 * If-Match is sent.
 */
interface ContactFieldVisibilityRepository {
    /** GET /contacts/{id}/field-visibility — overrides, defaults and the resolved decision. */
    suspend fun get(contactId: Int): Result<ContactFieldVisibility>

    /** PUT /contacts/{id}/field-visibility — full replace of the override set. */
    suspend fun put(
        contactId: Int,
        input: ContactFieldVisibilityInput
    ): Result<ContactFieldVisibility>

    /** DELETE /contacts/{id}/field-visibility — reset to the account defaults. */
    suspend fun reset(contactId: Int): Result<ContactFieldVisibility>
}
