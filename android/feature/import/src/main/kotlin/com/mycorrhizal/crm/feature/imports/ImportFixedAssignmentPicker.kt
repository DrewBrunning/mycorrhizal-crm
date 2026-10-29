package com.mycorrhizal.crm.feature.imports

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.AssistChip
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.ImportFixedAssignment
import com.mycorrhizal.crm.model.network.Tag
import com.mycorrhizal.crm.ui.R

/**
 * Issue #2135: the mapping-step picker for circles/tags applied to every imported contact
 * (web's `ImportFixedAssignmentPicker`). Chips toggle selection; "New circle/tag" creates one inline.
 */
@OptIn(ExperimentalLayoutApi::class, ExperimentalMaterial3Api::class)
@Composable
internal fun ImportFixedAssignmentPicker(
    circles: List<Circle>,
    tags: List<Tag>,
    selectedCircleIds: Set<String>,
    selectedTagIds: Set<String>,
    onToggleCircle: (String) -> Unit,
    onToggleTag: (String) -> Unit,
    onCreateCircle: (String) -> Unit,
    onCreateTag: (String) -> Unit,
    modifier: Modifier = Modifier
) {
    var creating by rememberSaveable { mutableStateOf<String?>(null) }
    Column(
        verticalArrangement = Arrangement.spacedBy(8.dp),
        modifier = modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp, vertical = 8.dp)
            .testTag("csv-fixed-assignment-picker")
    ) {
        Text(
            stringResource(R.string.import_fixed_title),
            style = MaterialTheme.typography.titleSmall
        )
        Text(
            stringResource(R.string.import_fixed_description),
            style = MaterialTheme.typography.bodySmall
        )
        Text(
            stringResource(R.string.import_fixed_circles),
            style = MaterialTheme.typography.labelLarge
        )
        FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            circles.forEach { circle ->
                FilterChip(
                    selected = circle.id in selectedCircleIds,
                    onClick = { onToggleCircle(circle.id) },
                    label = { Text(circle.name) }
                )
            }
            AssistChip(
                onClick = { creating = KIND_CIRCLE },
                label = { Text(stringResource(R.string.import_fixed_new_circle)) }
            )
        }
        Text(
            stringResource(R.string.import_fixed_tags),
            style = MaterialTheme.typography.labelLarge
        )
        FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            tags.forEach { tag ->
                FilterChip(
                    selected = tag.id in selectedTagIds,
                    onClick = { onToggleTag(tag.id) },
                    label = { Text(tag.name) }
                )
            }
            AssistChip(
                onClick = { creating = KIND_TAG },
                label = { Text(stringResource(R.string.import_fixed_new_tag)) }
            )
        }
    }
    creating?.let { kind ->
        CreateNameDialog(
            title = stringResource(
                if (kind ==
                    KIND_CIRCLE
                ) {
                    R.string.import_fixed_new_circle
                } else {
                    R.string.import_fixed_new_tag
                }
            ),
            onCreate = { name ->
                if (kind == KIND_CIRCLE) onCreateCircle(name) else onCreateTag(name)
                creating = null
            },
            onDismiss = { creating = null }
        )
    }
}

@Composable
private fun CreateNameDialog(title: String, onCreate: (String) -> Unit, onDismiss: () -> Unit) {
    var name by rememberSaveable { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(title) },
        text = {
            OutlinedTextField(
                value = name,
                onValueChange = { name = it },
                label = { Text(stringResource(R.string.import_fixed_name)) },
                singleLine = true,
                modifier = Modifier.fillMaxWidth().testTag("csv-fixed-new-name")
            )
        },
        confirmButton = {
            TextButton(onClick = { onCreate(name) }, enabled = name.isNotBlank()) {
                Text(stringResource(R.string.import_fixed_create))
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) }
        }
    )
}

/** Issue #2135: the preview-step recap of what every imported contact will be added to. */
@Composable
internal fun FixedAssignmentSummary(fixed: ImportFixedAssignment, modifier: Modifier = Modifier) {
    val names = (fixed.circles.map { it.name } + fixed.tags.map { it.name }).joinToString(", ")
    Text(
        text = pluralStringResource(
            R.plurals.import_fixed_summary,
            fixed.membershipCount,
            names,
            fixed.membershipCount
        ),
        style = MaterialTheme.typography.bodyMedium,
        modifier = modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp, vertical = 8.dp)
            .testTag("csv-fixed-assignment")
    )
}

private const val KIND_CIRCLE = "circle"
private const val KIND_TAG = "tag"
