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
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
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
import com.mycorrhizal.crm.model.network.ErrorBucket
import com.mycorrhizal.crm.model.network.JobRunHealth
import com.mycorrhizal.crm.model.network.JobRunStatuses
import com.mycorrhizal.crm.model.network.SubsystemHealth
import com.mycorrhizal.crm.model.network.Subsystems
import com.mycorrhizal.crm.model.network.SubsystemStatuses
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
        state.eventType != null || correlationInput.isNotBlank() || state.eventIds.isNotEmpty()

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

            SubsystemHealthSection(
                health = state.subsystemHealth,
                onSelectSubsystem = viewModel::applyComponent,
                onRefresh = viewModel::refreshSubsystemHealth,
            )

            BackgroundJobsSection(
                jobs = state.jobRunHealth,
                onRefresh = viewModel::refreshJobRunHealth,
            )

            ErrorAggregationSection(
                buckets = state.errorBuckets,
                onViewEvents = viewModel::applyEventIds,
                onRefresh = viewModel::refreshErrorAggregation,
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

            if (state.eventIds.isNotEmpty()) {
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 16.dp, vertical = 4.dp)
                        .testTag("sysevents-errors-banner"),
                    horizontalArrangement = Arrangement.spacedBy(6.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Icon(
                        Icons.Outlined.Hub,
                        contentDescription = null,
                        tint = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    Text(
                        text = stringResource(R.string.sysevents_errors_banner, state.eventIds.size),
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

/**
 * Per-subsystem last-known-good state (issue #427), above the timeline: a
 * horizontal strip of cards, one per subsystem, each tappable to filter the
 * timeline below to that component. Hidden until the first fetch lands.
 */
@Composable
internal fun SubsystemHealthSection(
    health: List<SubsystemHealth>,
    onSelectSubsystem: (String) -> Unit,
    onRefresh: () -> Unit,
) {
    if (health.isEmpty()) return
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp, vertical = 4.dp)
            .testTag("sysevents-subsystem-health"),
        verticalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                text = stringResource(R.string.subsystem_health_title),
                style = MaterialTheme.typography.titleSmall,
            )
            TextButton(
                onClick = onRefresh,
                modifier = Modifier.testTag("subsystem-health-refresh"),
            ) {
                Text(stringResource(R.string.subsystem_health_refresh))
            }
        }
        LazyRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            items(health, key = { it.subsystem }) { item ->
                SubsystemHealthCard(item = item, onClick = { onSelectSubsystem(item.subsystem) })
            }
        }
    }
}

