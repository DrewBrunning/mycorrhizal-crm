package com.mycorrhizal.crm.feature.contacts.entities

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.listSaver
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.model.network.Entity
import com.mycorrhizal.crm.model.network.EntityCore
import com.mycorrhizal.crm.model.network.EntityCoreResult
import com.mycorrhizal.crm.model.network.EntityForm
import com.mycorrhizal.crm.model.network.EntityInput
import com.mycorrhizal.crm.model.network.EntityKind
import com.mycorrhizal.crm.model.network.EntitySensitivity
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.DiscardChangesDialog
import com.mycorrhizal.crm.ui.components.DropdownOption
import com.mycorrhizal.crm.ui.components.LabelledDropdown
import com.mycorrhizal.crm.ui.components.localizedMessage

private const val NAME_MAX = 200
private const val NOTES_MAX = 5000

private val EntityFormSaver = listSaver<EntityForm, String>(
    save = {
        listOf(
            it.sortAs, it.unitsText, it.url, it.street, it.city, it.region, it.postal,
            it.country, it.species, it.breed, it.birth, it.death, it.notes
        )
    },
    restore = {
        EntityForm(
            sortAs = it[0], unitsText = it[1], url = it[2], street = it[3], city = it[4],
            region = it[5], postal = it[6], country = it[7], species = it[8], breed = it[9],
            birth = it[10], death = it[11], notes = it[12]
        )
    }
)

/**
 * Create / edit one entity (ADR 0043 decision 1). The kind is fixed once the entity exists, so an
 * edit never offers it. [onSave] gets the finished input; the caller owns the network call and
 * passes back [saving] / [error].
 */
@Composable
fun EntityFormDialog(
    kind: String,
    entity: Entity?,
    saving: Boolean,
    error: ApiError?,
    onSave: (EntityInput) -> Unit,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier
) {
    val initialForm = remember(entity?.id) { EntityCore.toForm(kind, entity?.core) }
    var name by rememberSaveable(entity?.id) { mutableStateOf(entity?.name.orEmpty()) }
    var sensitivity by rememberSaveable(entity?.id) {
        mutableStateOf(entity?.sensitivity ?: EntitySensitivity.NORMAL)
    }
    var form by rememberSaveable(
        entity?.id,
        stateSaver = EntityFormSaver
    ) { mutableStateOf(initialForm) }
    var nameError by remember { mutableStateOf(false) }
    var dateError by remember { mutableStateOf<String?>(null) }
    var confirmDiscard by remember { mutableStateOf(false) }

    val dirty = name != entity?.name.orEmpty() || form != initialForm ||
        sensitivity != (entity?.sensitivity ?: EntitySensitivity.NORMAL)
    val requestClose = { if (dirty && !saving) confirmDiscard = true else onDismiss() }

    val title = if (entity != null) {
        stringResource(R.string.shared_entities_form_edit_title, entity.name)
    } else {
        stringResource(entityNewRes(kind))
    }
    val invalidDateText = dateError?.let {
        stringResource(
            R.string.shared_entities_form_invalid_date,
            stringResource(
                if (it ==
                    "death"
                ) {
                    R.string.shared_entities_field_death
                } else {
                    R.string.shared_entities_field_birth
                }
            )
        )
    }

    AlertDialog(
        modifier = modifier.testTag("entity-form-dialog"),
        onDismissRequest = requestClose,
        title = { Text(title) },
        text = {
            Column(
                modifier = Modifier.verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                OutlinedTextField(
                    value = name,
                    onValueChange = {
                        name = it.take(NAME_MAX)
                        nameError = false
                    },
                    label = { Text(stringResource(R.string.shared_entities_field_name)) },
                    isError = nameError,
                    supportingText = if (nameError) {
                        { Text(stringResource(R.string.shared_entities_form_name_required)) }
                    } else {
                        null
                    },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth().testTag("entity-form-name")
                )
                when (kind) {
                    EntityKind.ORGANISATION -> OrganisationFields(form) { form = it }

                    EntityKind.PLACE -> PlaceFields(form) { form = it }

                    else -> PetFields(form) {
                        dateError = null
                        form = it
                    }
                }
                NotesField(form.notes) { form = form.copy(notes = it.take(NOTES_MAX)) }
                LabelledDropdown(
                    label = stringResource(R.string.shared_entities_sensitivity_label),
                    valueText = stringResource(entitySensitivityRes(sensitivity)),
                    options = EntitySensitivity.ALL.map {
                        DropdownOption(it, stringResource(entitySensitivityRes(it)))
                    },
                    onSelect = { sensitivity = it },
                    modifier = Modifier.testTag("entity-form-sensitivity")
                )
                if (invalidDateText != null) {
                    Text(
                        text = invalidDateText,
                        color = MaterialTheme.colorScheme.error,
                        style = MaterialTheme.typography.bodySmall
                    )
                }
                if (error != null) {
                    Text(
                        text = error.localizedMessage(),
                        color = MaterialTheme.colorScheme.error,
                        style = MaterialTheme.typography.bodySmall,
                        modifier = Modifier.testTag("entity-form-error")
                    )
                }
            }
        },
        confirmButton = {
            TextButton(
                enabled = !saving,
                modifier = Modifier.testTag("entity-form-save"),
                onClick = {
                    val trimmed = name.trim()
                    if (trimmed.isEmpty()) {
                        nameError = true
                        return@TextButton
                    }
                    when (val core = EntityCore.toCore(kind, trimmed, form, entity?.core)) {
                        is EntityCoreResult.InvalidDate -> dateError = core.field

                        is EntityCoreResult.Ok -> onSave(
                            EntityInput(
                                kind = kind,
                                name = trimmed,
                                core = core.core,
                                sensitivity = sensitivity
                            )
                        )
                    }
                }
            ) { Text(stringResource(R.string.action_save)) }
        },
        dismissButton = {
            TextButton(enabled = !saving, onClick = requestClose) {
                Text(stringResource(R.string.action_cancel))
            }
        }
    )
    if (confirmDiscard) {
        DiscardChangesDialog(
            entityLabel = stringResource(entityKindOneRes(kind)).lowercase(),
            onKeepEditing = { confirmDiscard = false },
            onDiscard = {
                confirmDiscard = false
                onDismiss()
            }
        )
    }
}

