package com.mycorrhizal.crm.feature.contacts.entities

import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Checkbox
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.model.network.Entity
import com.mycorrhizal.crm.model.network.EntityCore
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.localizedMessage

@StringRes
internal fun mergeFieldLabelRes(field: String): Int = when (field) {
    "organization.sortAs" -> R.string.shared_entities_field_sort_as
    "organization.units" -> R.string.shared_entities_field_units
    "url" -> R.string.shared_entities_field_url
    "address" -> R.string.shared_entities_field_address
    "species" -> R.string.shared_entities_field_species
    "breed" -> R.string.shared_entities_field_breed
    "birth" -> R.string.shared_entities_field_birth
    "death" -> R.string.shared_entities_field_death
    else -> R.string.shared_entities_field_notes
}

/**
 * Merge this entity with another of the same kind (ADR 0043 decision 8). Nothing merges without
 * the explicit confirm; the "also copy" checkboxes start unticked (the backend `resolutions`
 * contract: only listed fields are filled, only where the kept one has none).
 *
 * Opened from an entity's page, the user searches for [entity]'s partner among [candidates].
 * Opened from a duplicate suggestion (#2520), [pairedWith] is the other half of the pair: it is
 * the partner already, so the search and the candidate list are not shown.
 */
@Composable
fun EntityMergeDialog(
    entity: Entity,
    candidates: List<Entity>,
    loading: Boolean,
    merging: Boolean,
    error: ApiError?,
    onSearch: (String) -> Unit,
    onConfirm: (other: Entity, keepThis: Boolean, fill: Set<String>) -> Unit,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier,
    pairedWith: Entity? = null
) {
    var query by rememberSaveable { mutableStateOf("") }
    var selectedId by rememberSaveable { mutableStateOf<String?>(null) }
    var keepThis by rememberSaveable { mutableStateOf(false) }
    var fill by remember(selectedId, keepThis) { mutableStateOf(emptySet<String>()) }
    val currentOnSearch by rememberUpdatedState(onSearch)
    LaunchedEffect(Unit) { if (pairedWith == null) currentOnSearch("") }
    val other = pairedWith ?: candidates.firstOrNull { it.id == selectedId }
    val survivor = if (keepThis) entity else other
    val loser = if (keepThis) other else entity
    val fillable = if (survivor != null && loser != null) {
        EntityCore.fillableMergeFields(survivor, loser)
    } else {
        emptyList()
    }

    AlertDialog(
        modifier = modifier.testTag("entity-merge-dialog"),
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.shared_entities_merge_title, entity.name)) },
        text = {
            Column(
                modifier = Modifier.verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                Text(
                    stringResource(R.string.shared_entities_merge_nothing_unless_confirmed),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
                if (pairedWith == null) {
                    OutlinedTextField(
                        value = query,
                        onValueChange = {
                            query = it
                            onSearch(it)
                        },
                        label = { Text(stringResource(R.string.shared_entities_merge_search)) },
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth().testTag("entity-merge-search")
                    )
                    Text(
                        stringResource(R.string.shared_entities_merge_into),
                        style = MaterialTheme.typography.labelLarge
                    )
                    if (!loading && candidates.isEmpty()) {
                        Text(
                            stringResource(R.string.shared_entities_merge_no_candidates),
                            style = MaterialTheme.typography.bodyMedium
                        )
                    }
                    LazyColumn(Modifier.heightIn(max = 200.dp).testTag("entity-merge-candidates")) {
                        items(candidates, key = { it.id }) { candidate ->
                            Row(
                                modifier = Modifier.fillMaxWidth()
                                    .selectable(
                                        selected = candidate.id == selectedId,
                                        role = Role.RadioButton,
                                        onClick = { selectedId = candidate.id }
                                    )
                                    .padding(vertical = 4.dp)
                                    .testTag("entity-merge-candidate-${candidate.id}"),
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                RadioButton(selected = candidate.id == selectedId, onClick = null)
                                Text(candidate.name, modifier = Modifier.padding(start = 8.dp))
                            }
                        }
                    }
                }
                if (other != null && survivor != null && loser != null) {
                    Text(
                        stringResource(
                            R.string.shared_entities_merge_intro,
                            loser.name,
                            survivor.name
                        ),
                        style = MaterialTheme.typography.bodyMedium
                    )
                    Row(
                        modifier = Modifier.fillMaxWidth()
                            .toggleable(
                                value = keepThis,
                                role = Role.Switch,
                                onValueChange = { keepThis = it }
                            )
                            .padding(vertical = 4.dp)
                            .testTag("entity-merge-swap"),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.SpaceBetween
                    ) {
                        Text(
                            stringResource(R.string.shared_entities_merge_keep_this),
                            modifier = Modifier.weight(1f)
                        )
                        Switch(checked = keepThis, onCheckedChange = null)
                    }
                    if (fillable.isNotEmpty()) {
                        Text(
                            stringResource(R.string.shared_entities_merge_fill, loser.name),
                            style = MaterialTheme.typography.labelLarge
                        )
                        fillable.forEach { field ->
                            Row(
                                modifier = Modifier.fillMaxWidth()
                                    .toggleable(
                                        value = field in fill,
                                        role = Role.Checkbox,
                                        onValueChange = { on ->
                                            fill = if (on) fill + field else fill - field
                                        }
                                    )
                                    .padding(vertical = 4.dp)
                                    .testTag("entity-merge-fill-$field"),
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                Checkbox(checked = field in fill, onCheckedChange = null)
                                Text(
                                    stringResource(mergeFieldLabelRes(field)),
                                    modifier = Modifier.padding(start = 8.dp)
                                )
                            }
                        }
                    }
                }
                if (error != null) {
                    Text(
                        error.localizedMessage(),
                        color = MaterialTheme.colorScheme.error,
                        style = MaterialTheme.typography.bodySmall,
                        modifier = Modifier.testTag("entity-merge-error")
                    )
                }
            }
        },
        confirmButton = {
            TextButton(
                enabled = other != null && !merging,
                modifier = Modifier.testTag("entity-merge-confirm"),
                onClick = { other?.let { onConfirm(it, keepThis, fill) } }
            ) { Text(stringResource(R.string.shared_entities_merge_confirm)) }
        },
        dismissButton = {
            TextButton(enabled = !merging, onClick = onDismiss) {
                Text(stringResource(R.string.action_cancel))
            }
        }
    )
}
