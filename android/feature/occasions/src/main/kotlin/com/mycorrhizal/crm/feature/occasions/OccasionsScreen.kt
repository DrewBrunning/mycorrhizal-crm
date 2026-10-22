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
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import com.mycorrhizal.crm.ui.components.BrandFab
import com.mycorrhizal.crm.ui.components.DiscardChangesDialog
import com.mycorrhizal.crm.ui.components.FabListContentPadding
import com.mycorrhizal.crm.ui.components.EmptyState
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
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.OccasionEvent
import com.mycorrhizal.crm.model.registry.OccasionSensitivity
import com.mycorrhizal.crm.model.util.DateFormat
import com.mycorrhizal.crm.ui.LocalDateFormat
import com.mycorrhizal.crm.ui.LocalIs24Hour
import com.mycorrhizal.crm.ui.R
import java.time.Instant
import java.time.ZoneId
import java.time.ZoneOffset
import androidx.compose.material.icons.outlined.Menu
import java.util.Locale

/**
 * The Occasions events list (docs/adrs/0026-occasions-events.md, issue #1228):
 * create/edit/delete a hosted event, and open its attendee/RSVP ledger.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun OccasionsScreen(
    // Issue #1758: a top-level drawer destination; null hides the hamburger (no drawer at Expanded).
    onMenuClick: (() -> Unit)? = {},
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
                    // Issue #1758: a top-level drawer destination opens the drawer (no Back arrow);
                    // null at Expanded width where the rail replaces it.
                    onMenuClick?.let { onMenu ->
                        IconButton(onClick = onMenu) {
                            Icon(Icons.Outlined.Menu, contentDescription = stringResource(R.string.cd_menu))
                        }
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
            BrandFab(onClick = { editing = null; showDialog = true }) {
                Icon(Icons.Outlined.Add, contentDescription = stringResource(R.string.occasions_add_event))
            }
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        Box(Modifier.fillMaxSize().padding(padding)) {
            when {
                state.errorRes != null -> CenteredMessage(stringResource(state.errorRes!!))
                state.isLoading && state.events.isEmpty() -> CenteredMessage(stringResource(R.string.a11y_state_loading))
                state.events.isEmpty() -> EmptyState(
                    title = stringResource(R.string.occasions_empty),
                    body = stringResource(R.string.occasions_empty_body),
                    actionLabel = stringResource(R.string.occasions_add_event),
                    onAction = { editing = null; showDialog = true },
                )
                else -> LazyColumn(
                    modifier = Modifier.fillMaxSize().padding(horizontal = 16.dp),
                    contentPadding = FabListContentPadding,
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
        Text(formatEventWhen(event, LocalDateFormat.current, LocalIs24Hour.current), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
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
    // Resolved in composition (not via the Context inside onClick) so a
    // configuration change -- e.g. a locale switch -- re-resolves them.
    val titleRequiredMsg = stringResource(R.string.occasions_validation_title_required)
    val startRequiredMsg = stringResource(R.string.occasions_validation_start_required)
    val endBeforeStartMsg = stringResource(R.string.occasions_validation_end_before_start)
    var title by remember(event) { mutableStateOf(event?.title ?: "") }
    var startsAt by remember(event) { mutableStateOf(event?.startsAt ?: "") }
    var endsAt by remember(event) { mutableStateOf(event?.endsAt ?: "") }
    var location by remember(event) { mutableStateOf(event?.location ?: "") }
    var sensitivity by remember(event) { mutableStateOf(event?.sensitivity ?: OccasionSensitivity.NORMAL) }
    var notes by remember(event) { mutableStateOf(event?.notes ?: "") }
    var error by remember { mutableStateOf<String?>(null) }
    var showDiscardDialog by rememberSaveable { mutableStateOf(false) }
    val isDirty = title != (event?.title ?: "") ||
        startsAt != (event?.startsAt ?: "") ||
        endsAt != (event?.endsAt ?: "") ||
        location != (event?.location ?: "") ||
        sensitivity != (event?.sensitivity ?: OccasionSensitivity.NORMAL) ||
        notes != (event?.notes ?: "")
    val requestDismiss = { if (isDirty) showDiscardDialog = true else onDismiss() }

    AlertDialog(
        onDismissRequest = requestDismiss,
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
                    trimmed.isEmpty() -> error = titleRequiredMsg
                    startsAt.isBlank() -> error = startRequiredMsg
                    isEndBeforeStart(startsAt, endsAt) -> error = endBeforeStartMsg
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
        dismissButton = { TextButton(onClick = requestDismiss) { Text(stringResource(R.string.action_cancel)) } },
    )

    if (showDiscardDialog) {
        DiscardChangesDialog(
            entityLabel = stringResource(R.string.entity_occasion),
            onKeepEditing = { showDiscardDialog = false },
            onDiscard = {
                showDiscardDialog = false
                onDismiss()
            },
        )
    }
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
        value = instantToDisplay(value, LocalDateFormat.current, LocalIs24Hour.current),
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
    val initialLocal = remember(initialInstant) { pickerInitial(initialInstant) }
    val is24Hour = LocalIs24Hour.current
    var step by remember { mutableStateOf(0) }
    val dateState = rememberDatePickerState(initialSelectedDateMillis = initialLocal.dateMillis)
    val timeState = rememberTimePickerState(
        initialHour = initialLocal.hour,
        initialMinute = initialLocal.minute,
        is24Hour = is24Hour,
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
                    val millis = dateState.selectedDateMillis ?: initialLocal.dateMillis
                    onConfirm(toRfc3339(millis, timeState.hour, timeState.minute))
                }) { Text(stringResource(R.string.action_confirm)) }
            },
            dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) } },
        )
    }
}

/**
 * Renders an RFC 3339 instant in [zone] (the device zone by default) as the
 * user's preferred date plus a locale-short time, e.g. `26.10.2026 13:06`.
 * Occasions are timed events, so (unlike calendar days, ADR 0015 rule 2) they
 * are real instants and shift to local time (#1753). Blank input renders blank
 * and an unparseable value renders as-is rather than throwing.
 */