@Composable
private fun TextField(
    value: String,
    label: Int,
    tag: String,
    onChange: (String) -> Unit,
    modifier: Modifier = Modifier,
    singleLine: Boolean = true,
    keyboardType: KeyboardType = KeyboardType.Text,
    supporting: Int? = null
) {
    OutlinedTextField(
        value = value,
        onValueChange = onChange,
        label = { Text(stringResource(label)) },
        supportingText = supporting?.let { { Text(stringResource(it)) } },
        singleLine = singleLine,
        keyboardOptions = KeyboardOptions(keyboardType = keyboardType),
        modifier = modifier.fillMaxWidth().testTag(tag)
    )
}

@Composable
private fun OrganisationFields(form: EntityForm, onChange: (EntityForm) -> Unit) {
    Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
        TextField(
            form.url,
            R.string.shared_entities_field_url,
            "entity-form-url",
            onChange = { onChange(form.copy(url = it)) },
            keyboardType = KeyboardType.Uri
        )
        TextField(
            form.sortAs,
            R.string.shared_entities_field_sort_as,
            "entity-form-sort-as",
            onChange = { onChange(form.copy(sortAs = it)) }
        )
        TextField(
            form.unitsText,
            R.string.shared_entities_field_units,
            "entity-form-units",
            onChange = { onChange(form.copy(unitsText = it)) },
            singleLine = false
        )
    }
}

@Composable
private fun PlaceFields(form: EntityForm, onChange: (EntityForm) -> Unit) {
    Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
        TextField(
            form.street,
            R.string.shared_entities_field_street,
            "entity-form-street",
            onChange = { onChange(form.copy(street = it)) }
        )
        TextField(
            form.city,
            R.string.shared_entities_field_city,
            "entity-form-city",
            onChange = { onChange(form.copy(city = it)) }
        )
        TextField(
            form.region,
            R.string.shared_entities_field_region,
            "entity-form-region",
            onChange = { onChange(form.copy(region = it)) }
        )
        TextField(
            form.postal,
            R.string.shared_entities_field_postal,
            "entity-form-postal",
            onChange = { onChange(form.copy(postal = it)) }
        )
        TextField(
            form.country,
            R.string.shared_entities_field_country,
            "entity-form-country",
            onChange = { onChange(form.copy(country = it)) }
        )
    }
}

@Composable
private fun PetFields(form: EntityForm, onChange: (EntityForm) -> Unit) {
    Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
        TextField(
            form.species,
            R.string.shared_entities_field_species,
            "entity-form-species",
            onChange = { onChange(form.copy(species = it)) }
        )
        TextField(
            form.breed,
            R.string.shared_entities_field_breed,
            "entity-form-breed",
            onChange = { onChange(form.copy(breed = it)) }
        )
        TextField(
            form.birth,
            R.string.shared_entities_field_birth,
            "entity-form-birth",
            onChange = { onChange(form.copy(birth = it)) },
            keyboardType = KeyboardType.Number,
            supporting = R.string.shared_entities_field_date_hint
        )
        TextField(
            form.death,
            R.string.shared_entities_field_death,
            "entity-form-death",
            onChange = { onChange(form.copy(death = it)) },
            keyboardType = KeyboardType.Number,
            supporting = R.string.shared_entities_field_date_hint
        )
    }
}

@Composable
private fun NotesField(value: String, onChange: (String) -> Unit) {
    TextField(
        value,
        R.string.shared_entities_field_notes,
        "entity-form-notes",
        onChange = onChange,
        singleLine = false
    )
}
