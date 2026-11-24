package com.mycorrhizal.crm.feature.contacts.entities

import androidx.annotation.StringRes
import androidx.compose.runtime.Composable
import androidx.compose.ui.res.stringResource
import com.mycorrhizal.crm.model.network.EntityKind
import com.mycorrhizal.crm.model.network.TemporalRange
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.entityRoleLabelRes

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

/** Why a pair is suggested; an unknown backend token falls back to the weakest wording. */
@StringRes
internal fun entitySuggestionReasonRes(reason: String): Int = when (reason) {
    "exact_name" -> R.string.shared_entities_suggestions_reason_exact_name
    "address_components" -> R.string.shared_entities_suggestions_reason_address
    else -> R.string.shared_entities_suggestions_reason_similar_name
}

@StringRes
internal fun entitySensitivityRes(sensitivity: String): Int = when (sensitivity) {
    "private" -> R.string.shared_entities_sensitivity_private
    "secret" -> R.string.shared_entities_sensitivity_secret
    else -> R.string.shared_entities_sensitivity_normal
}

/** The translated role, or null for a role this client does not know (shown verbatim). */
@StringRes
internal fun entityRoleRes(kind: String, role: String): Int? = entityRoleLabelRes(kind, role)

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
