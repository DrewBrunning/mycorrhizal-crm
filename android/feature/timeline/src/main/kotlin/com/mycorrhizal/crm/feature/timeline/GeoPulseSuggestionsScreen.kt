package com.mycorrhizal.crm.feature.timeline

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.CalendarToday
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DatePicker
import androidx.compose.material3.DatePickerDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.material3.rememberDatePickerState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LifecycleEventEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.GeoPulseStaySuggestion
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.AccessibleIconButton
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import java.time.ZoneOffset

/**
 * Issue #160 (ADR 0033): "Log from location history". [onLogStay] opens the activity form
 * pre-filled from the stay (location, the stay's own local date, external_ref); the user picks the
 * contacts there. Reached from the Activities screen on non-embedded profiles only.
 */
@Composable
fun GeoPulseSuggestionsScreen(
    onBack: () -> Unit,
    onLogStay: (GeoPulseStaySuggestion, prefillDate: String) -> Unit,
    onOpenSettings: () -> Unit,
    viewModel: GeoPulseSuggestionsViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    // Coming back from the form: re-run the last lookup so the stay now reads "Already logged".
    LifecycleEventEffect(Lifecycle.Event.ON_RESUME) { viewModel.refreshIfLoaded() }
    GeoPulseSuggestionsContent(
        state = state,
        onBack = onBack,
        onDateChange = viewModel::onDateChange,
        onLookup = viewModel::lookup,
        onLogStay = onLogStay,
        onOpenSettings = onOpenSettings,
    )
}

