package com.mycorrhizal.crm.feature.contacts.entities

import androidx.annotation.StringRes
import androidx.compose.runtime.Composable
import androidx.compose.ui.res.stringResource
import com.mycorrhizal.crm.model.network.EntityKind
import com.mycorrhizal.crm.model.network.TemporalRange
import com.mycorrhizal.crm.ui.R

/** Plural tab label of a kind ("Organisations"). */
@StringRes
internal fun entityKindTabRes(kind: String): Int = when (kind) {
    EntityKind.PLACE -> R.string.shared_entities_kind_place
    EntityKind.PET -> R.string.shared_entities_kind_pet
    else -> R.string.shared_entities_kind_organisation
}

/** Singular label of a kind ("Organisation"). */
@StringRes
internal fun entityKindOneRes(kind: String): Int = when (kind) {
    EntityKind.PLACE -> R.string.shared_entities_kind_one_place
    EntityKind.PET -> R.string.shared_entities_kind_one_pet
    else -> R.string.shared_entities_kind_one_organisation
}

@StringRes
internal fun entityNewRes(kind: String): Int = when (kind) {
    EntityKind.PLACE -> R.string.shared_entities_new_place
    EntityKind.PET -> R.string.shared_entities_new_pet
    else -> R.string.shared_entities_new_organisation
}

@StringRes
internal fun entityEmptyRes(kind: String): Int = when (kind) {
    EntityKind.PLACE -> R.string.shared_entities_empty_place
    EntityKind.PET -> R.string.shared_entities_empty_pet
    else -> R.string.shared_entities_empty_organisation
}

@StringRes
internal fun entitySensitivityRes(sensitivity: String): Int = when (sensitivity) {
    "private" -> R.string.shared_entities_sensitivity_private
    "secret" -> R.string.shared_entities_sensitivity_secret
    else -> R.string.shared_entities_sensitivity_normal
}

/** The translated role, or null for a role this client does not know (shown verbatim). */
@StringRes
internal fun entityRoleRes(kind: String, role: String): Int? = when (kind to role) {
    EntityKind.ORGANISATION to "employee" -> R.string.shared_entities_role_organisation_employee
    EntityKind.ORGANISATION to "owner" -> R.string.shared_entities_role_organisation_owner
    EntityKind.ORGANISATION to "contractor" -> R.string.shared_entities_role_organisation_contractor
    EntityKind.ORGANISATION to "volunteer" -> R.string.shared_entities_role_organisation_volunteer
    EntityKind.ORGANISATION to "member" -> R.string.shared_entities_role_organisation_member
    EntityKind.ORGANISATION to "other" -> R.string.shared_entities_role_organisation_other
    EntityKind.PLACE to "resident" -> R.string.shared_entities_role_place_resident
    EntityKind.PLACE to "owner" -> R.string.shared_entities_role_place_owner
    EntityKind.PLACE to "visitor" -> R.string.shared_entities_role_place_visitor
    EntityKind.PLACE to "other" -> R.string.shared_entities_role_place_other
    EntityKind.PET to "owner" -> R.string.shared_entities_role_pet_owner
    EntityKind.PET to "carer" -> R.string.shared_entities_role_pet_carer
    EntityKind.PET to "other" -> R.string.shared_entities_role_pet_other
    else -> null
}

/** "Employee" or "Employee · Head chef" when a free-text qualifier is set. */
@Composable
internal fun entityRoleText(kind: String, role: String, roleLabel: String): String {
    val base = entityRoleRes(kind, role)?.let { stringResource(it) } ?: role
    return if (roleLabel.isBlank()) base else "$base · $roleLabel"
}

/** "2019 – 2024", "2019 –", "– 2024", or null when there is no period. */
internal fun entityPeriodText(range: TemporalRange?): String? {
    val start = range?.start?.year
    val end = range?.end?.year
    return when {
        start != null && end != null -> "$start – $end"
        start != null -> "$start –"
        end != null -> "– $end"
        else -> null
    }
}