@Composable
private fun SubsystemHealthCard(item: SubsystemHealth, onClick: () -> Unit) {
    val failing = item.status == SubsystemStatuses.FAILING
    val statusColor = when (item.status) {
        SubsystemStatuses.HEALTHY -> MaterialTheme.colorScheme.primary
        SubsystemStatuses.FAILING -> MaterialTheme.colorScheme.error
        else -> MaterialTheme.colorScheme.onSurfaceVariant
    }
    Card(
        modifier = Modifier
            .width(220.dp)
            .testTag("subsystem-health-card-${item.subsystem}"),
        onClick = onClick,
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.surfaceContainerLow,
        ),
    ) {
        Column(
            modifier = Modifier.padding(12.dp),
            verticalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            Text(
                text = subsystemLabel(item.subsystem),
                style = MaterialTheme.typography.labelLarge,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            Text(
                text = subsystemStatusLabel(item.status),
                style = MaterialTheme.typography.labelMedium,
                color = statusColor,
            )
            if (failing) {
                Text(
                    text = stringResource(
                        R.string.subsystem_health_consecutive_failures,
                        item.consecutiveFailures,
                    ),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.error,
                )
                val incidentStart = item.incidentFirstFailureAt
                if (!incidentStart.isNullOrBlank()) {
                    Text(
                        text = stringResource(
                            R.string.subsystem_health_incident_since,
                            formatEventTime(incidentStart),
                        ),
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                if (item.lastError.isNotBlank()) {
                    Text(
                        text = item.lastError,
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.error,
                        maxLines = 2,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
            } else {
                Text(
                    text = stringResource(
                        R.string.subsystem_health_last_success,
                        item.lastSuccessAt?.let { formatEventTime(it) }
                            ?: stringResource(R.string.subsystem_health_never),
                    ),
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

@Composable
internal fun subsystemLabel(token: String): String = when (token) {
    Subsystems.CONTACT_SYNC -> stringResource(R.string.subsystem_name_contact_sync)
    Subsystems.CALENDAR_SYNC -> stringResource(R.string.subsystem_name_calendar_sync)
    Subsystems.NOTIFICATION -> stringResource(R.string.subsystem_name_notification)
    Subsystems.BACKUP -> stringResource(R.string.subsystem_name_backup)
    Subsystems.SCHEDULER -> stringResource(R.string.subsystem_name_scheduler)
    Subsystems.WEBHOOK -> stringResource(R.string.subsystem_name_webhook)
    else -> token
}

@Composable
internal fun subsystemStatusLabel(token: String): String = when (token) {
    SubsystemStatuses.HEALTHY -> stringResource(R.string.subsystem_health_status_healthy)
    SubsystemStatuses.FAILING -> stringResource(R.string.subsystem_health_status_failing)
    SubsystemStatuses.UNKNOWN -> stringResource(R.string.subsystem_health_status_unknown)
    else -> token
}

/**
 * Per-job background-job run health (issue #391), above the timeline: a
 * horizontal strip of cards, one per scheduled job, each with its status, last
 * run, duration, and — when failing — the consecutive-failure count and last
 * error. Mirrors [SubsystemHealthSection]. Hidden until the first fetch lands.
 */
@Composable
internal fun BackgroundJobsSection(
    jobs: List<JobRunHealth>,
    onRefresh: () -> Unit,
) {
    if (jobs.isEmpty()) return
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp, vertical = 4.dp)
            .testTag("sysevents-background-jobs"),
        verticalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                text = stringResource(R.string.jobruns_title),
                style = MaterialTheme.typography.titleSmall,
            )
            TextButton(
                onClick = onRefresh,
                modifier = Modifier.testTag("background-jobs-refresh"),
            ) {
                Text(stringResource(R.string.jobruns_refresh))
            }
        }
        LazyRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            items(jobs, key = { it.jobName }) { item ->
                BackgroundJobCard(item = item)
            }
        }
    }
}

@Composable
private fun BackgroundJobCard(item: JobRunHealth) {
    val failing = item.status == JobRunStatuses.FAILING
    val statusColor = when (item.status) {
        JobRunStatuses.HEALTHY -> MaterialTheme.colorScheme.primary
        JobRunStatuses.FAILING -> MaterialTheme.colorScheme.error
        else -> MaterialTheme.colorScheme.onSurfaceVariant
    }
    Card(
        modifier = Modifier
            .width(220.dp)
            .testTag("background-job-card-${item.jobName}"),
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.surfaceContainerLow,
        ),
    ) {
        Column(
            modifier = Modifier.padding(12.dp),
            verticalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            Text(
                text = jobRunLabel(item.jobName),
                style = MaterialTheme.typography.labelLarge,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            Text(
                text = jobRunStatusLabel(item.status),
                style = MaterialTheme.typography.labelMedium,
                color = statusColor,
            )
            if (failing) {
                Text(
                    text = stringResource(
                        R.string.jobruns_consecutive_failures,
                        item.consecutiveFailures,
                    ),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.error,
                )
                if (item.lastError.isNotBlank()) {
                    Text(
                        text = item.lastError,
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.error,
                        maxLines = 2,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
            }
            Text(
                text = stringResource(
                    R.string.jobruns_last_run,
                    item.lastRunAt?.let { formatEventTime(it) }
                        ?: stringResource(R.string.jobruns_never),
                ),
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            item.lastDurationMs?.let { ms ->
                Text(
                    text = stringResource(R.string.jobruns_duration, formatJobRunDuration(ms)),
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

@Composable
internal fun jobRunStatusLabel(token: String): String = when (token) {
    JobRunStatuses.HEALTHY -> stringResource(R.string.jobruns_status_healthy)
    JobRunStatuses.FAILING -> stringResource(R.string.jobruns_status_failing)
    JobRunStatuses.UNKNOWN -> stringResource(R.string.jobruns_status_unknown)
    else -> token
}

/**
 * Friendly label for a canonical job name. Hand-maintained mirror of
 * backend/models/job_run.go's KnownJobNames and web's backgroundJobs.jobs.*
 * (frontend trap #4) — an unknown token from a future job still renders as-is.
 */
@Composable
internal fun jobRunLabel(token: String): String = when (token) {
    "daily_reminders" -> stringResource(R.string.jobruns_job_daily_reminders)
    "webhook_retries" -> stringResource(R.string.jobruns_job_webhook_retries)
    "calendar_sync" -> stringResource(R.string.jobruns_job_calendar_sync)
    "purge_deleted" -> stringResource(R.string.jobruns_job_purge_deleted)
    "audit_purge" -> stringResource(R.string.jobruns_job_audit_purge)
    "system_event_purge" -> stringResource(R.string.jobruns_job_system_event_purge)
    "cadence_overdue" -> stringResource(R.string.jobruns_job_cadence_overdue)
    "reach_out_detection" -> stringResource(R.string.jobruns_job_reach_out_detection)
    "immich_sync" -> stringResource(R.string.jobruns_job_immich_sync)
    "db_integrity_check" -> stringResource(R.string.jobruns_job_db_integrity_check)
    "restore_drill" -> stringResource(R.string.jobruns_job_restore_drill)
    "alert_eval" -> stringResource(R.string.jobruns_job_alert_eval)
    "job_run_purge" -> stringResource(R.string.jobruns_job_job_run_purge)
    else -> token
}

/** Formats a millisecond duration as "820 ms" / "1.4 s". */
private fun formatJobRunDuration(ms: Long): String =
    if (ms < 1000) "$ms ms" else "%.1f s".format(ms / 1000.0)

/**
 * Operational failures over the last 24h bucketed by cause (issue #426), above
 * the timeline: a horizontal strip of cards, one per cause, each with its
 * count, the component, a sample of the raw error, and a "View N events"
 * action that filters the timeline below to exactly those system_events rows.
 * Hidden until the first fetch lands.
 */
@Composable
internal fun ErrorAggregationSection(
    buckets: List<ErrorBucket>,
    onViewEvents: (List<Long>) -> Unit,
    onRefresh: () -> Unit,
) {
    if (buckets.isEmpty()) return
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp, vertical = 4.dp)
            .testTag("sysevents-error-aggregation"),
        verticalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                text = stringResource(R.string.error_aggregation_title),
                style = MaterialTheme.typography.titleSmall,
            )
            TextButton(
                onClick = onRefresh,
                modifier = Modifier.testTag("error-aggregation-refresh"),
            ) {
                Text(stringResource(R.string.error_aggregation_refresh))
            }
        }
        LazyRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            items(buckets, key = { "${it.component}${it.cause}" }) { bucket ->
                ErrorBucketCard(bucket = bucket, onViewEvents = { onViewEvents(bucket.eventIds) })
            }
        }
    }
}

@Composable
private fun ErrorBucketCard(bucket: ErrorBucket, onViewEvents: () -> Unit) {
    Card(
        modifier = Modifier
            .width(240.dp)
            .testTag("error-bucket-card-${bucket.component}"),
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.surfaceContainerLow,
        ),
    ) {
        Column(
            modifier = Modifier.padding(12.dp),
            verticalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                Text(
                    text = bucket.count.toString(),
                    style = MaterialTheme.typography.titleMedium,
                    color = if (bucket.recurring) {
                        MaterialTheme.colorScheme.error
                    } else {
                        MaterialTheme.colorScheme.onSurface
                    },
                )
                Text(
                    text = subsystemLabel(bucket.component),
                    style = MaterialTheme.typography.labelLarge,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
            if (bucket.recurring) {
                Text(
                    text = stringResource(R.string.error_aggregation_recurring),
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.error,
                )
            }
            Text(
                text = bucket.sampleError.ifBlank { bucket.cause },
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.error,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
            )
            TextButton(
                onClick = onViewEvents,
                enabled = bucket.eventIds.isNotEmpty(),
                modifier = Modifier.testTag("error-bucket-view-${bucket.component}"),
            ) {
                Text(stringResource(R.string.error_aggregation_view_events, bucket.eventIds.size))
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
