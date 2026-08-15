package com.mycorrhizal.crm.model.network

/**
 * The field-section tokens accepted by POST /contact-shares (and T9's export
 * field selection), mirroring backend/models/field_selection.go's
 * FieldSections() + IsSensitiveSection and the web frontend's
 * EXPORT_FIELD_SECTIONS (CLAUDE.md frontend trap #4 — hardcoded mirror, kept
 * in sync by hand: adding a token backend-side requires updating this list).
 */
object ShareFieldSections {
    data class Section(
        val token: String,
        val sensitive: Boolean,
    )

    val ALL: List<Section> = listOf(
        Section("emails", sensitive = false),
        Section("phones", sensitive = false),
        Section("addresses", sensitive = false),
        Section("organizations", sensitive = false),
        Section("anniversaries", sensitive = false),
        Section("media", sensitive = false),
        Section("online_services", sensitive = false),
        Section("links", sensitive = false),
        Section("notes", sensitive = false),
        Section("keywords", sensitive = false),
        Section("related_to", sensitive = true),
        Section("personal_info", sensitive = true),
        Section("speak_to_as", sensitive = false),
        Section("members", sensitive = false),
        Section("languages", sensitive = false),
        Section("custom_fields", sensitive = true),
    )

    /** Default selection: every non-sensitive section, matching the web dialog. */
    val DEFAULT_SELECTED: List<String> = ALL.filterNot { it.sensitive }.map { it.token }
}
