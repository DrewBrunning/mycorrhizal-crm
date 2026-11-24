package com.mycorrhizal.crm.ui.components

import androidx.annotation.StringRes
import com.mycorrhizal.crm.model.network.EntityKind
import com.mycorrhizal.crm.ui.R

/**
 * The translated link role of an entity kind, or null for a role this client does not know (show
 * it verbatim). Mirrors the backend's closed role vocabulary per kind (`entityLinkRoles`).
 */
@StringRes
fun entityRoleLabelRes(kind: String, role: String): Int? = when (kind to role) {
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