/** Stateless body — testable without a Hilt-backed ViewModel. */
@OptIn(ExperimentalMaterial3Api::class, ExperimentalLayoutApi::class)
@Composable
fun GeoPulseSuggestionsContent(
    state: GeoPulseSuggestionsUiState,
    onBack: () -> Unit = {},
    onDateChange: (String) -> Unit = {},
    onLookup: () -> Unit = {},
    onLogStay: (GeoPulseStaySuggestion, prefillDate: String) -> Unit = { _, _ -> },
    onOpenSettings: () -> Unit = {},
) {
    var showDatePicker by remember { mutableStateOf(false) }
    val zone = remember(state.timezone) { runCatching { ZoneId.of(state.timezone) }.getOrDefault(ZoneId.systemDefault()) }
    val unnamedPlace = stringResource(R.string.geopulse_unnamed_place)

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    AccessibleIconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
                title = { Text(stringResource(R.string.geopulse_title), style = MaterialTheme.typography.titleLarge) },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary,
                    actionIconContentColor = MaterialTheme.colorScheme.onPrimary,
                ),
            )
        },
    ) { padding ->
        LazyColumn(
            modifier = Modifier.fillMaxSize().padding(padding).testTag("geopulse-suggestions-list"),
            contentPadding = androidx.compose.foundation.layout.PaddingValues(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            item {
                Text(
                    stringResource(R.string.geopulse_description),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            item {
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.Top) {
                    OutlinedTextField(
                        value = state.date,
                        onValueChange = {},
                        readOnly = true,
                        singleLine = true,
                        label = { Text(stringResource(R.string.geopulse_date)) },
                        trailingIcon = {
                            AccessibleIconButton(onClick = { showDatePicker = true }) {
                                Icon(
                                    Icons.Outlined.CalendarToday,
                                    contentDescription = stringResource(R.string.geopulse_pick_date),
                                )
                            }
                        },
                        modifier = Modifier.weight(1f),
                    )
                    Button(
                        onClick = onLookup,
                        enabled = state.date.isNotBlank() && !state.isLoading,
                        modifier = Modifier.heightIn(min = 56.dp),
                    ) {
                        Text(stringResource(if (state.isLoading) R.string.geopulse_looking_up else R.string.geopulse_look_up))
                    }
                }
            }

            // Polite live region: loading, the outcome count, or nothing (errors are their own
            // assertive region below).
            item {
                val suggestions = state.suggestions
                val announcement = when {
                    state.isLoading -> stringResource(R.string.geopulse_looking_up)
                    suggestions == null -> ""
                    suggestions.isEmpty() -> stringResource(R.string.geopulse_announce_none)
                    else -> stringResource(R.string.geopulse_announce_found, suggestions.size)
                }
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    modifier = Modifier.fillMaxWidth().semantics { liveRegion = LiveRegionMode.Polite },
                ) {
                    if (state.isLoading) CircularProgressIndicator(modifier = Modifier.padding(2.dp), strokeWidth = 2.dp)
                    if (announcement.isNotEmpty()) {
                        Text(announcement, style = MaterialTheme.typography.labelLarge)
                    }
                }
            }

            state.error?.let { error ->
                item {
                    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                        Text(
                            error,
                            color = MaterialTheme.colorScheme.error,
                            style = MaterialTheme.typography.bodyMedium,
                            modifier = Modifier.semantics { liveRegion = LiveRegionMode.Assertive },
                        )
                        OutlinedButton(onClick = onOpenSettings) {
                            Text(stringResource(R.string.geopulse_open_settings))
                        }
                    }
                }
            }

            val suggestions = state.suggestions
            if (suggestions != null && suggestions.isEmpty()) {
                item { Text(stringResource(R.string.geopulse_no_stays), style = MaterialTheme.typography.bodyMedium) }
            }
            items(suggestions.orEmpty(), key = { it.externalRef.ifBlank { it.stayId.toString() } }) { stay ->
                val place = stay.location.ifBlank { unnamedPlace }
                val time = formatStayTime(stay.timestamp, zone)
                val area = listOf(stay.city, stay.country).filter { it.isNotBlank() }.joinToString(", ")
                val duration = durationText(stay.durationSeconds)
                val logged = stay.existingActivityId != null
                val logLabel = stringResource(R.string.geopulse_log_activity_at, place, time)
                Card(modifier = Modifier.fillMaxWidth().testTag("geopulse-stay-${stay.stayId}")) {
                    Column(modifier = Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        Row(verticalAlignment = Alignment.Top, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            Column(modifier = Modifier.weight(1f)) {
                                Text(
                                    place,
                                    style = MaterialTheme.typography.titleSmall,
                                    modifier = Modifier.semantics { heading() },
                                )
                                Text(
                                    listOf(time, duration, area).filter { it.isNotBlank() }.joinToString(" · "),
                                    style = MaterialTheme.typography.bodyMedium,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                )
                            }
                            if (logged) {
                                Text(
                                    stringResource(R.string.geopulse_already_logged),
                                    style = MaterialTheme.typography.labelLarge,
                                    color = MaterialTheme.colorScheme.tertiary,
                                )
                            } else {
                                OutlinedButton(
                                    onClick = { onLogStay(stay, stayPrefillDate(stay.timestamp, zone, state.date)) },
                                    modifier = Modifier
                                        .heightIn(min = 48.dp)
                                        .semantics { contentDescription = logLabel },
                                ) {
                                    Text(stringResource(R.string.geopulse_log_activity))
                                }
                            }
                        }
                        if (stay.photos.isNotEmpty()) {
                            Text(
                                stringResource(R.string.geopulse_photos, stay.photos.size),
                                style = MaterialTheme.typography.labelMedium,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                            FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                                stay.photos.forEach { photo ->
                                    Surface(
                                        shape = MaterialTheme.shapes.small,
                                        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
                                    ) {
                                        Text(
                                            photo.fileName.ifBlank { photo.id },
                                            style = MaterialTheme.typography.labelMedium,
                                            modifier = Modifier.padding(horizontal = 8.dp, vertical = 4.dp),
                                        )
                                    }
                                }
                            }
                        } else if (stay.photosUnavailable) {
                            Text(
                                stringResource(R.string.geopulse_photos_unavailable),
                                style = MaterialTheme.typography.labelMedium,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    }
                }
            }
        }
    }

    if (showDatePicker) {
        val initialMillis = runCatching { LocalDate.parse(state.date).atStartOfDay(ZoneOffset.UTC).toInstant().toEpochMilli() }.getOrNull()
        val pickerState = rememberDatePickerState(initialSelectedDateMillis = initialMillis)
        DatePickerDialog(
            onDismissRequest = { showDatePicker = false },
            confirmButton = {
                TextButton(onClick = {
                    pickerState.selectedDateMillis?.let { millis ->
                        onDateChange(Instant.ofEpochMilli(millis).atZone(ZoneOffset.UTC).toLocalDate().toString())
                    }
                    showDatePicker = false
                }) { Text(stringResource(R.string.action_confirm)) }
            },
            dismissButton = {
                TextButton(onClick = { showDatePicker = false }) { Text(stringResource(R.string.action_cancel)) }
            },
        ) {
            DatePicker(state = pickerState, showModeToggle = false)
        }
    }
}

@Composable
private fun durationText(seconds: Long): String {
    val minutes = maxOf(1L, Math.round(seconds / 60.0)).toInt()
    if (minutes < 60) return stringResource(R.string.geopulse_duration_minutes, minutes)
    val hours = minutes / 60
    val rest = minutes % 60
    return if (rest == 0) {
        stringResource(R.string.geopulse_duration_hours, hours)
    } else {
        stringResource(R.string.geopulse_duration_hours_minutes, hours, rest)
    }
}
