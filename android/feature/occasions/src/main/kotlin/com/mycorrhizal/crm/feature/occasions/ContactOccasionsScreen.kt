package com.mycorrhizal.crm.feature.occasions

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.SuggestionChip
import androidx.compose.material3.Surface
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.OccasionObligation
import com.mycorrhizal.crm.model.network.OccasionObligationKinds
import com.mycorrhizal.crm.model.registry.OccasionSensitivity
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.BrandFab
import com.mycorrhizal.crm.ui.components.DropdownOption
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.FabListContentPadding
import com.mycorrhizal.crm.ui.components.FormScaffold
import com.mycorrhizal.crm.ui.components.LabelledDropdown
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.components.RefreshableContent
import com.mycorrhizal.crm.ui.components.ScreenTitle
import com.mycorrhizal.crm.ui.components.localizedMessage
import java.time.Month
import java.time.format.TextStyle
import java.util.Locale

/**
 * A contact's standing occasion obligations (docs/adrs/0024-occasions.md, issue #2521): the
 * recurring card/gift/invite rules that create their own reminders. Reached from the contact
 * page's "Occasions" drill-down, like preferences and gifts.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ContactOccasionsScreen(
    onBack: () -> Unit,
    viewModel: ContactOccasionsViewModel = hiltViewModel()
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    var showForm by rememberSaveable { mutableStateOf(false) }
    var editingId by rememberSaveable { mutableStateOf<String?>(null) }
    var pendingDelete by remember { mutableStateOf<OccasionObligation?>(null) }
    val editing = editingId?.let { id -> state.items.firstOrNull { it.id == id } }
    val snackbarHostState = remember { SnackbarHostState() }
    val currentOnErrorShown = rememberUpdatedState(viewModel::onErrorShown)
    val errorMessage = state.errorRes?.let { stringResource(it) } ?: state.error?.localizedMessage()

    // A successful save returns to the list; a rejected one keeps the form open with its input.
    LaunchedEffect(state.saved) {
        if (state.saved) {
            showForm = false
            editingId = null
            viewModel.onSavedHandled()
        }
    }
    errorMessage?.let { message ->
        LaunchedEffect(message) {
            snackbarHostState.showSnackbar(message)
            currentOnErrorShown.value()
        }
    }

    if (showForm) {
        OccasionObligationForm(
            initial = editing,
            isSaving = state.isSaving,
            snackbarHostState = snackbarHostState,
            onSave = { form -> viewModel.save(editing, form) },
            onBack = {
                showForm = false
                editingId = null
            }
        )
        return
    }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(
                            Icons.AutoMirrored.Outlined.ArrowBack,
                            contentDescription = stringResource(R.string.cd_back)
                        )
                    }
                },
                title = { ScreenTitle(text = stringResource(R.string.contact_occasions)) },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary,
                    actionIconContentColor = MaterialTheme.colorScheme.onPrimary
                )
            )
        },
        floatingActionButton = {
            BrandFab(onClick = { showForm = true }) {
                Icon(
                    Icons.Outlined.Add,
                    contentDescription = stringResource(R.string.occasion_ob_add)
                )
            }
        },
        snackbarHost = { SnackbarHost(snackbarHostState) }
    ) { padding ->
        RefreshableContent(
            isRefreshing = state.isLoading && state.items.isNotEmpty(),
            onRefresh = viewModel::load,
            modifier = Modifier.fillMaxSize().padding(padding)
        ) {
            LazyColumn(
                modifier = Modifier.fillMaxSize().testTag("occasion-ob-list"),
                contentPadding = FabListContentPadding
            ) {
                when {
                    state.isLoading && state.items.isEmpty() -> item { LoadingSkeleton() }

                    state.items.isEmpty() && errorMessage == null -> item {
                        EmptyState(
                            title = stringResource(R.string.occasion_ob_empty),
                            body = stringResource(R.string.occasion_ob_empty_body),
                            actionLabel = stringResource(R.string.occasion_ob_add),
                            onAction = { showForm = true }
                        )
                    }

                    else -> items(state.items, key = { it.id }) { obligation ->
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clickable {
                                    editingId = obligation.id
                                    showForm = true
                                }
                                .padding(horizontal = 16.dp, vertical = 12.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(4.dp)
                        ) {
                            OccasionObligationRow(
                                obligation,
                                modifier = Modifier.weight(1f)
                            )
                            IconButton(
                                onClick = {
                                    editingId = obligation.id
                                    showForm = true
                                }
                            ) {
                                Icon(
                                    Icons.Outlined.Edit,
                                    contentDescription = stringResource(R.string.action_edit)
                                )
                            }
                            IconButton(
                                onClick = { pendingDelete = obligation },
                                enabled = state.deletingId != obligation.id
                            ) {
                                Icon(
                                    Icons.Outlined.Delete,
                                    contentDescription = stringResource(
                                        R.string.entities_delete_named,
                                        obligation.label
                                    )
                                )
                            }
                        }
                    }
                }
            }
        }
    }

    pendingDelete?.let { target ->
        AlertDialog(
            onDismissRequest = { pendingDelete = null },
            title = { Text(stringResource(R.string.occasion_ob_delete_title)) },
            text = { Text(stringResource(R.string.occasions_delete_confirm, target.label)) },
            confirmButton = {
                TextButton(onClick = {
                    viewModel.delete(target.id)
                    pendingDelete = null
                }) { Text(stringResource(R.string.action_delete)) }
            },
            dismissButton = {
                TextButton(onClick = { pendingDelete = null }) {
                    Text(stringResource(R.string.action_cancel))
                }
            }
        )
    }
}

/**
 * One obligation: kind chip, annual date and lead time on the first line, then the label and any
 * notes — web's `OccasionObligationList` row. `internal` so tests can drive it directly.
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
internal fun OccasionObligationRow(obligation: OccasionObligation, modifier: Modifier = Modifier) {
    Column(modifier = modifier) {
        FlowRow(
            verticalArrangement = Arrangement.spacedBy(4.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            itemVerticalAlignment = Alignment.CenterVertically
        ) {
            Text(
                text = obligationKindText(obligation.kind),
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.primary
            )
            anchorText(obligation)?.let {
                Text(
                    it,
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            }
            if (obligation.leadTimeDays > 0) {
                Text(
                    pluralStringResource(
                        R.plurals.occasion_ob_lead_time_display,
                        obligation.leadTimeDays,
                        obligation.leadTimeDays
                    ),
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            }
            if (!obligation.active) {
                Badge(stringResource(R.string.occasion_ob_inactive))
            }
            if (obligation.sensitivity != OccasionSensitivity.NORMAL) {
                Badge(stringResource(sensitivityLabelRes(obligation.sensitivity)))
            }
        }
        Text(text = obligation.label, style = MaterialTheme.typography.bodyLarge)
        obligation.notes?.takeIf { it.isNotBlank() }?.let { notes ->
            Text(
                text = notes,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }
    }
}

@Composable
private fun Badge(text: String) {
    Surface(color = MaterialTheme.colorScheme.surfaceVariant, shape = RoundedCornerShape(4.dp)) {
        Text(
            text = text,
            style = MaterialTheme.typography.labelSmall,
            modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.dp)
        )
    }
}

/** "December 25": the month spelled in the device locale, then the day; null without an anchor. */
private fun anchorText(obligation: OccasionObligation): String? {
    val month = obligation.anchorMonth
    val day = obligation.anchorDay
    if (month == null || day == null) return null
    return "${monthName(month)} $day"
}