internal fun instantToDisplay(
    iso: String,
    dateFormat: String = DateFormat.EU,
    is24Hour: Boolean = true,
    zone: ZoneId = ZoneId.systemDefault(),
    locale: Locale = Locale.getDefault(),
): String {
    if (iso.isBlank()) return ""
    val instant = runCatching { Instant.parse(iso) }.getOrNull() ?: return iso
    val local = instant.atZone(zone)
    val date = DateFormat.formatFull(local.year, local.monthValue, local.dayOfMonth, dateFormat, locale)
    return "$date ${DateFormat.formatTime(iso, is24Hour, zone, locale)}"
}

/**
 * The start (and optional end) of an event, for the list row. Same ranges as
 * web (#1789):
 *  - no end, or end == start (the same instant): just the start;
 *  - ends the same local day: `<date> <start> – <end time>`;
 *  - otherwise: the full start and the full end.
 */
internal fun formatEventWhen(
    event: OccasionEvent,
    dateFormat: String = DateFormat.EU,
    is24Hour: Boolean = true,
    zone: ZoneId = ZoneId.systemDefault(),
    locale: Locale = Locale.getDefault(),
): String {
    val start = instantToDisplay(event.startsAt, dateFormat, is24Hour, zone, locale)
    val endIso = event.endsAt?.takeIf { it.isNotBlank() } ?: return start
    val full = "$start – ${instantToDisplay(endIso, dateFormat, is24Hour, zone, locale)}"
    val startInstant = runCatching { Instant.parse(event.startsAt) }.getOrNull() ?: return full
    val endInstant = runCatching { Instant.parse(endIso) }.getOrNull() ?: return full
    if (endInstant == startInstant) return start
    val sameDay = startInstant.atZone(zone).toLocalDate() == endInstant.atZone(zone).toLocalDate()
    return if (sameDay) "$start – ${DateFormat.formatTime(endIso, is24Hour, zone, locale)}" else full
}

/** True when both are parseable instants and [endsAt] is strictly before [startsAt]. */
internal fun isEndBeforeStart(startsAt: String, endsAt: String): Boolean {
    if (endsAt.isBlank()) return false
    val start = runCatching { Instant.parse(startsAt) }.getOrNull() ?: return false
    val end = runCatching { Instant.parse(endsAt) }.getOrNull() ?: return false
    return end.isBefore(start)
}

/** The picker's initial selection: the local date as the picker's UTC-midnight millis, and the local wall time. */
internal data class PickerInitial(val dateMillis: Long, val hour: Int, val minute: Int)

internal fun pickerInitial(instant: Instant, zone: ZoneId = ZoneId.systemDefault()): PickerInitial {
    val local = instant.atZone(zone)
    return PickerInitial(
        // Material's DatePicker speaks UTC midnight of the chosen calendar date.
        dateMillis = local.toLocalDate().atStartOfDay(ZoneOffset.UTC).toInstant().toEpochMilli(),
        hour = local.hour,
        minute = local.minute,
    )
}

/**
 * Builds an RFC 3339 UTC instant from a date picker's UTC-midnight millis (the
 * chosen calendar date) plus a wall time the user picked **in [zone]** (the
 * device zone by default). The inverse of [pickerInitial] / [instantToDisplay]:
 * an event created at 18:00 local shows 18:00 on the same device.
 */
internal fun toRfc3339(
    dateMillis: Long,
    hour: Int,
    minute: Int,
    zone: ZoneId = ZoneId.systemDefault(),
): String {
    val date = Instant.ofEpochMilli(dateMillis).atZone(ZoneOffset.UTC).toLocalDate()
    return date.atTime(hour, minute).atZone(zone).toInstant().toString()
}
