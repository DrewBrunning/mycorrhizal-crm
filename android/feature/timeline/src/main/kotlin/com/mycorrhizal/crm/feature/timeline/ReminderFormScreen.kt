package com.mycorrhizal.crm.feature.timeline

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExposedDropdownMenuBox
import androidx.compose.material3.ExposedDropdownMenuDefaults
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.ReminderRecurrence
import com.mycorrhizal.crm.model.util.DateFieldMode
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.DateField
import com.mycorrhizal.crm.ui.components.FormScaffold
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.components.WriteConflictDialog
import java.time.LocalDate

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ReminderFormScreen(
    onSaved: () -> Unit,
    onBack: () -> Unit,
    viewModel: ReminderFormViewModel = hiltViewModel()
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val events by viewModel.events.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }

    LaunchedEffect(events) {
        if (events is ReminderFormEvent.Saved) {
            viewModel.onSaveShown()
            onSaved()
        }
    }

    FormScaffold(
        title = if (state.isEdit) {
            stringResource(
                R.string.reminder_edit
            )
        } else {
            stringResource(R.string.reminder_new)
        },
        onBack = onBack,
        saveLabel = if (state.isEdit) {
            stringResource(
                R.string.reminder_save
            )
        } else {
            stringResource(R.string.reminder_create)
        },
        isSaving = state.isSaving,
        onSave = viewModel::save,
        snackbarHostState = snackbarHostState,
        isDirty = state.isDirty,
        entityLabel = stringResource(R.string.entity_reminder),
        showSaveBar = !state.isLoading
    ) { padding ->
        when {
            state.isLoading -> LoadingSkeleton(modifier = Modifier.padding(padding))

            else -> ReminderFormContent(
                state = state,
                onMessageChange = viewModel::onMessageChange,
                onRemindAtChange = viewModel::onRemindAtChange,
                onRecurrenceChange = viewModel::onRecurrenceChange,
                onByMailChange = viewModel::onByMailChange,
                onReoccurFromCompletionChange = viewModel::onReoccurFromCompletionChange,
                modifier = Modifier.padding(padding)
            )
        }
    }

    if (state.conflict) {
        WriteConflictDialog(
            onLoadLatest = viewModel::onConflictLoadLatest,
            onKeep = viewModel::onConflictKeepEditing
        )
    }

    val errorMessage = state.errorRes?.let { stringResource(it) } ?: state.error
    errorMessage?.let { message ->
        LaunchedEffect(message) {
            snackbarHostState.showSnackbar(message)
            viewModel.onErrorShown()
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ReminderFormContent(
    state: ReminderFormState,
    onMessageChange: (String) -> Unit,
    onRemindAtChange: (String) -> Unit,
    onRecurrenceChange: (String) -> Unit,
    onByMailChange: (Boolean) -> Unit,
    onReoccurFromCompletionChange: (Boolean) -> Unit,
    modifier: Modifier = Modifier
) {
    var recurrenceExpanded by remember { mutableStateOf(false) }

    Column(
        modifier = modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp)
    ) {
        OutlinedTextField(
            value = state.message,
            onValueChange = onMessageChange,
            label = { Text(stringResource(R.string.reminder_message)) },
            singleLine = true,
            modifier = Modifier.fillMaxWidth()
        )
        DateField(
            value = state.remindAt,
            onValueChange = onRemindAtChange,
            label = stringResource(R.string.reminder_remind_at),
            mode = DateFieldMode.DATE_UTC_MIDNIGHT,
            selectableFrom = LocalDate.now(),
            testTag = "reminder-date",
            modifier = Modifier.fillMaxWidth()
        )
        ExposedDropdownMenuBox(
            expanded = recurrenceExpanded,
            onExpandedChange = { recurrenceExpanded = it }
        ) {
            OutlinedTextField(
                value = stringResource(recurrenceLabelRes(state.recurrence)),
                onValueChange = {},
                readOnly = true,
                label = { Text(stringResource(R.string.reminder_recurrence)) },
                trailingIcon = {
                    ExposedDropdownMenuDefaults.TrailingIcon(
                        expanded = recurrenceExpanded
                    )
                },
                modifier = Modifier.fillMaxWidth().menuAnchor()
            )
            ExposedDropdownMenu(
                expanded = recurrenceExpanded,
                onDismissRequest = { recurrenceExpanded = false }
            ) {
                ReminderRecurrence.ALL.forEach { option ->
                    DropdownMenuItem(
                        text = { Text(stringResource(recurrenceLabelRes(option))) },
                        onClick = {
                            onRecurrenceChange(option)
                            recurrenceExpanded = false
                        }
                    )
                }
            }
        }
        if (state.recurrence != ReminderRecurrence.ONCE) {
            // #199: a bare Switch in trailingContent has no text/contentDescription
            // of its own; the headline/supporting Text sit in sibling slots
            // TalkBack never merges into it. Modifier.toggleable on the ListItem
            // itself merges all three into one accessible name.
            ListItem(
                modifier = Modifier.toggleable(
                    value = state.reoccurFromCompletion,
                    onValueChange = onReoccurFromCompletionChange,
                    role = Role.Switch
                ),
                headlineContent = {
                    Text(
                        stringResource(R.string.reminder_reoccur_from_completion),
                        style = MaterialTheme.typography.bodyLarge
                    )
                },
                supportingContent = {
                    Text(
                        stringResource(R.string.reminder_reoccur_from_completion_hint),
                        style = MaterialTheme.typography.bodySmall
                    )
                },
                trailingContent = {
                    Switch(checked = state.reoccurFromCompletion, onCheckedChange = null)
                }
            )
        }
        ListItem(
            modifier = Modifier.toggleable(
                value = state.byMail,
                onValueChange = onByMailChange,
                role = Role.Switch
            ),
            headlineContent = {
                Text(
                    stringResource(R.string.reminder_email),
                    style = MaterialTheme.typography.bodyLarge
                )
            },
            trailingContent = {
                Switch(checked = state.byMail, onCheckedChange = null)
            }
        )
    }
}

/** Localized recurrence label resource, mirroring the web `reminders.recurrence.*` keys. */
private fun recurrenceLabelRes(token: String): Int = when (token) {
    ReminderRecurrence.ONCE -> R.string.reminder_recurrence_once
    ReminderRecurrence.WEEKLY -> R.string.reminder_recurrence_weekly
    ReminderRecurrence.MONTHLY -> R.string.reminder_recurrence_monthly
    ReminderRecurrence.QUARTERLY -> R.string.reminder_recurrence_quarterly
    ReminderRecurrence.SIX_MONTHS -> R.string.reminder_recurrence_six_months
    ReminderRecurrence.YEARLY -> R.string.reminder_recurrence_yearly
    else -> R.string.reminder_recurrence_once
}
