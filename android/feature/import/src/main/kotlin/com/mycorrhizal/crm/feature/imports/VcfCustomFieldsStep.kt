package com.mycorrhizal.crm.feature.imports

import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.model.network.DiscoveredCustomProperty
import com.mycorrhizal.crm.model.network.FIELD_TYPES
import com.mycorrhizal.crm.model.network.FieldDefinition
import com.mycorrhizal.crm.model.network.IMPORT_FIELD_ACTION_CREATE
import com.mycorrhizal.crm.model.network.IMPORT_FIELD_ACTION_IGNORE
import com.mycorrhizal.crm.model.network.IMPORT_FIELD_ACTION_MAP
import com.mycorrhizal.crm.model.network.ImportFieldMapping
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.DropdownOption
import com.mycorrhizal.crm.ui.components.LabelledDropdown

/**
 * Issue #514 / #2183 / #2523: the "Custom fields" step between upload and review. Mirrors web's
 * `renderCustomFieldsStep` in `ImportContactsDialog.tsx`: each discovered X-* property is kept in
 * passthrough (default), mapped into an existing definition, or promoted to a new one. Every
 * choice shows its own help text; "map" is disabled, with the reason as its help text, when the
 * user has no custom fields yet.
 */
@Composable
internal fun CustomFieldsStep(
    candidates: List<DiscoveredCustomProperty>,
    definitions: List<FieldDefinition>,
    mappings: Map<String, ImportFieldMapping>,
    canContinue: Boolean,
    actions: CustomFieldActions
) {
    Column(modifier = Modifier.fillMaxSize().testTag("vcf-custom-fields-step")) {
        LazyColumn(
            modifier = Modifier.weight(1f),
            contentPadding = PaddingValues(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            item {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(
                        stringResource(R.string.import_vcf_cf_title),
                        style = MaterialTheme.typography.titleMedium
                    )
                    Text(
                        stringResource(R.string.import_vcf_cf_description),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                }
            }
            items(candidates, key = { it.name }) { candidate ->
                val mapping = mappings[candidate.name]
                    ?: ImportFieldMapping(candidate.name, IMPORT_FIELD_ACTION_IGNORE)
                CandidateCard(
                    name = candidate.name,
                    value = candidate.value,
                    mapping = mapping,
                    definitions = definitions,
                    onActionChange = { actions.setFieldMappingAction(candidate.name, it) },
                    onDefinitionChange = { actions.setFieldMappingDefinition(candidate.name, it) },
                    onLabelChange = { actions.setFieldMappingLabel(candidate.name, it) },
                    onTypeChange = { actions.setFieldMappingType(candidate.name, it) }
                )
            }
        }
        Button(
            onClick = actions::continueFromCustomFields,
            enabled = canContinue,
            modifier = Modifier.fillMaxWidth().padding(16.dp).testTag("vcf-cf-continue")
        ) {
            Text(stringResource(R.string.import_vcf_cf_continue))
        }
    }
}

@Composable
private fun CandidateCard(
    name: String,
    value: String,
    mapping: ImportFieldMapping,
    definitions: List<FieldDefinition>,
    onActionChange: (String) -> Unit,
    onDefinitionChange: (String) -> Unit,
    onLabelChange: (String) -> Unit,
    onTypeChange: (String) -> Unit
) {
    Card(modifier = Modifier.fillMaxWidth().testTag("vcf-cf-card-$name")) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            Text(name, style = MaterialTheme.typography.titleSmall)
            if (value.isNotEmpty()) {
                Text(
                    stringResource(R.string.import_vcf_cf_sample_value, value),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 2,
                    overflow = TextOverflow.Ellipsis
                )
            }
            Column(modifier = Modifier.selectableGroup()) {
                ActionOption(
                    tag = "vcf-cf-$name-ignore",
                    selected = mapping.action == IMPORT_FIELD_ACTION_IGNORE,
                    labelRes = R.string.import_vcf_cf_action_ignore,
                    helpRes = R.string.import_vcf_cf_help_ignore,
                    onClick = { onActionChange(IMPORT_FIELD_ACTION_IGNORE) }
                )
                val mapAvailable = definitions.isNotEmpty()
                ActionOption(
                    tag = "vcf-cf-$name-map",
                    selected = mapping.action == IMPORT_FIELD_ACTION_MAP,
                    labelRes = R.string.import_vcf_cf_action_map,
                    helpRes = if (mapAvailable) {
                        R.string.import_vcf_cf_help_map
                    } else {
                        R.string.import_vcf_cf_help_map_empty
                    },
                    enabled = mapAvailable,
                    onClick = { onActionChange(IMPORT_FIELD_ACTION_MAP) }
                )
                ActionOption(
                    tag = "vcf-cf-$name-create",
                    selected = mapping.action == IMPORT_FIELD_ACTION_CREATE,
                    labelRes = R.string.import_vcf_cf_action_create,
                    helpRes = R.string.import_vcf_cf_help_create,
                    onClick = { onActionChange(IMPORT_FIELD_ACTION_CREATE) }
                )
            }
            when (mapping.action) {
                IMPORT_FIELD_ACTION_MAP -> LabelledDropdown(
                    label = stringResource(R.string.import_vcf_cf_field),
                    valueText = definitions.firstOrNull { it.id == mapping.fieldDefinitionId }
                        ?.label
                        ?: stringResource(R.string.import_vcf_cf_select_field),
                    options = definitions.map { DropdownOption(it.id, it.label.orEmpty()) },
                    onSelect = onDefinitionChange,
                    modifier = Modifier.testTag("vcf-cf-$name-field")
                )

                IMPORT_FIELD_ACTION_CREATE -> {
                    OutlinedTextField(
                        value = mapping.label.orEmpty(),
                        onValueChange = onLabelChange,
                        label = { Text(stringResource(R.string.import_vcf_cf_label)) },
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth().testTag("vcf-cf-$name-label")
                    )
                    LabelledDropdown(
                        label = stringResource(R.string.settings_custom_fields_type),
                        valueText = stringResource(fieldTypeLabelRes(mapping.type)),
                        options = FIELD_TYPES.map {
                            DropdownOption(it, stringResource(fieldTypeLabelRes(it)))
                        },
                        onSelect = onTypeChange,
                        modifier = Modifier.testTag("vcf-cf-$name-type")
                    )
                }
            }
        }
    }
}