internal fun monthName(month: Int): String =
    Month.of(month).getDisplayName(TextStyle.FULL_STANDALONE, Locale.getDefault())

@Composable
private fun obligationKindText(kind: String): String = when (kind) {
    OccasionObligationKinds.CARD -> stringResource(R.string.occasion_ob_kind_card)
    OccasionObligationKinds.GIFT -> stringResource(R.string.occasion_ob_kind_gift)
    OccasionObligationKinds.INVITE -> stringResource(R.string.occasion_ob_kind_invite)
    else -> kind
}

@androidx.annotation.StringRes
private fun sensitivityLabelRes(sensitivity: String): Int = when (sensitivity) {
    OccasionSensitivity.PRIVATE -> R.string.occasions_sensitivity_private
    OccasionSensitivity.SECRET -> R.string.occasions_sensitivity_secret
    else -> R.string.occasions_sensitivity_normal
}

private const val DAYS_IN_MONTH_MAX = 31

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun OccasionObligationForm(
    initial: OccasionObligation?,
    isSaving: Boolean,
    snackbarHostState: SnackbarHostState,
    onSave: (OccasionObligationFormData) -> Unit,
    onBack: () -> Unit
) {
    val isEditing = initial != null
    var kind by rememberSaveable(initial?.id) {
        mutableStateOf(initial?.kind ?: OccasionObligationKinds.CARD)
    }
    var label by rememberSaveable(initial?.id) { mutableStateOf(initial?.label ?: "") }
    var month by rememberSaveable(initial?.id) { mutableStateOf(initial?.anchorMonth) }
    var day by rememberSaveable(initial?.id) { mutableStateOf(initial?.anchorDay) }
    var leadTime by rememberSaveable(initial?.id) {
        mutableStateOf((initial?.leadTimeDays ?: 0).toString())
    }
    var active by rememberSaveable(initial?.id) { mutableStateOf(initial?.active ?: true) }
    var sensitivity by rememberSaveable(initial?.id) {
        mutableStateOf(initial?.sensitivity ?: OccasionSensitivity.NORMAL)
    }
    var notes by rememberSaveable(initial?.id) { mutableStateOf(initial?.notes ?: "") }
    val isDirty = kind != (initial?.kind ?: OccasionObligationKinds.CARD) ||
        label != (initial?.label ?: "") ||
        month != initial?.anchorMonth ||
        day != initial?.anchorDay ||
        leadTime != (initial?.leadTimeDays ?: 0).toString() ||
        active != (initial?.active ?: true) ||
        sensitivity != (initial?.sensitivity ?: OccasionSensitivity.NORMAL) ||
        notes != (initial?.notes ?: "")

    FormScaffold(
        title = stringResource(
            if (isEditing) R.string.occasion_ob_edit_title else R.string.occasion_ob_create_title
        ),
        onBack = onBack,
        saveLabel = stringResource(if (isEditing) R.string.action_save else R.string.action_create),
        isSaving = isSaving,
        onSave = {
            onSave(
                OccasionObligationFormData(
                    kind = kind,
                    label = label,
                    anchorMonth = month,
                    anchorDay = day,
                    leadTimeText = leadTime,
                    active = active,
                    sensitivity = sensitivity,
                    notes = notes
                )
            )
        },
        snackbarHostState = snackbarHostState,
        saveEnabled = label.isNotBlank(),
        saveDisabledHint = stringResource(R.string.occasion_ob_validation_label_required),
        isDirty = isDirty,
        entityLabel = stringResource(R.string.entity_occasion)
    ) { padding ->
        Column(
            modifier = Modifier
                .padding(padding)
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            Text(
                text = stringResource(R.string.occasion_ob_hint),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            OutlinedTextField(
                value = kind,
                onValueChange = { kind = it },
                label = { Text(stringResource(R.string.occasion_ob_kind)) },
                supportingText = { Text(stringResource(R.string.occasion_ob_kind_hint)) },
                singleLine = true,
                modifier = Modifier.fillMaxWidth().testTag("occasion-ob-kind")
            )
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                OccasionObligationKinds.ALL.forEach { suggestion ->
                    SuggestionChip(
                        onClick = { kind = suggestion },
                        label = { Text(obligationKindText(suggestion)) }
                    )
                }
            }
            OutlinedTextField(
                value = label,
                onValueChange = { label = it },
                label = { Text(stringResource(R.string.occasion_ob_label)) },
                supportingText = { Text(stringResource(R.string.occasion_ob_label_hint)) },
                singleLine = true,
                modifier = Modifier.fillMaxWidth().testTag("occasion-ob-label")
            )
            val unset = stringResource(R.string.occasion_ob_anchor_unset)
            LabelledDropdown(
                label = stringResource(R.string.occasion_ob_anchor_month),
                valueText = month?.let { monthName(it) } ?: unset,
                options = listOf(DropdownOption<Int?>(null, unset)) +
                    (1..Month.entries.size).map { DropdownOption<Int?>(it, monthName(it)) },
                onSelect = { month = it },
                modifier = Modifier.testTag("occasion-ob-month")
            )
            LabelledDropdown(
                label = stringResource(R.string.occasion_ob_anchor_day),
                valueText = day?.toString() ?: unset,
                options = listOf(DropdownOption<Int?>(null, unset)) +
                    (1..DAYS_IN_MONTH_MAX).map { DropdownOption<Int?>(it, it.toString()) },
                onSelect = { day = it },
                modifier = Modifier.testTag("occasion-ob-day")
            )
            OutlinedTextField(
                value = leadTime,
                onValueChange = { leadTime = it },
                label = { Text(stringResource(R.string.occasion_ob_lead_time)) },
                supportingText = { Text(stringResource(R.string.occasion_ob_lead_time_hint)) },
                singleLine = true,
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                modifier = Modifier.fillMaxWidth().testTag("occasion-ob-lead-time")
            )
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .semantics(mergeDescendants = true) { role = Role.Switch }
                    .clickable { active = !active },
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                Text(stringResource(R.string.occasion_ob_active))
                Switch(checked = active, onCheckedChange = null)
            }
            LabelledDropdown(
                label = stringResource(R.string.occasions_sensitivity),
                valueText = stringResource(sensitivityLabelRes(sensitivity)),
                options = OccasionSensitivity.ALL.map {
                    DropdownOption(it, stringResource(sensitivityLabelRes(it)))
                },
                onSelect = { sensitivity = it },
                modifier = Modifier.testTag("occasion-ob-sensitivity")
            )
            OutlinedTextField(
                value = notes,
                onValueChange = { notes = it },
                label = { Text(stringResource(R.string.occasions_notes)) },
                modifier = Modifier.fillMaxWidth().testTag("occasion-ob-notes")
            )
        }
    }
}
