package com.mycorrhizal.crm.feature.contacts

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Checkbox
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.localizedMessage

/** Issue #2136: stateful host; closes and calls [onAdded] once every selection is a member. */
@Composable
internal fun AddToActivitiesDialog(
    contactId: Int,
    onDismiss: () -> Unit,
    onAdded: () -> Unit,
    viewModel: AddToActivitiesViewModel = hiltViewModel()
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val currentOnAdded by rememberUpdatedState(onAdded)
    LaunchedEffect(state.done) {
        if (state.done) currentOnAdded()
    }
    AddToActivitiesDialogContent(
        state = state,
        onQueryChange = viewModel::onQueryChange,
        onToggle = viewModel::toggle,
        onSubmit = { viewModel.submit(contactId) },
        onDismiss = onDismiss
    )
}

@Composable
internal fun AddToActivitiesDialogContent(
    state: AddToActivitiesUiState,
    onQueryChange: (String) -> Unit,
    onToggle: (Int) -> Unit,
    onSubmit: () -> Unit,
    onDismiss: () -> Unit
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.add_to_activities_title)) },
        text = {
            Column {
                OutlinedTextField(
                    value = state.query,
                    onValueChange = onQueryChange,
                    label = { Text(stringResource(R.string.add_to_activities_search)) },
                    singleLine = true,
                    modifier = Modifier
                        .fillMaxWidth()
                        .testTag("add-to-activities-search")
                )
                if (state.failedTitles.isNotEmpty()) {
                    Text(
                        text = stringResource(R.string.add_to_activities_not_found),
                        color = MaterialTheme.colorScheme.error,
                        style = MaterialTheme.typography.labelLarge,
                        modifier = Modifier.padding(top = 8.dp)
                    )
                    state.failedTitles.forEach {
                        Text(
                            text = it.ifBlank {
                                stringResource(
                                    R.string.add_to_activities_untitled
                                )
                            },
                            color = MaterialTheme.colorScheme.error,
                            style = MaterialTheme.typography.bodySmall
                        )
                    }
                }
                state.error?.let {
                    Text(
                        text = it.localizedMessage(),
                        color = MaterialTheme.colorScheme.error,
                        style = MaterialTheme.typography.bodySmall,
                        modifier = Modifier.padding(top = 8.dp)
                    )
                }
                if (!state.isLoading && state.activities.isEmpty()) {
                    Text(
                        text = stringResource(R.string.add_to_activities_empty),
                        modifier = Modifier.padding(top = 12.dp)
                    )
                }
                LazyColumn(modifier = Modifier.heightIn(max = 320.dp)) {
                    items(state.activities, key = { it.id }) { activity ->
                        val checked = activity.id in state.selected
                        val title = activity.title.orEmpty()
                            .ifBlank { stringResource(R.string.add_to_activities_untitled) }
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            modifier = Modifier
                                .fillMaxWidth()
                                .clickable(role = Role.Checkbox) { onToggle(activity.id) }
                                .testTag("add-to-activities-row-${activity.id}")
                        ) {
                            Checkbox(checked = checked, onCheckedChange = null)
                            Text(
                                text = title,
                                style = MaterialTheme.typography.bodyLarge,
                                modifier = Modifier.padding(
                                    start = 12.dp,
                                    top = 12.dp,
                                    bottom = 12.dp
                                )
                            )
                        }
                    }
                }
            }
        },
        confirmButton = {
            TextButton(
                onClick = onSubmit,
                enabled = state.canSubmit,
                modifier = Modifier.testTag("add-to-activities-submit")
            ) {
                Text(stringResource(R.string.add_to_activities_submit))
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) }
        }
    )
}