@Composable
private fun ActionOption(
    tag: String,
    selected: Boolean,
    @StringRes labelRes: Int,
    @StringRes helpRes: Int,
    onClick: () -> Unit,
    enabled: Boolean = true
) {
    Row(
        verticalAlignment = Alignment.Top,
        modifier = Modifier
            .fillMaxWidth()
            .selectable(
                selected = selected,
                enabled = enabled,
                role = Role.RadioButton,
                onClick = onClick
            )
            .padding(vertical = 4.dp)
            .testTag(tag)
    ) {
        RadioButton(selected = selected, onClick = null, enabled = enabled)
        Column(modifier = Modifier.padding(start = 12.dp)) {
            Text(
                stringResource(labelRes),
                style = MaterialTheme.typography.bodyLarge,
                color = if (enabled) {
                    MaterialTheme.colorScheme.onSurface
                } else {
                    MaterialTheme.colorScheme.onSurface.copy(alpha = 0.38f)
                }
            )
            Text(
                stringResource(helpRes),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }
    }
}

/** Localized field-type label; the settings screen has the same mapping but this module does not
 *  depend on `feature:settings`. */
private fun fieldTypeLabelRes(type: String?): Int = when (type) {
    "text" -> R.string.custom_field_type_text
    "number" -> R.string.custom_field_type_number
    "boolean" -> R.string.custom_field_type_boolean
    "date" -> R.string.custom_field_type_date
    "datetime" -> R.string.custom_field_type_datetime
    "uri" -> R.string.custom_field_type_uri
    "email" -> R.string.custom_field_type_email
    "phone" -> R.string.custom_field_type_phone
    "enum" -> R.string.custom_field_type_enum
    else -> R.string.custom_field_type_string
}
