package com.mycorrhizal.crm.feature.timeline

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.CalendarToday
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DatePicker
import androidx.compose.material3.DatePickerDialog
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExposedDropdownMenuBox
import androidx.compose.material3.ExposedDropdownMenuDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SelectableDates
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.material3.rememberDatePickerState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.unit.dp
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.hilt.navigation.compose.hiltViewModel
import com.mycorrhizal.crm.model.network.ReminderRecurrence
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.R
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneOffset

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ReminderFormScreen(
    onSaved: () -> Unit,
    onBack: () -> Unit,
    viewModel: ReminderFormViewModel = hiltViewModel(),
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

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
                title = {
                    Text(
                        text = if (state.isEdit) {
                            stringResource(R.string.reminder_edit)
                        } else {
                            stringResource(R.string.reminder_new)
                        },
                        style = MaterialTheme.typography.titleLarge,
                    )
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary,
                    actionIconContentColor = MaterialTheme.colorScheme.onPrimary,
                ),
            )
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
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
                onSave = viewModel::save,
                modifier = Modifier.padding(padding),
            )
        }
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
    onSave: () -> Unit,
    modifier: Modifier = Modifier,
) {
    var recurrenceExpanded by remember { mutableStateOf(false) }
    var showDatePicker by remember { mutableStateOf(false) }

    Column(
        modifier = modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        OutlinedTextField(
            value = state.message,
            onValueChange = onMessageChange,
            label = { Text(stringResource(R.string.reminder_message)) },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
        OutlinedTextField(
            value = state.remindAt.take(10),
            onValueChange = onRemindAtChange,
            label = { Text(stringResource(R.string.reminder_remind_at)) },
            placeholder = { Text(stringResource(R.string.reminder_remind_at_hint)) },
            singleLine = true,
            readOnly = true,
            trailingIcon = {
                IconButton(onClick = { showDatePicker = true }) {
                    Icon(Icons.Outlined.CalendarToday, contentDescription = stringResource(R.string.reminder_pick_date))
                }
            },
            modifier = Modifier.fillMaxWidth(),
        )
        ExposedDropdownMenuBox(
            expanded = recurrenceExpanded,
            onExpandedChange = { recurrenceExpanded = it },
        ) {
            OutlinedTextField(
                value = stringResource(recurrenceLabelRes(state.recurrence)),
                onValueChange = {},
                readOnly = true,
                label = { Text(stringResource(R.string.reminder_recurrence)) },
                trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded = recurrenceExpanded) },
                modifier = Modifier.fillMaxWidth().menuAnchor(),
            )
            ExposedDropdownMenu(
                expanded = recurrenceExpanded,
                onDismissRequest = { recurrenceExpanded = false },
            ) {
                ReminderRecurrence.ALL.forEach { option ->
                    DropdownMenuItem(
                        text = { Text(stringResource(recurrenceLabelRes(option))) },
                        onClick = {
                            onRecurrenceChange(option)
                            recurrenceExpanded = false
                        },
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
                    role = Role.Switch,
                ),
                headlineContent = {
                    Text(stringResource(R.string.reminder_reoccur_from_completion), style = MaterialTheme.typography.bodyLarge)
                },
                supportingContent = {
                    Text(stringResource(R.string.reminder_reoccur_from_completion_hint), style = MaterialTheme.typography.bodySmall)
                },
                trailingContent = {
                    Switch(checked = state.reoccurFromCompletion, onCheckedChange = null)
                },
            )
        }
        ListItem(
            modifier = Modifier.toggleable(
                value = state.byMail,
                onValueChange = onByMailChange,
                role = Role.Switch,
            ),
            headlineContent = { Text(stringResource(R.string.reminder_email), style = MaterialTheme.typography.bodyLarge) },
            trailingContent = {
                Switch(checked = state.byMail, onCheckedChange = null)
            },
        )
        val savingLabel = stringResource(R.string.a11y_state_saving)
        Button(
            onClick = onSave,
            enabled = !state.isSaving,
            modifier = Modifier
                .fillMaxWidth()
                .semantics { if (state.isSaving) stateDescription = savingLabel },
        ) {
            if (state.isSaving) {
                CircularProgressIndicator(modifier = Modifier.padding(end = 8.dp))
            }
            Text(if (state.isEdit) stringResource(R.string.reminder_save) else stringResource(R.string.reminder_create))
        }
    }

    if (showDatePicker) {
        ReminderDatePickerDialog(
            initial = state.remindAt,
            onCreate = { date -> onRemindAtChange(date); showDatePicker = false },
            onDismiss = { showDatePicker = false },
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

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ReminderDatePickerDialog(
    initial: String,
    onCreate: (String) -> Unit,
    onDismiss: () -> Unit,
) {
    val today = LocalDate.now()
    val initialMillis = initial.take(10).let { part ->
        if (part.length == 10) {
            runCatching { LocalDate.parse(part).atStartOfDay(ZoneOffset.UTC).toInstant().toEpochMilli() }.getOrNull()
        } else {
            null
        }
    }
    val pickerState = rememberDatePickerState(
        initialSelectedDateMillis = initialMillis,
        // Web's `min: today` attribute lets an existing past date (an overdue
        // reminder being edited) stay in the field but blocks *picking* a new
        // date before today. Mirror that: the initial date is always selectable,
        // anything else must be today or later.
        selectableDates = object : SelectableDates {
            override fun isSelectableDate(utcTimeMillis: Long): Boolean {
                if (utcTimeMillis == initialMillis) return true
                return Instant.ofEpochMilli(utcTimeMillis).atZone(ZoneOffset.UTC).toLocalDate() >= today
            }
        },
    )

    DatePickerDialog(
        onDismissRequest = onDismiss,
        confirmButton = {
            TextButton(onClick = {
                pickerState.selectedDateMillis?.let { millis ->
                    val date = Instant.ofEpochMilli(millis).atZone(ZoneOffset.UTC).toLocalDate()
                    onCreate("${date}T00:00:00Z")
                }
            }) {
                Text(stringResource(R.string.action_confirm))
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) {
                Text(stringResource(R.string.action_cancel))
            }
        },
    ) {
        DatePicker(state = pickerState, showModeToggle = false)
    }
}
