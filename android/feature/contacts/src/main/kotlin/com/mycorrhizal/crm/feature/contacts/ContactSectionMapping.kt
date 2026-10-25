package com.mycorrhizal.crm.feature.contacts

import com.mycorrhizal.crm.model.network.Anniversary
import com.mycorrhizal.crm.model.network.AnniversaryDate
import com.mycorrhizal.crm.model.network.CRMEnvelope
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordInput
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.OrgUnit
import com.mycorrhizal.crm.model.network.Organization
import com.mycorrhizal.crm.model.network.PersonalInfo
import com.mycorrhizal.crm.model.network.PersonalInfoMeta
import com.mycorrhizal.crm.model.network.Title
import com.mycorrhizal.crm.model.network.upsertEntryPeriod
import com.mycorrhizal.crm.model.network.yearTemporalRange
import com.mycorrhizal.crm.model.util.parsePartialDateForEdit
import java.util.UUID

/**
 * Issue #1728: form data + pure record mapping for the three contact-page
 * drill-downs that moved out of the edit form (skills/interests, work,
 * anniversaries). They all save through the existing full-record PUT with
 * If-Match, so these functions rebuild the whole [ContactRecordInput] from the
 * loaded record with only the changed collection replaced — every field the
 * drill-down does not model rides along unchanged.
 */

data class PersonalInfoFormData(
    val kind: String = "expertise",
    val value: String = "",
    /** Proficiency (expertise: beginner/average/expert; hobby/interest: high/medium/low). */
    val level: String? = null,
    val label: String? = null,
    val notes: String? = null,
    val sensitivity: String = "normal",
    val favorite: Boolean = false
)

data class OrganizationFormData(
    val name: String = "",
    val department: String = "",
    val startYear: String = "",
    val endYear: String = ""
)

data class TitleFormData(
    val name: String = "",
    val kind: String = "title",
    val startYear: String = "",
    val endYear: String = ""
)

data class AnniversaryFormData(
    /** Non-birth kinds only; birthday stays in the contact edit form. */
    val kind: String = "wedding",
    /** Partial-date editor string: YYYY-MM-DD or --MM-DD. */
    val date: String = ""
)

/** Expertise uses beginner/average/expert; hobby and interest use high/medium/low. */
fun personalInfoLevelOptions(kind: String): List<String> = when (kind) {
    "expertise" -> listOf("beginner", "average", "expert")
    else -> listOf("high", "medium", "low")
}

fun personalInfoLevelLabelRes(level: String?): Int = when (level) {
    "beginner" -> com.mycorrhizal.crm.ui.R.string.contact_personal_info_level_beginner
    "average" -> com.mycorrhizal.crm.ui.R.string.contact_personal_info_level_average
    "expert" -> com.mycorrhizal.crm.ui.R.string.contact_personal_info_level_expert
    "high" -> com.mycorrhizal.crm.ui.R.string.contact_personal_info_level_high
    "medium" -> com.mycorrhizal.crm.ui.R.string.contact_personal_info_level_medium
    "low" -> com.mycorrhizal.crm.ui.R.string.contact_personal_info_level_low
    else -> com.mycorrhizal.crm.ui.R.string.contact_personal_info_level_none
}

/**
 * Add or replace one [PersonalInfo] entry and its [PersonalInfoMeta] entry
 * (issue #1738), keyed by a stable Card ID. Editing preserves every field the
 * dialog does not surface (notably `listAs`) via `copy`, and leaves every
 * other entry's metadata untouched.
 *
 * A normal sensitivity, no notes and no favorite collapse the metadata entry
 * away entirely; an entry deleted from the card is pruned from the metadata map
 * (the server also prunes orphans, but not sending them keeps the wire minimal).
 */
fun ContactRecordResponse.withPersonalInfo(
    editingId: String?,
    form: PersonalInfoFormData,
    newId: String = UUID.randomUUID().toString()
): ContactRecordInput {
    val existing = editingId?.let { id -> card?.personalInfo?.firstOrNull { it.id == id } }
    val id = existing?.id ?: editingId ?: newId
    val entry = (existing ?: PersonalInfo()).copy(
        id = id,
        kind = form.kind,
        value = form.value.trim(),
        level = form.level?.trim()?.ifBlank { null },
        label = form.label?.trim()?.ifBlank { null }
    )
    val entries = card?.personalInfo.orEmpty()
    val next = if (existing != null) {
        entries.map { if (it.id == existing.id) entry else it }
    } else {
        entries + entry
    }

    val metaById = crm?.personalInfoMeta.orEmpty().toMutableMap()
    val meta = (metaById[id] ?: PersonalInfoMeta()).copy(
        sensitivity = form.sensitivity.takeUnless { it == "normal" || it.isBlank() },
        favorite = form.favorite.takeIf { it },
        notes = form.notes?.trim()?.ifBlank { null }
    )
    if (meta.sensitivity == null && meta.favorite == null && meta.notes == null &&
        meta.source == null && meta.confidence == null && meta.lastConfirmed == null
    ) {
        metaById.remove(id)
    } else {
        metaById[id] = meta
    }

    return withCardAndEnvelope(
        card = cardOrEmpty().copy(personalInfo = next.ifEmpty { null }),
        crm = crmOrEmpty().copy(personalInfoMeta = metaById.ifEmpty { null })
    )
}

