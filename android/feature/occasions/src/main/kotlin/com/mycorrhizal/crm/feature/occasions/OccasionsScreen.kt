package com.mycorrhizal.crm.feature.occasions

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material.icons.outlined.Event
import androidx.compose.material.icons.outlined.Group
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.DatePicker
import androidx.compose.material3.DatePickerDialog
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TimePicker
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.material3.rememberDatePickerState
import androidx.compose.material3.rememberTimePickerState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.OccasionEvent
import com.mycorrhizal.crm.model.registry.OccasionSensitivity
import com.mycorrhizal.crm.ui.R
import java.time.Instant
import java.time.ZoneOffset

/**
 * The Occasions events list (docs/adrs/0026-occasions-events.md, issue #1228):
 * create/edit/delete a hosted event, and open its attendee/RSVP ledger.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun OccasionsScreen(
    onBack: () -> Unit,
    onOpenAttendees: (String) -> Unit,
    viewModel: OccasionsViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }
    var editing by remember { mutableStateOf<OccasionEvent?>(null) }
    var showDialog by remember { mutableStateOf(false) }
    var pendingDelete by remember { mutableStateOf<OccasionEvent?>(null) }

    state.error?.let { message ->
        LaunchedEffect(message) {
            snackbarHostState.showSnackbar(message)
            viewModel.onErrorShown()
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
                title = { Text(stringResource(R.string.occasions_title), style = MaterialTheme.typography.titleLarge) },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary,
                    actionIconContentColor = MaterialTheme.colorScheme.onPrimary,
                ),
            )
        },
        floatingActionButton = {
            FloatingActionButton(onClick = { editing = null; showDialog = true }) {
                Icon(Icons.Outlined.Add, contentDescription = stringResource(R.string.occasions_add_event))
            }
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        Box(Modifier.fillMaxSize().padding(padding)) {
            when {
                state.errorRes != null -> CenteredMessage(stringResource(state.errorRes!!))
                state.isLoading && state.events.isEmpty() -> CenteredMessage(stringResource(R.string.a11y_state_loading))
                state.events.isEmpty() -> CenteredMessage(stringResource(R.string.occasions_empty))
                else -> LazyColumn(
                    modifier = Modifier.fillMaxSize().padding(horizontal = 16.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    items(state.events, key = { it.id }) { event ->
                        OccasionEventRow(
                            event = event,
                            onOpen = { onOpenAttendees(event.id) },
                            onEdit = { editing = event; showDialog = true },
                            onDelete = { pendingDelete = event },
                        )
                    }
                }
            }
        }
    }

    if (showDialog) {
        OccasionEventDialog(
            event = editing,
            onDismiss = { showDialog = false },
            onSave = { form ->
                val current = editing
                if (current != null) {
                    viewModel.update(current.id, form.title, form.startsAt, form.endsAt, form.location, form.sensitivity, form.notes)
                } else {
                    viewModel.create(form.title, form.startsAt, form.endsAt, form.location, form.sensitivity, form.notes)
                }
                showDialog = false
            },
        )
    }

    pendingDelete?.let { event ->
        AlertDialog(
            onDismissRequest = { pendingDelete = null },
            title = { Text(stringResource(R.string.occasions_delete_title)) },
            text = { Text(stringResource(R.string.occasions_delete_confirm, event.title)) },
            confirmButton = {
                TextButton(enabled = !state.isMutating, onClick = { viewModel.delete(event.id); pendingDelete = null }) {
                    Text(stringResource(R.string.action_delete))
                }
            },
            dismissButton = { TextButton(onClick = { pendingDelete = null }) { Text(stringResource(R.string.action_cancel)) } },
        )
    }
}

@Composable
internal fun CenteredMessage(text: String) {
    Box(Modifier.fillMaxSize().padding(24.dp), contentAlignment = Alignment.Center) {
        Text(text, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

@Composable
private fun OccasionEventRow(
    event: OccasionEvent,
    onOpen: () -> Unit,
    onEdit: () -> Unit,
    onDelete: () -> Unit,
) {
    Column(
        modifier = Modifier.fillMaxWidth().clickable(onClick = onOpen).padding(vertical = 8.dp),
    ) {
        Text(event.title, style = MaterialTheme.typography.bodyLarge, maxLines = 2, overflow = TextOverflow.Ellipsis)
        Text(formatEventWhen(event), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        event.location?.takeIf { it.isNotBlank() }?.let {
            Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
            TextButton(onClick = onOpen) {
                Icon(Icons.Outlined.Group, contentDescription = null)
                Text(stringResource(R.string.occasions_attendees))
            }
            IconButton(onClick = onEdit) {
                Icon(Icons.Outlined.Edit, contentDescription = stringResource(R.string.action_edit))
            }
            IconButton(onClick = onDelete) {
                Icon(Icons.Outlined.Delete, contentDescription = stringResource(R.string.action_delete))
            }
        }
    }
}

internal data class OccasionEventForm(
    val title: String,
    val startsAt: String,
    val endsAt: String?,
    val location: String?,
    val sensitivity: String,
    val notes: String?,
)

/**
 * The create/edit form. A start is required; an end, when given, must not
 * precede it.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun OccasionEventDialog(
    event: OccasionEvent?,
    onDismiss: () -> Unit,
    onSave: (OccasionEventForm) -> Unit,
) {
    val context = LocalContext.current
    var title by remember(event) { mutableStateOf(event?.title ?: "") }
    var startsAt by remember(event) { mutableStateOf(event?.startsAt ?: "") }
    var endsAt by remember(event) { mutableStateOf(event?.endsAt ?: "") }
    var location by remember(event) { mutableStateOf(event?.location ?: "") }
    var sensitivity by remember(event) { mutableStateOf(event?.sensitivity ?: OccasionSensitivity.NORMAL) }
    var notes by remember(event) { mutableStateOf(event?.notes ?: "") }
    var error by remember { mutableStateOf<String?>(null) }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = {
            Text(
                text = stringResource(if (event == null) R.string.occasions_create_title else R.string.occasions_edit_title),
                modifier = Modifier.semantics { heading() },
            )
        },
        text = {
            Column(
                modifier = Modifier.verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                OutlinedTextField(
                    value = title,
                    onValueChange = { title = it; error = null },
                    label = { Text(stringResource(R.string.occasions_event_title)) },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                InstantField(stringResource(R.string.occasions_starts_at), startsAt) { startsAt = it; error = null }
                InstantField(stringResource(R.string.occasions_ends_at), endsAt) { endsAt = it; error = null }
                OutlinedTextField(
                    value = location,
                    onValueChange = { location = it },
                    label = { Text(stringResource(R.string.occasions_location)) },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                SensitivitySelect(sensitivity) { sensitivity = it }
                OutlinedTextField(
                    value = notes,
                    onValueChange = { notes = it },
                    label = { Text(stringResource(R.string.occasions_notes)) },
                    minLines = 3,
                    modifier = Modifier.fillMaxWidth(),
                )
                error?.let { Text(it, color = MaterialTheme.colorScheme.error) }
            }
        },
        confirmButton = {
            TextButton(onClick = {
                val trimmed = title.trim()
                when {
                    trimmed.isEmpty() -> error = context.getString(R.string.occasions_validation_title_required)
                    startsAt.isBlank() -> error = context.getString(R.string.occasions_validation_start_required)
                    endsAt.isNotBlank() && endsAt < startsAt -> error = context.getString(R.string.occasions_validation_end_before_start)
                    else -> onSave(
                        OccasionEventForm(
                            title = trimmed,
                            startsAt = startsAt,
                            endsAt = endsAt.ifBlank { null },
                            location = location.trim().ifBlank { null },
                            sensitivity = sensitivity,
                            notes = notes.trim().ifBlank { null },
                        ),
                    )
                }
            }) { Text(stringResource(R.string.action_save)) }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) } },
    )
}

@Composable
private fun SensitivitySelect(value: String, onSelect: (String) -> Unit) {
    var expanded by remember { mutableStateOf(false) }
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text(
            stringResource(R.string.occasions_sensitivity),
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Box {
            OutlinedButton(onClick = { expanded = true }, modifier = Modifier.fillMaxWidth()) {
                Text(sensitivityLabel(value), maxLines = 1, overflow = TextOverflow.Ellipsis)
            }
            DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
                OccasionSensitivity.ALL.forEach { option ->
                    DropdownMenuItem(
                        text = { Text(sensitivityLabel(option)) },
                        onClick = { expanded = false; onSelect(option) },
                    )
                }
            }
        }
    }
}

@Composable
private fun sensitivityLabel(value: String): String = when (value) {
    OccasionSensitivity.PRIVATE -> stringResource(R.string.occasions_sensitivity_private)
    OccasionSensitivity.SECRET -> stringResource(R.string.occasions_sensitivity_secret)
    else -> stringResource(R.string.occasions_sensitivity_normal)
}

@Composable
private fun InstantField(label: String, value: String, onChange: (String) -> Unit) {
    var picking by remember { mutableStateOf(false) }
    OutlinedTextField(
        value = instantToDisplay(value),
        onValueChange = {},
        readOnly = true,
        label = { Text(label) },
        trailingIcon = {
            IconButton(onClick = { picking = true }) {
                Icon(Icons.Outlined.Event, contentDescription = label)
            }
        },
        modifier = Modifier.fillMaxWidth(),
    )
    if (picking) {
        InstantPickerDialog(
            initial = value,
            onConfirm = { iso -> onChange(iso); picking = false },
            onDismiss = { picking = false },
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun InstantPickerDialog(initial: String, onConfirm: (String) -> Unit, onDismiss: () -> Unit) {
    val fallback = remember { Instant.now() }
    val initialInstant = remember(initial) { runCatching { Instant.parse(initial) }.getOrDefault(fallback) }
    var step by remember { mutableStateOf(0) }
    val dateState = rememberDatePickerState(initialSelectedDateMillis = initialInstant.toEpochMilli())
    val timeState = rememberTimePickerState(
        initialHour = initialInstant.atZone(ZoneOffset.UTC).hour,
        initialMinute = initialInstant.atZone(ZoneOffset.UTC).minute,
        is24Hour = true,
    )

    if (step == 0) {
        DatePickerDialog(
            onDismissRequest = onDismiss,
            confirmButton = { TextButton(onClick = { step = 1 }) { Text(stringResource(R.string.action_confirm)) } },
            dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) } },
        ) {
            DatePicker(state = dateState, showModeToggle = false)
        }
    } else {
        AlertDialog(
            onDismissRequest = onDismiss,
            title = { Text(stringResource(R.string.occasions_time)) },
            text = { TimePicker(state = timeState) },
            confirmButton = {
                TextButton(onClick = {
                    val millis = dateState.selectedDateMillis ?: initialInstant.toEpochMilli()
                    onConfirm(toRfc3339(millis, timeState.hour, timeState.minute))
                }) { Text(stringResource(R.string.action_confirm)) }
            },
            dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) } },
        )
    }
}

/** Renders an RFC 3339 instant as `yyyy-MM-dd HH:mm` (UTC, no device zone). */
internal fun instantToDisplay(iso: String): String =
    iso.take(16).replace('T', ' ')

/** The start (and optional end) of an event, for the list row. */
internal fun formatEventWhen(event: OccasionEvent): String {
    val start = instantToDisplay(event.startsAt)
    val end = event.endsAt?.takeIf { it.isNotBlank() }?.let(::instantToDisplay)
    return if (end == null) start else "$start – $end"
}

/** Builds an RFC 3339 UTC instant from a date picker's UTC millis + a wall time. */
internal fun toRfc3339(dateMillis: Long, hour: Int, minute: Int): String {
    val date = Instant.ofEpochMilli(dateMillis).atZone(ZoneOffset.UTC).toLocalDate()
    return date.atTime(hour, minute).toInstant(ZoneOffset.UTC).toString()
}
