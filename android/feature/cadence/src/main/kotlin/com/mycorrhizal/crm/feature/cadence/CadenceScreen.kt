package com.mycorrhizal.crm.feature.cadence

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.CheckCircle
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material.icons.outlined.Schedule
import androidx.compose.material.icons.outlined.Warning
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Checkbox
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.CadencePolicy
import com.mycorrhizal.crm.model.registry.CadenceQualifyingType
import com.mycorrhizal.crm.model.util.DateFormat
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.BrandFab
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.theme.MycorrhizalColors

/**
 * Per-contact cadence/relationship-health surface (M12, mirroring web's
 * CadencePanel + CadenceDialog). The health readout is read straight from the
 * server's computed fields, never recomputed locally — see the test cases.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun CadenceScreen(
    onBack: () -> Unit,
    viewModel: CadenceViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }
    var showDialog by remember { mutableStateOf(false) }
    var pendingDelete by remember { mutableStateOf(false) }
    // Two error kinds: `errorRes` is fatal and unrecoverable (missing contact
    // id / no VCardUID) and becomes the persistent body content; `error` is a
    // transient server failure (a failed load or write) and is toasted.
    val fatalError = state.errorRes?.let { stringResource(it) }
    val transientError = state.error

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
                title = {
                    Text(stringResource(R.string.cadence_title), style = MaterialTheme.typography.titleLarge)
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary,
                    actionIconContentColor = MaterialTheme.colorScheme.onPrimary,
                ),
            )
        },
        // A contact has at most one policy, so creating a second is a 409 —
        // the add affordance only exists in the empty (no-policy) state, like web.
        // Hidden on the fatal error state (a "no VCardUID" contact can't be created).
        floatingActionButton = {
            if (state.policy == null && !state.isLoading && fatalError == null) {
                BrandFab(onClick = { showDialog = true }) {
                    Icon(Icons.Outlined.Add, contentDescription = stringResource(R.string.cadence_add))
                }
            }
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            when {
                state.isLoading -> LoadingSkeleton()
                state.policy == null && fatalError != null ->
                    EmptyState(message = fatalError)
                state.policy == null -> CadenceEmptyState(onAdd = { showDialog = true })
                else -> CadencePanelContent(
                    policy = state.policy!!,
                    dateFormat = state.dateFormat ?: DateFormat.EU,
                    isMutating = state.isMutating,
                    onEdit = { showDialog = true },
                    onDelete = { pendingDelete = true },
                )
            }
        }
    }

    if (showDialog) {
        CadenceDialog(
            policy = state.policy,
            isMutating = state.isMutating,
            onConfirm = { intervalDays, qualifyingTypes ->
                val policy = state.policy
                if (policy == null) {
                    viewModel.create(intervalDays, qualifyingTypes)
                } else {
                    viewModel.update(policy.id, intervalDays, qualifyingTypes)
                }
                showDialog = false
            },
            onDismiss = { showDialog = false },
        )
    }

    if (pendingDelete) {
        AlertDialog(
            onDismissRequest = { pendingDelete = false },
            title = { Text(stringResource(R.string.cadence_delete_title)) },
            text = { Text(stringResource(R.string.cadence_confirm_delete)) },
            confirmButton = {
                TextButton(
                    enabled = !state.isMutating,
                    onClick = {
                        state.policy?.let { viewModel.delete(it.id) }
                        pendingDelete = false
                    },
                ) {
                    Text(stringResource(R.string.action_delete))
                }
            },
            dismissButton = {
                TextButton(onClick = { pendingDelete = false }) {
                    Text(stringResource(R.string.action_cancel))
                }
            },
        )
    }

    // Transient errors (failed loads or writes) are toasted regardless of the
    // body state; the fatal `errorRes` case is the persistent EmptyState above.
    if (transientError != null) {
        LaunchedEffect(transientError) {
            snackbarHostState.showSnackbar(transientError)
            viewModel.onErrorShown()
        }
    }
}

@Composable
private fun CadenceEmptyState(onAdd: () -> Unit) {
    Column(
        modifier = Modifier.fillMaxSize().padding(32.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text(
            text = stringResource(R.string.cadence_no_policy),
            style = MaterialTheme.typography.bodyLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            textAlign = TextAlign.Center,
        )
        OutlinedButton(onClick = onAdd, modifier = Modifier.padding(top = 16.dp)) {
            Text(stringResource(R.string.cadence_add))
        }
    }
}

/**
 * The readout surface for an existing policy. `health` is the server's
 * verdict — this renders `health.overdue_by`/`has_qualifying_interaction`
 * verbatim (mirroring web's CadencePanel), it never derives the status from
 * the local dates. [dateFormat] is the user's `date_format` preference,
 * defaulting to "eu".
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun CadencePanelContent(
    policy: CadencePolicy,
    dateFormat: String = DateFormat.EU,
    isMutating: Boolean = false,
    onEdit: () -> Unit,
    onDelete: () -> Unit,
) {
    val health = policy.health
    val hasInteraction = health?.hasQualifyingInteraction == true
    val isOverdue = health != null && health.isOverdue
    val nextDue = health?.nextDue?.let { DateFormat.formatTimestamp(it, dateFormat) }
    val lastInteraction = health?.lastInteraction?.let { DateFormat.formatTimestamp(it, dateFormat) }
    val qualifyingTypes = policy.qualifyingTypes
    val showTypes = qualifyingTypes.isNotEmpty()

    Card(
        modifier = Modifier.fillMaxWidth().padding(16.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerHighest),
    ) {
        Column(modifier = Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    text = stringResource(R.string.cadence_interval),
                    style = MaterialTheme.typography.titleSmall,
                    modifier = Modifier.weight(1f),
                )
                CadencePill(stringResource(R.string.cadence_interval_value, policy.targetIntervalDays))
            }

            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                when {
                    isOverdue -> {
                        Icon(Icons.Outlined.Warning, contentDescription = null, tint = MycorrhizalColors.chanterelle)
                        Text(
                            text = stringResource(R.string.cadence_overdue_by, health!!.overdueBy),
                            style = MaterialTheme.typography.bodyMedium,
                            // Web parity: overdue renders in the warning (chantarelle)
                            // color, not the error color — being overdue is a nudge,
                            // not a failure (matches M11's prep view cadence card).
                            color = MycorrhizalColors.chanterelle,
                        )
                    }
                    hasInteraction -> {
                        Icon(Icons.Outlined.CheckCircle, contentDescription = null, tint = MaterialTheme.colorScheme.tertiary)
                        Text(
                            text = stringResource(R.string.cadence_on_track),
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.tertiary,
                        )
                    }
                    else -> {
                        Icon(Icons.Outlined.Schedule, contentDescription = null, tint = MaterialTheme.colorScheme.onSurfaceVariant)
                        Text(
                            text = stringResource(R.string.cadence_no_interactions_yet),
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
            }

            if (hasInteraction && !nextDue.isNullOrBlank()) {
                Text(
                    text = stringResource(R.string.cadence_next_due, nextDue),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            if (hasInteraction && !lastInteraction.isNullOrBlank()) {
                Text(
                    text = stringResource(R.string.cadence_last_interaction, lastInteraction),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }

            if (showTypes) {
                FlowRow(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                    qualifyingTypes.forEach { token ->
                        CadencePill(cadenceTypeLabel(token))
                    }
                }
            }

            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.fillMaxWidth()) {
                OutlinedButton(onClick = onEdit, enabled = !isMutating, modifier = Modifier.weight(1f)) {
                    Icon(Icons.Outlined.Edit, contentDescription = null)
                    Text(stringResource(R.string.action_edit), modifier = Modifier.padding(start = 4.dp))
                }
                OutlinedButton(onClick = onDelete, enabled = !isMutating, modifier = Modifier.weight(1f)) {
                    Icon(Icons.Outlined.Delete, contentDescription = null)
                    Text(stringResource(R.string.action_delete), modifier = Modifier.padding(start = 4.dp))
                }
            }
        }
    }
}

/** A small non-interactive pill, standing in for web's display-only Chip. */
@Composable
private fun CadencePill(text: String) {
    Surface(
        shape = RoundedCornerShape(8.dp),
        color = MaterialTheme.colorScheme.surfaceVariant,
    ) {
        Text(
            text = text,
            style = MaterialTheme.typography.labelMedium,
            modifier = Modifier.padding(horizontal = 10.dp, vertical = 4.dp),
        )
    }
}

