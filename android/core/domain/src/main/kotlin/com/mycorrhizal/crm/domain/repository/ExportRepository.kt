package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.ExportLossPreflightResponse

/**
 * Full-dataset export (web-parity for the Settings → Data export section).
 * Each method returns the raw file bytes for the format; sensitivity and
 * field-selection defaults are the backend's own (all sections, private/
 * secret excluded) unless a method exposes the explicit override.
 */
interface ExportRepository {
    /** GET /export — the full per-user backup as one CSV file. */
    suspend fun exportDataCsv(): Result<ByteArray>

    /**
     * GET /export/vcf (no vcard_uid) — every contact as one .vcf file.
     * [version] 3 or null → 4.0. [sections]/[includeSensitive] are the T9
     * selective-export params (issue #835); null [sections] means every
     * section, the backend's default.
     */
    suspend fun exportContactsVcf(
        version: Int?,
        sections: List<String>? = null,
        includeSensitive: Boolean = false,
    ): Result<ByteArray>

    /**
     * GET /export/jscontact — every contact as a JSContact (RFC 9553) JSON
     * array. See [exportContactsVcf] for the [sections]/[includeSensitive]
     * param contract (issue #835).
     */
    suspend fun exportContactsJsContact(
        sections: List<String>? = null,
        includeSensitive: Boolean = false,
    ): Result<ByteArray>

    /** GET /audit/export — the caller's full audit trail as CSV. */
    suspend fun exportAuditLogCsv(): Result<ByteArray>

    /**
     * GET /export/preflight (DATA-02, issue #442) — what an export with the
     * given params would lose, without producing the file. [format] is one
     * of `vcard4`/`vcard3`/`jscontact`.
     */
    suspend fun preflight(
        format: String,
        sections: List<String>,
        includeSensitive: Boolean,
    ): Result<ExportLossPreflightResponse>
}
