package com.mycorrhizal.crm.feature.sysevents

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Clear
import androidx.compose.material.icons.outlined.Hub
import androidx.compose.material.icons.outlined.MonitorHeart
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExposedDropdownMenuBox
import androidx.compose.material3.ExposedDropdownMenuDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.MenuAnchorType
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.SystemEvent
import com.mycorrhizal.crm.model.network.SystemEventComponents
import com.mycorrhizal.crm.model.network.SystemEventSeverities
import com.mycorrhizal.crm.model.network.SystemEventTypes
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.AccessibleIconButton
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/**
 * The operational-event timeline (issue #424, mirroring web's
 * SystemEventsPage over `GET /admin/system-events`). Admin-only, read-only.
 * Filter by component / severity / event type, then drill from any event into
 * every other event sharing its correlation ID ("view related").
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SystemEventsScreen(
    onBack: () -> Unit,
    viewModel: SystemEventsViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }
    val loadError = state.error

    var correlationInput by rememberSaveable { mutableStateOf(state.correlationId) }
    // Keep the field in sync when "view related" sets the filter programmatically.
    LaunchedEffect(state.correlationId) {
        if (correlationInput != state.correlationId) correlationInput = state.correlationId
    }

    var selected by remember { mutableStateOf<SystemEvent?>(null) }

    val hasFilters = state.component != null || state.severity != null ||
        state.eventType != null || correlationInput.isNotBlank()

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    AccessibleIconButton(onClick = onBack) {
                        Icon(
                            Icons.AutoMirrored.Outlined.ArrowBack,
                            contentDescription = stringResource(R.string.cd_back),
                        )
                    }
                },
                title = {
                    Text(
                        stringResource(R.string.sysevents_title),
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
        Column(modifier = Modifier.fillMaxSize().padding(padding)) {
            Text(
                text = stringResource(R.string.sysevents_description),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(horizontal = 16.dp, vertical = 4.dp),
            )

            SystemEventsFilterToolbar(
                component = state.component,
                severity = state.severity,
                eventType = state.eventType,
                correlationInput = correlationInput,
                hasFilters = hasFilters,
                onComponentChange = viewModel::applyComponent,
                onSeverityChange = viewModel::applySeverity,
                onEventTypeChange = viewModel::applyEventType,
                onCorrelationChange = {
                    correlationInput = it
                    viewModel.onCorrelationIdChange(it)
                },
                onClearFilters = {
                    correlationInput = ""
                    viewModel.clearFilters()
                },
            )

            if (state.correlationId.isNotBlank()) {
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 16.dp, vertical = 4.dp)
                        .testTag("sysevents-related-banner"),
                    horizontalArrangement = Arrangement.spacedBy(6.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Icon(
                        Icons.Outlined.Hub,
                        contentDescription = null,
                        tint = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    Text(
                        text = stringResource(R.string.sysevents_related_banner, state.correlationId),
                        style = MaterialTheme.typography.bodySmall,
                    )
                }
            }

            Box(modifier = Modifier.fillMaxSize()) {
                when {
                    state.isLoading && state.events.isEmpty() ->
                        LoadingSkeleton(modifier = Modifier.testTag("sysevents-loading"))

                    state.events.isEmpty() && loadError != null ->
                        Box(modifier = Modifier.fillMaxWidth().padding(16.dp)) {
                            Text(
                                text = loadError,
                                color = MaterialTheme.colorScheme.error,
                                modifier = Modifier.align(Alignment.Center),
                            )
                        }

                    state.events.isEmpty() ->
                        EmptyState(
                            message = stringResource(
                                if (hasFilters) R.string.sysevents_empty
                                else R.string.sysevents_empty_no_filters
                            ),
                            icon = {
                                Icon(
                                    Icons.Outlined.MonitorHeart,
                                    contentDescription = null,
                                    tint = MaterialTheme.colorScheme.onSurfaceVariant,
                                )
                            },
                        )

                    else -> SystemEventList(
                        events = state.events,
                        canLoadMore = state.canLoadMore,
                        isLoadingMore = state.isLoading,
                        onLoadMore = viewModel::loadMore,
                        onRowClick = { selected = it },
                    )
                }
            }
        }
    }

    if (loadError != null && state.events.isNotEmpty()) {
        LaunchedEffect(loadError) {
            snackbarHostState.showSnackbar(loadError)
            viewModel.onErrorShown()
        }
    }

    val detail = selected
    if (detail != null) {
        SystemEventDetailDialog(
            event = detail,
            onDismiss = { selected = null },
            onViewRelated = {
                selected = null
                correlationInput = detail.correlationId
                viewModel.showRelated(detail.correlationId)
            },
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class, ExperimentalLayoutApi::class)
@Composable
internal fun SystemEventsFilterToolbar(
    component: String?,
    severity: String?,
    eventType: String?,
    correlationInput: String,
    hasFilters: Boolean,
    onComponentChange: (String?) -> Unit,
    onSeverityChange: (String?) -> Unit,
    onEventTypeChange: (String?) -> Unit,
    onCorrelationChange: (String) -> Unit,
    onClearFilters: () -> Unit,
) {
    Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp)) {
        FlowRow(
            horizontalArrangement = Arrangement.spacedBy(12.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            FilterDropdown(
                label = stringResource(R.string.sysevents_filter_component),
                selectedLabel = component ?: stringResource(R.string.sysevents_filter_all),
                options = SystemEventComponents.ALL,
                optionLabel = { it },
                onSelect = onComponentChange,
                testTag = "sysevents-component",
            )
            FilterDropdown(
                label = stringResource(R.string.sysevents_filter_severity),
                selectedLabel = severity?.let { severityLabel(it) }
                    ?: stringResource(R.string.sysevents_filter_all),
                options = SystemEventSeverities.ALL,
                optionLabel = { severityLabel(it) },
                onSelect = onSeverityChange,
                testTag = "sysevents-severity",
            )
            FilterDropdown(
                label = stringResource(R.string.sysevents_filter_event_type),
                selectedLabel = eventType?.let { eventTypeLabel(it) }
                    ?: stringResource(R.string.sysevents_filter_all),
                options = SystemEventTypes.ALL,
                optionLabel = { eventTypeLabel(it) },
                onSelect = onEventTypeChange,
                testTag = "sysevents-event-type",
            )
            OutlinedButton(
                onClick = onClearFilters,
                enabled = hasFilters,
                modifier = Modifier
                    .testTag("sysevents-clear-filters")
                    .align(Alignment.CenterVertically),
            ) {
                Icon(Icons.Outlined.Clear, contentDescription = null)
                Text(
                    stringResource(R.string.sysevents_filter_clear),
                    modifier = Modifier.padding(start = 4.dp),
                )
            }
        }
        OutlinedTextField(
            value = correlationInput,
            onValueChange = onCorrelationChange,
            label = { Text(stringResource(R.string.sysevents_filter_correlation_id)) },
            singleLine = true,
            modifier = Modifier
                .fillMaxWidth()
                .padding(top = 8.dp)
                .testTag("sysevents-correlation-id"),
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun FilterDropdown(
    label: String,
    selectedLabel: String,
    options: List<String>,
    optionLabel: @Composable (String) -> String,
    onSelect: (String?) -> Unit,
    testTag: String,
) {
    var expanded by remember { mutableStateOf(false) }
    ExposedDropdownMenuBox(
        expanded = expanded,
        onExpandedChange = { expanded = it },
    ) {
        OutlinedTextField(
            value = selectedLabel,
            onValueChange = {},
            readOnly = true,
            label = { Text(label) },
            trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded = expanded) },
            modifier = Modifier
                .menuAnchor(MenuAnchorType.PrimaryNotEditable)
                .testTag(testTag),
        )
        ExposedDropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
            DropdownMenuItem(
                text = { Text(stringResource(R.string.sysevents_filter_all)) },
                onClick = {
                    onSelect(null)
                    expanded = false
                },
            )
            options.forEach { token ->
                DropdownMenuItem(
                    text = { Text(optionLabel(token)) },
                    onClick = {
                        onSelect(token)
                        expanded = false
                    },
                )
            }
        }
    }
}

@Composable
internal fun SystemEventList(
    events: List<SystemEvent>,
    canLoadMore: Boolean,
    isLoadingMore: Boolean,
    onLoadMore: () -> Unit,
    onRowClick: (SystemEvent) -> Unit,
) {
    LazyColumn(
        modifier = Modifier.fillMaxSize().testTag("sysevents-list"),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        items(events, key = { it.id }) { event ->
            SystemEventRow(event = event, onClick = { onRowClick(event) })
        }
        if (canLoadMore) {
            item(key = "load-more") {
                Box(
                    modifier = Modifier.fillMaxWidth().padding(top = 4.dp),
                    contentAlignment = Alignment.Center,
                ) {
                    OutlinedButton(
                        onClick = onLoadMore,
                        enabled = !isLoadingMore,
                        modifier = Modifier.testTag("sysevents-load-more"),
                    ) {
                        Text(stringResource(R.string.action_load_more))
                    }
                }
            }
        }
    }
}

@Composable
internal fun SystemEventRow(event: SystemEvent, onClick: () -> Unit) {
    Card(
        modifier = Modifier.fillMaxWidth().testTag("sysevents-row-${event.id}"),
        onClick = onClick,
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.surfaceContainerLow,
        ),
    ) {
        Column(
            modifier = Modifier.padding(12.dp),
            verticalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            Row(
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                SeverityBadge(event.severity)
                Text(
                    text = eventTypeLabel(event.eventType),
                    style = MaterialTheme.typography.labelMedium,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f),
                )
                Text(
                    text = formatEventTime(event.occurredAt),
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            val resultText = event.result?.let { resultLabel(it) }
            Text(
                text = buildString {
                    append(event.component.ifBlank { "—" })
                    val op = event.operation
                    if (!op.isNullOrBlank()) append(" · ").append(op)
                    if (resultText != null) append(" · ").append(resultText)
                },
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
    }
}

@Composable
private fun SeverityBadge(severity: String) {
    val color = when (severity) {
        SystemEventSeverities.ERROR -> MaterialTheme.colorScheme.error
        SystemEventSeverities.WARN -> MaterialTheme.colorScheme.tertiary
        else -> MaterialTheme.colorScheme.onSurfaceVariant
    }
    Text(
        text = severityLabel(severity),
        style = MaterialTheme.typography.labelSmall,
        color = MaterialTheme.colorScheme.onSurface,
        modifier = Modifier
            .background(color.copy(alpha = 0.18f), MaterialTheme.shapes.small)
            .border(1.dp, color, MaterialTheme.shapes.small)
            .padding(horizontal = 6.dp, vertical = 2.dp),
    )
}

@Composable
private fun SystemEventDetailDialog(
    event: SystemEvent,
    onDismiss: () -> Unit,
    onViewRelated: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.sysevents_details_title)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                DetailLine(stringResource(R.string.sysevents_col_event), eventTypeLabel(event.eventType))
                DetailLine(stringResource(R.string.sysevents_col_time), formatEventTime(event.occurredAt))
                DetailLine(stringResource(R.string.sysevents_col_severity), severityLabel(event.severity))
                DetailLine(stringResource(R.string.sysevents_col_component), event.component.ifBlank { "—" })
                DetailLine(stringResource(R.string.sysevents_col_operation), event.operation ?: "—")
                DetailLine(
                    stringResource(R.string.sysevents_col_result),
                    event.result?.let { resultLabel(it) } ?: "—",
                )
                DetailLine(
                    stringResource(R.string.sysevents_col_duration),
                    event.durationMs?.let { "$it ms" } ?: "—",
                )
                Text(
                    text = stringResource(R.string.sysevents_details_correlation_id),
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Text(
                    text = event.correlationId.ifBlank { "—" },
                    style = MaterialTheme.typography.bodySmall,
                    fontFamily = FontFamily.Monospace,
                )
                val detailText = event.detail
                if (!detailText.isNullOrBlank()) {
                    DetailLine(stringResource(R.string.sysevents_details_detail), detailText)
                }
                val errorText = event.error
                if (!errorText.isNullOrBlank()) {
                    Text(
                        text = stringResource(R.string.sysevents_details_error),
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    Text(
                        text = errorText,
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.error,
                    )
                }
            }
        },
        confirmButton = {
            TextButton(
                onClick = onViewRelated,
                enabled = event.correlationId.isNotBlank(),
                modifier = Modifier.testTag("sysevents-view-related"),
            ) {
                Text(stringResource(R.string.sysevents_details_view_related))
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_close)) }
        },
    )
}

@Composable
private fun DetailLine(label: String, value: String) {
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        Text(
            text = label,
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Text(text = value, style = MaterialTheme.typography.bodySmall)
    }
}

@Composable
internal fun severityLabel(token: String): String = when (token) {
    SystemEventSeverities.INFO -> stringResource(R.string.sysevents_severity_info)
    SystemEventSeverities.WARN -> stringResource(R.string.sysevents_severity_warn)
    SystemEventSeverities.ERROR -> stringResource(R.string.sysevents_severity_error)
    else -> token
}

@Composable
internal fun resultLabel(token: String): String = when (token) {
    "success" -> stringResource(R.string.sysevents_result_success)
    "failure" -> stringResource(R.string.sysevents_result_failure)
    "skipped" -> stringResource(R.string.sysevents_result_skipped)
    else -> token
}

/** Human label for an event-type token, falling back to the raw token for future ones. */
@Composable
internal fun eventTypeLabel(token: String): String = when (token) {
    SystemEventTypes.APPLICATION_STARTED -> stringResource(R.string.sysevents_type_application_started)
    SystemEventTypes.APPLICATION_STOPPED -> stringResource(R.string.sysevents_type_application_stopped)
    SystemEventTypes.MIGRATION_STARTED -> stringResource(R.string.sysevents_type_migration_started)
    SystemEventTypes.MIGRATION_COMPLETED -> stringResource(R.string.sysevents_type_migration_completed)
    SystemEventTypes.MIGRATION_FAILED -> stringResource(R.string.sysevents_type_migration_failed)
    SystemEventTypes.JOB_STARTED -> stringResource(R.string.sysevents_type_job_started)
    SystemEventTypes.JOB_COMPLETED -> stringResource(R.string.sysevents_type_job_completed)
    SystemEventTypes.JOB_FAILED -> stringResource(R.string.sysevents_type_job_failed)
    SystemEventTypes.SYNC_STARTED -> stringResource(R.string.sysevents_type_sync_started)
    SystemEventTypes.SYNC_COMPLETED -> stringResource(R.string.sysevents_type_sync_completed)
    SystemEventTypes.SYNC_FAILED -> stringResource(R.string.sysevents_type_sync_failed)
    SystemEventTypes.NOTIFICATION_SENT -> stringResource(R.string.sysevents_type_notification_sent)
    SystemEventTypes.NOTIFICATION_FAILED -> stringResource(R.string.sysevents_type_notification_failed)
    SystemEventTypes.BACKUP_COMPLETED -> stringResource(R.string.sysevents_type_backup_completed)
    SystemEventTypes.BACKUP_FAILED -> stringResource(R.string.sysevents_type_backup_failed)
    SystemEventTypes.RESTORE_TEST_COMPLETED -> stringResource(R.string.sysevents_type_restore_test_completed)
    SystemEventTypes.INTEGRATION_FAILED -> stringResource(R.string.sysevents_type_integration_failed)
    else -> token
}

private fun formatEventTime(iso: String?): String {
    if (iso.isNullOrBlank()) return ""
    return runCatching {
        val instant = Instant.parse(iso)
        DateTimeFormatter.ofLocalizedDateTime(FormatStyle.MEDIUM)
            .withZone(ZoneId.systemDefault())
            .format(instant)
    }.getOrDefault(iso)
}