/**
 * Create/edit dialog mirroring web's CadenceDialog. Interval (days) with a
 * positive-number validation, plus a checkbox per qualifying-interaction type.
 * An empty selection is sent as empty (means "every default-qualifying type
 * counts"), never defaulted to a populated list.
 */
@Composable
fun CadenceDialog(
    policy: CadencePolicy?,
    isMutating: Boolean = false,
    onConfirm: (intervalDays: Int, qualifyingTypes: List<String>) -> Unit,
    onDismiss: () -> Unit,
) {
    val isEditing = policy != null
    var intervalText by remember { mutableStateOf((policy?.targetIntervalDays ?: 30).toString()) }
    var selected by remember { mutableStateOf(policy?.qualifyingTypes.orEmpty().toSet()) }
    var intervalError by remember { mutableStateOf(false) }

    AlertDialog(
        onDismissRequest = { if (!isMutating) onDismiss() },
        // #208: an AlertDialog's title slot isn't marked as a heading by
        // default, so TalkBack's heading navigation skips right over it.
        title = {
            Text(
                stringResource(if (isEditing) R.string.cadence_edit_title else R.string.cadence_create_title),
                modifier = Modifier.semantics { heading() },
            )
        },
        text = {
            Column(
                verticalArrangement = Arrangement.spacedBy(8.dp),
                modifier = Modifier.verticalScroll(rememberScrollState()),
            ) {
                OutlinedTextField(
                    value = intervalText,
                    onValueChange = { intervalText = it; intervalError = false },
                    label = { Text(stringResource(R.string.cadence_interval_days)) },
                    supportingText = { Text(stringResource(R.string.cadence_interval_hint)) },
                    isError = intervalError,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                if (intervalError) {
                    Text(
                        text = stringResource(R.string.cadence_interval_required),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.error,
                        modifier = Modifier.semantics { liveRegion = LiveRegionMode.Assertive },
                    )
                }
                Text(
                    stringResource(R.string.cadence_qualifying_types),
                    style = MaterialTheme.typography.titleSmall,
                    modifier = Modifier.semantics { heading() },
                )
                Text(
                    text = stringResource(R.string.cadence_qualifying_hint),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                CadenceQualifyingType.ALL.forEach { token ->
                    // #199: a bare Checkbox has no text of its own, and the label
                    // Text carried its own separate .clickable — TalkBack found two
                    // adjacent focusable nodes (an unnamed checkbox, then a plain
                    // clickable label with no role/state). Modifier.toggleable on
                    // the row merges the label into the checkbox's accessible name.
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        modifier = Modifier
                            .fillMaxWidth()
                            .toggleable(
                                value = token in selected,
                                onValueChange = { checked ->
                                    selected = if (checked) selected + token else selected - token
                                },
                                role = Role.Checkbox,
                            ),
                    ) {
                        Checkbox(
                            checked = token in selected,
                            onCheckedChange = null,
                        )
                        Text(text = cadenceTypeLabel(token))
                    }
                }
            }
        },
        confirmButton = {
            TextButton(
                enabled = !isMutating,
                onClick = {
                    val days = intervalText.toIntOrNull() ?: 0
                    if (days < 1) {
                        intervalError = true
                    } else {
                        onConfirm(days, CadenceQualifyingType.ALL.filter { it in selected })
                    }
                },
            ) {
                Text(stringResource(if (isEditing) R.string.action_save else R.string.action_create))
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss, enabled = !isMutating) {
                Text(stringResource(R.string.action_cancel))
            }
        },
    )
}

/** Human label for a qualifying-type token, falling back to the raw token for future ones. */
@Composable
fun cadenceTypeLabel(token: String): String = when (token) {
    CadenceQualifyingType.CALL -> stringResource(R.string.cadence_type_call)
    CadenceQualifyingType.VIDEO_CALL -> stringResource(R.string.cadence_type_video_call)
    CadenceQualifyingType.VISIT -> stringResource(R.string.cadence_type_visit)
    CadenceQualifyingType.MEAL -> stringResource(R.string.cadence_type_meal)
    CadenceQualifyingType.GIFT -> stringResource(R.string.cadence_type_gift)
    CadenceQualifyingType.MESSAGE -> stringResource(R.string.cadence_type_message)
    CadenceQualifyingType.SHARED_ACTIVITY -> stringResource(R.string.cadence_type_shared_activity)
    else -> token
}