fun ContactRecordResponse.withoutPersonalInfo(id: String): ContactRecordInput {
    val next = card?.personalInfo.orEmpty().filterNot { it.id == id }
    val metaById = crm?.personalInfoMeta.orEmpty() - id
    return withCardAndEnvelope(
        card = cardOrEmpty().copy(personalInfo = next.ifEmpty { null }),
        crm = crmOrEmpty().copy(personalInfoMeta = metaById.ifEmpty { null })
    )
}

fun ContactRecordResponse.withOrganization(
    editingId: String?,
    form: OrganizationFormData,
    newId: String = UUID.randomUUID().toString()
): ContactRecordInput {
    val existing = editingId?.let { id -> card?.organizations?.firstOrNull { it.id == id } }
    val id = existing?.id ?: editingId ?: newId
    val entry = (existing ?: Organization()).copy(
        id = id,
        name = form.name.trim(),
        units = form.department.trim().takeIf { it.isNotBlank() }?.let {
            listOf(
                OrgUnit(name = it)
            )
        }
    )
    val organizations = card?.organizations.orEmpty()
    val next = if (existing != null) {
        organizations.map { if (it.id == existing.id) entry else it }
    } else {
        organizations + entry
    }
    val periods = upsertEntryPeriod(
        crm?.periods.orEmpty(),
        "organization",
        id,
        yearTemporalRange(form.startYear, form.endYear)
    )
    return withCardAndEnvelope(
        card = cardOrEmpty().copy(organizations = next.ifEmpty { null }),
        crm = crmOrEmpty().copy(periods = periods.ifEmpty { null })
    )
}

fun ContactRecordResponse.withoutOrganization(id: String): ContactRecordInput {
    val next = card?.organizations.orEmpty().filterNot { it.id == id }
    val periods = upsertEntryPeriod(crm?.periods.orEmpty(), "organization", id, null)
    return withCardAndEnvelope(
        card = cardOrEmpty().copy(organizations = next.ifEmpty { null }),
        crm = crmOrEmpty().copy(periods = periods.ifEmpty { null })
    )
}

fun ContactRecordResponse.withTitle(
    editingId: String?,
    form: TitleFormData,
    newId: String = UUID.randomUUID().toString()
): ContactRecordInput {
    val existing = editingId?.let { id -> card?.titles?.firstOrNull { it.id == id } }
    val id = existing?.id ?: editingId ?: newId
    val entry = (existing ?: Title()).copy(
        id = id,
        name = form.name.trim(),
        kind = form.kind.ifBlank { "title" }
    )
    val titles = card?.titles.orEmpty()
    val next = if (existing != null) {
        titles.map { if (it.id == existing.id) entry else it }
    } else {
        titles + entry
    }
    val periods = upsertEntryPeriod(
        crm?.periods.orEmpty(),
        "title",
        id,
        yearTemporalRange(form.startYear, form.endYear)
    )
    return withCardAndEnvelope(
        card = cardOrEmpty().copy(titles = next.ifEmpty { null }),
        crm = crmOrEmpty().copy(periods = periods.ifEmpty { null })
    )
}

fun ContactRecordResponse.withoutTitle(id: String): ContactRecordInput {
    val next = card?.titles.orEmpty().filterNot { it.id == id }
    val periods = upsertEntryPeriod(crm?.periods.orEmpty(), "title", id, null)
    return withCardAndEnvelope(
        card = cardOrEmpty().copy(titles = next.ifEmpty { null }),
        crm = crmOrEmpty().copy(periods = periods.ifEmpty { null })
    )
}

/** Work information is free CRM text; an explicitly blank dialog value clears it. */
fun ContactRecordResponse.withWorkInformation(text: String): ContactRecordInput =
    withCardAndEnvelope(
        card = cardOrEmpty(),
        crm = crmOrEmpty().copy(workInformation = text.trim().ifBlank { null })
    )

/**
 * Add or replace a non-birth [Anniversary]. The header birthday entry is never
 * touched; editing preserves `place` and every other field via `copy`.
 */
fun ContactRecordResponse.withAnniversary(
    editingId: String?,
    form: AnniversaryFormData,
    newId: String = UUID.randomUUID().toString()
): ContactRecordInput {
    val existing = editingId?.let { id -> card?.anniversaries?.firstOrNull { it.id == id } }
    val id = existing?.id ?: editingId ?: newId
    val entry = (existing ?: Anniversary()).copy(
        id = id,
        kind = form.kind,
        date = AnniversaryDate(partial = parsePartialDateForEdit(form.date))
    )
    val entries = card?.anniversaries.orEmpty()
    val next = if (existing != null) {
        entries.map { if (it.id == existing.id) entry else it }
    } else {
        entries + entry
    }
    return withCardAndEnvelope(card = cardOrEmpty().copy(anniversaries = next.ifEmpty { null }))
}

fun ContactRecordResponse.withoutAnniversary(id: String): ContactRecordInput {
    val next = card?.anniversaries.orEmpty().filterNot { it.id == id }
    return withCardAndEnvelope(card = cardOrEmpty().copy(anniversaries = next.ifEmpty { null }))
}

/** Non-birth entries only — the birthday lives in the identity form's header. */
fun ContactRecordResponse.nonBirthAnniversaries(): List<Anniversary> =
    card?.anniversaries.orEmpty().filter { it.kind != "birth" }

private fun ContactRecordResponse.cardOrEmpty(): Card = card ?: Card()

private fun ContactRecordResponse.crmOrEmpty(): CRMEnvelope = crm ?: CRMEnvelope()

private fun ContactRecordResponse.withCardAndEnvelope(
    card: Card,
    crm: CRMEnvelope = crmOrEmpty()
): ContactRecordInput = ContactRecordInput(gender = gender, card = card, crm = crm)
