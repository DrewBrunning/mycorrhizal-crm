package com.mycorrhizal.crm.feature.timelineentities

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
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.CheckCircle
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.Done
import androidx.compose.material.icons.outlined.OpenInNew
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Checkbox
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
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.foundation.selection.toggleable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.platform.testTag
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.hilt.navigation.compose.hiltViewModel
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.ConversationAgenda
import com.mycorrhizal.crm.model.network.Gift
import com.mycorrhizal.crm.model.network.GiftStatuses
import com.mycorrhizal.crm.model.network.LifeEvent
import com.mycorrhizal.crm.model.network.PartialDate
import com.mycorrhizal.crm.model.network.Preference
import com.mycorrhizal.crm.model.network.PreferenceSensitivities
import com.mycorrhizal.crm.model.registry.LifeEventCategory
import com.mycorrhizal.crm.model.registry.LifeEventTypes
import com.mycorrhizal.crm.model.registry.PreferenceCategory
import com.mycorrhizal.crm.model.registry.PreferenceCategoryConfig
import com.mycorrhizal.crm.model.registry.PreferenceSection
import com.mycorrhizal.crm.model.util.Validators
import com.mycorrhizal.crm.ui.components.BrandFab
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.R
import java.math.BigDecimal

// M18 (100-M18-android-entity-field-richness.md): each entity's dialog gains
// the fields its web counterpart models — life-event category/typed partial
// date/related contacts/remind; gift status/url/notes/occasion/date/amount+
// currency/life-event+activity links plus a mark-given action; preference
// category select/key autocomplete/sensitivity plus section grouping and the
// clothing-sizes panel; agenda reference URL plus mark-discussed with an
// activity link and an open/discussed section split.

private const val LIFE_EVENT_UNCATEGORIZED = "uncategorized"

// ---------------------------------------------------------------------------
// Shared scaffold
// ---------------------------------------------------------------------------

/**
 * The shared entity-list scaffold. `internal` (not `private`) so
 * EntityListScaffoldTest can drive it directly with a fake [EntityListUiState]
 * and plain callbacks — no ViewModel, no coroutines, matching this module's
 * existing test-layer split (ViewModel logic is tested separately in
 * TimelineEntitiesViewModelTest).
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun EntityListScaffold(
    title: String,
    addLabel: String,
    uiState: EntityListUiState,
    onAdd: () -> Unit,
    onItemClick: (String) -> Unit,
    onDelete: (String) -> Unit,
    onErrorShown: () -> Unit,
    onBack: () -> Unit,
    // M18: optional list-grouping headers (e.g. "open"/"discussed").
    // @Composable because the label may be a localized string resource.
    sectionLabel: (@Composable (String) -> String)? = null,
    // M18: an optional per-row trailing action (e.g. gift mark-given, agenda discuss).
    extraAction: (@Composable (EntityItem) -> Unit)? = null,
    // Optional in-layout content rendered above the list (e.g. gifts' clothing-sizes
    // panel). Deliberately separate from [dialog]: dialog content (AlertDialog) draws
    // in its own Popup/window and never affects layout, but this slot's content is a
    // normal composable and must be laid out in-flow with the scaffold, not stacked
    // as an overlapping sibling of it (that overlap was a real bug — review-pass fix).
    header: (@Composable () -> Unit)? = null,
    dialog: @Composable () -> Unit,
) {
    val snackbarHostState = remember { SnackbarHostState() }
    val errorMessage = uiState.errorRes?.let { stringResource(it) } ?: uiState.error
    var pendingDeleteId by remember { mutableStateOf<String?>(null) }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
                title = { Text(title, style = MaterialTheme.typography.titleLarge) },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary,
                    actionIconContentColor = MaterialTheme.colorScheme.onPrimary,
                ),
            )
        },
        floatingActionButton = {
            BrandFab(onClick = onAdd) {
                Icon(Icons.Outlined.Add, contentDescription = addLabel)
            }
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        Column(modifier = Modifier.fillMaxSize().padding(padding)) {
            header?.invoke()
            Box(modifier = Modifier.weight(1f)) {
                when {
                    uiState.isLoading -> LoadingSkeleton()
                    uiState.items.isEmpty() && errorMessage == null ->
                        EmptyState(message = stringResource(R.string.entities_empty))
                    uiState.items.isEmpty() && errorMessage != null -> {
                        Text(
                            text = errorMessage,
                            color = MaterialTheme.colorScheme.error,
                            modifier = Modifier.align(Alignment.Center),
                        )
                    }
                    else -> {
                        val uriHandler = LocalUriHandler.current
                        // M18: flatten section headers into the row stream so the
                        // generic scaffold can render grouped lists (preferences,
                        // agenda) without per-entity list implementations.
                        val rows = sectionRows(uiState.items, sectionLabel != null)
                        LazyColumn(modifier = Modifier.fillMaxSize()) {
                            items(rows, key = { it.key }) { row ->
                                when (row) {
                                    is SectionRow.Header -> {
                                        Text(
                                            text = sectionLabel?.invoke(row.sectionKey).orEmpty(),
                                            style = MaterialTheme.typography.titleSmall,
                                            color = MaterialTheme.colorScheme.primary,
                                            modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
                                        )
                                    }
                                    is SectionRow.Item -> {
                                        val item = row.item
                                        Row(
                                            modifier = Modifier
                                                .fillMaxWidth()
                                                .clickable(onClick = { onItemClick(item.id) })
                                                .padding(horizontal = 16.dp, vertical = 12.dp),
                                            verticalAlignment = Alignment.CenterVertically,
                                            horizontalArrangement = Arrangement.spacedBy(4.dp),
                                        ) {
                                            Column(modifier = Modifier.weight(1f)) {
                                                Text(
                                                    text = item.label,
                                                    style = MaterialTheme.typography.bodyLarge,
                                                    maxLines = 2,
                                                    overflow = TextOverflow.Ellipsis,
                                                )
                                                if (!item.url.isNullOrBlank()) {
                                                    Text(
                                                        text = item.url,
                                                        style = MaterialTheme.typography.bodyMedium,
                                                        color = MaterialTheme.colorScheme.primary,
                                                        maxLines = 1,
                                                        overflow = TextOverflow.Ellipsis,
                                                    )
                                                }
                                            }
                                            if (!item.url.isNullOrBlank()) {
                                                IconButton(onClick = { uriHandler.openUri(item.url) }) {
                                                    Icon(
                                                        Icons.Outlined.OpenInNew,
                                                        contentDescription = stringResource(R.string.cd_open_link),
                                                        tint = MaterialTheme.colorScheme.primary,
                                                    )
                                                }
                                            }
                                            extraAction?.invoke(item)
                                            IconButton(
                                                onClick = { pendingDeleteId = item.id },
                                                enabled = uiState.deletingId != item.id,
                                            ) {
                                                Icon(Icons.Outlined.Delete, contentDescription = stringResource(R.string.action_delete))
                                            }
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
    }

    dialog()

    pendingDeleteId?.let { id ->
        val label = uiState.items.firstOrNull { it.id == id }?.label
        AlertDialog(
            onDismissRequest = { pendingDeleteId = null },
            title = { Text(stringResource(R.string.entities_delete_title)) },
            text = {
                Text(
                    label?.let { stringResource(R.string.entities_delete_confirm, it) }
                        ?: stringResource(R.string.entities_delete_title),
                )
            },
            confirmButton = {
                TextButton(onClick = { onDelete(id); pendingDeleteId = null }) {
                    Text(stringResource(R.string.action_delete))
                }
            },
            dismissButton = {
                TextButton(onClick = { pendingDeleteId = null }) {
                    Text(stringResource(R.string.action_cancel))
                }
            },
        )
    }

    errorMessage?.let { message ->
        LaunchedEffect(message) {
            snackbarHostState.showSnackbar(message)
            onErrorShown()
        }
    }
}

/** A [SectionRow.Header] between [SectionRow.Item]s whenever the section key changes. */
private fun sectionRows(items: List<EntityItem>, sectioned: Boolean): List<SectionRow> {
    if (!sectioned) return items.map { SectionRow.Item(it) }
    return buildList {
        var last: String? = null
        items.forEach { item ->
            if (item.sectionKey != null && item.sectionKey != last) {
                add(SectionRow.Header(item.sectionKey))
                last = item.sectionKey
            }
            add(SectionRow.Item(item))
        }
    }
}

private sealed interface SectionRow {
    val key: String
    data class Header(val sectionKey: String) : SectionRow {
        override val key: String get() = "header-$sectionKey"
    }
    data class Item(val item: EntityItem) : SectionRow {
        override val key: String get() = "item-${item.id}"
    }
}

// ---------------------------------------------------------------------------
// Life events
// ---------------------------------------------------------------------------

/**
 * `initial == null` is add mode; a non-null [LifeEvent] pre-fills every field
 * this form models and switches the dialog to edit mode (title/button label,
 * and [onConfirm] is expected to route to update() instead of create() —
 * that branch lives in the caller, which is the one that knows whether it has
 * an `editingItem`).
 */
@Composable
internal fun LifeEventDialog(
    initial: LifeEvent?,
    relatedContacts: List<ContactSummary>,
    contactSearchQuery: String,
    contactSearchResults: List<ContactSummary>,
    contactSearchLoading: Boolean,
    onSearchContacts: (String) -> Unit,
    onAddRelated: (ContactSummary) -> Unit,
    onRemoveRelated: (String) -> Unit,
    onConfirm: (LifeEventFormData) -> Unit,
    onDismiss: () -> Unit,
) {
    val isEditing = initial != null
    // Category state: a real token, "" (nothing chosen yet), or the
    // "uncategorized" sentinel (only reachable for existing data whose
    // category is a legacy/unknown value or absent — brand-new events must
    // pick a real category). The sentinel falls back on `initial != null`,
    // NOT `initial?.category != null`: pre-T36 rows carry category NULL, which
    // Go serializes as an absent JSON key (review-pass fix).
    var category by remember(initial) {
        mutableStateOf(
            initial?.category
                ?.takeIf { it in LifeEventCategory.ALL }
                ?: (if (initial != null) LIFE_EVENT_UNCATEGORIZED else ""),
        )
    }
    var type by remember(initial) { mutableStateOf(initial?.type ?: "") }
    var customType by remember(initial) {
        mutableStateOf(
            initial?.type != null &&
                category in LifeEventCategory.ALL &&
                initial.type !in LifeEventTypes.forCategory(category),
        )
    }
    var description by remember(initial) { mutableStateOf(initial?.description ?: "") }
    var year by remember(initial) { mutableStateOf(initial?.date?.year?.toString() ?: "") }
    var month by remember(initial) { mutableStateOf(initial?.date?.month?.toString() ?: "") }
    var day by remember(initial) { mutableStateOf(initial?.date?.day?.toString() ?: "") }
    var remind by remember(initial) { mutableStateOf(initial?.remind == true) }

    val canRemind = month.toIntOrNull() != null && day.toIntOrNull() != null
    val typeEnabled = category.isNotEmpty()
    val canSave = type.isNotBlank()

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(if (isEditing) R.string.life_events_edit else R.string.life_events_new)) },
        text = {
            Column(
                verticalArrangement = Arrangement.spacedBy(8.dp),
                modifier = Modifier.fillMaxWidth().verticalScroll(rememberScrollState()),
            ) {
                // Category select.
                var categoryMenu by remember { mutableStateOf(false) }
                Box {
                    OutlinedButton(onClick = { categoryMenu = true }, modifier = Modifier.fillMaxWidth().testTag("life-event-category")) {
                        Text(
                            text = categoryLabel(category),
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                        )
                    }
                    DropdownMenu(expanded = categoryMenu, onDismissRequest = { categoryMenu = false }) {
                        LifeEventCategory.ALL.forEach { token ->
                            DropdownMenuItem(
                                text = { Text(stringResource(categoryLabelRes(token))) },
                                onClick = {
                                    categoryMenu = false
                                    category = token
                                    // Picking a real category may orphan a
                                    // custom type; fall back to the select.
                                    customType = type.isNotBlank() && type !in LifeEventTypes.forCategory(token)
                                },
                            )
                        }
                        if (category == LIFE_EVENT_UNCATEGORIZED) {
                            DropdownMenuItem(
                                text = { Text(stringResource(R.string.life_events_category_uncategorized)) },
                                onClick = {
                                    categoryMenu = false
                                    category = LIFE_EVENT_UNCATEGORIZED
                                },
                            )
                        }
                    }
                }

                // Type: disabled until a category is chosen; free text for
                // uncategorized; a category-scoped select with a custom escape
                // hatch for real categories.
                if (category == LIFE_EVENT_UNCATEGORIZED || customType) {
                    OutlinedTextField(
                        value = type, onValueChange = { type = it },
                        label = { Text(stringResource(R.string.life_events_custom_type_label)) }, singleLine = true,
                        enabled = typeEnabled,
                    )
                } else {
                    var typeMenu by remember { mutableStateOf(false) }
                    Box {
                        OutlinedButton(
                            onClick = { typeMenu = true },
                            enabled = typeEnabled,
                            modifier = Modifier.fillMaxWidth().testTag("life-event-type"),
                        ) {
                            Text(
                                text = type.replace('_', ' ').ifBlank { stringResource(R.string.life_events_select_category_first) },
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis,
                            )
                        }
                        DropdownMenu(expanded = typeMenu && category in LifeEventCategory.ALL, onDismissRequest = { typeMenu = false }) {
                            LifeEventTypes.forCategory(category).forEach { token ->
                                DropdownMenuItem(
                                    text = { Text(token.replace('_', ' ')) },
                                    onClick = {
                                        typeMenu = false
                                        type = token
                                        customType = false
                                    },
                                )
                            }
                            DropdownMenuItem(
                                text = { Text(stringResource(R.string.life_events_custom_type)) },
                                onClick = {
                                    typeMenu = false
                                    customType = true
                                },
                            )
                        }
                    }
                }

                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedTextField(
                        value = year, onValueChange = { year = it.filter { c -> c.isDigit() }.take(4) },
                        label = { Text(stringResource(R.string.life_events_year)) }, singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                        modifier = Modifier.weight(1f),
                    )
                    OutlinedTextField(
                        value = month, onValueChange = { month = it.filter { c -> c.isDigit() }.take(2) },
                        label = { Text(stringResource(R.string.life_events_month)) }, singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                        modifier = Modifier.weight(1f),
                    )
                    OutlinedTextField(
                        value = day, onValueChange = { day = it.filter { c -> c.isDigit() }.take(2) },
                        label = { Text(stringResource(R.string.life_events_day)) }, singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                        modifier = Modifier.weight(1f),
                    )
                }

                OutlinedTextField(
                    value = description, onValueChange = { description = it },
                    label = { Text(stringResource(R.string.life_events_description)) }, singleLine = true,
                )

                // Related contacts: selected chips + a search to add more.
                Text(
                    text = stringResource(R.string.life_events_related),
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                relatedContacts.forEach { contact ->
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                        Text(
                            text = contact.displayName,
                            style = MaterialTheme.typography.bodyMedium,
                            modifier = Modifier.weight(1f),
                        )
                        IconButton(onClick = { contact.uid?.let(onRemoveRelated) }) {
                            Icon(
                                Icons.Outlined.Delete,
                                contentDescription = stringResource(R.string.life_events_remove_related, contact.displayName),
                                tint = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    }
                }
                OutlinedTextField(
                    value = contactSearchQuery, onValueChange = onSearchContacts,
                    label = { Text(stringResource(R.string.life_events_search_related)) }, singleLine = true,
                )
                if (contactSearchLoading) {
                    Text(
                        text = "…",
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                } else {
                    contactSearchResults.forEach { contact ->
                        TextButton(
                            onClick = { onAddRelated(contact) },
                            modifier = Modifier.fillMaxWidth(),
                        ) {
                            Text(contact.displayName, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        }
                    }
                }

                // #199: a bare Checkbox has no text of its own — the adjacent
                // "Remind" Text was a separate, unassociated node, so TalkBack
                // announced the checkbox with no name. Modifier.toggleable on
                // the row merges the label into the checkbox's accessible name;
                // enabled/testTag move from the (now decorative) Checkbox to the
                // row, since the row is the interactive element now.
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    modifier = Modifier
                        .toggleable(
                            value = remind,
                            onValueChange = { remind = it },
                            enabled = canRemind,
                            role = Role.Checkbox,
                        )
                        .testTag("life-event-remind"),
                ) {
                    Checkbox(
                        checked = remind,
                        onCheckedChange = null,
                        enabled = canRemind,
                    )
                    Text(
                        text = stringResource(R.string.life_events_remind),
                        style = MaterialTheme.typography.bodyMedium,
                        color = if (canRemind) MaterialTheme.colorScheme.onSurface else MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        },
        confirmButton = {
            TextButton(
                onClick = {
                    val date = PartialDate(
                        year = year.toIntOrNull(),
                        month = month.toIntOrNull(),
                        day = day.toIntOrNull(),
                    ).takeIf { it.year != null || it.month != null || it.day != null }
                    onConfirm(
                        LifeEventFormData(
                            type = type,
                            category = category.takeIf { it in LifeEventCategory.ALL },
                            description = description,
                            date = date,
                            relatedEntityIds = relatedContacts.mapNotNull { it.uid },
                            remind = (date?.hasMonthDay == true) && remind,
                        ),
                    )
                },
                enabled = canSave,
            ) { Text(stringResource(if (isEditing) R.string.action_save else R.string.action_create)) }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) }
        },
    )
}

@Composable
private fun categoryLabel(category: String): String = when {
    category in LifeEventCategory.ALL -> stringResource(categoryLabelRes(category))
    category == LIFE_EVENT_UNCATEGORIZED -> stringResource(R.string.life_events_category_uncategorized)
    else -> stringResource(R.string.life_events_select_category)
}

@androidx.annotation.StringRes
private fun categoryLabelRes(token: String): Int = when (token) {
    LifeEventCategory.HOME_LIVING -> R.string.life_events_category_home_living
    LifeEventCategory.HEALTH_WELLNESS -> R.string.life_events_category_health_wellness
    LifeEventCategory.WORK_EDUCATION -> R.string.life_events_category_work_education
    LifeEventCategory.TRAVEL_EXPERIENCES -> R.string.life_events_category_travel_experiences
    LifeEventCategory.FAMILY_RELATIONSHIPS -> R.string.life_events_category_family_relationships
    else -> R.string.life_events_select_category
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun LifeEventsScreen(
    onBack: () -> Unit,
    viewModel: LifeEventsViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val relatedContacts by viewModel.relatedContacts.collectAsStateWithLifecycle()
    val relatedQuery by viewModel.relatedSearchQuery.collectAsStateWithLifecycle()
    val relatedResults by viewModel.relatedSearchResults.collectAsStateWithLifecycle()
    val relatedLoading by viewModel.relatedSearchLoading.collectAsStateWithLifecycle()
    var showAdd by remember { mutableStateOf(false) }
    var editingItem by remember { mutableStateOf<LifeEvent?>(null) }

    EntityListScaffold(
        title = stringResource(R.string.life_events_title),
        addLabel = stringResource(R.string.life_events_new),
        uiState = state,
        onAdd = { showAdd = true },
        onItemClick = { id -> viewModel.findById(id)?.let { editingItem = it } },
        onDelete = viewModel::delete,
        onErrorShown = viewModel::onErrorShown,
        onBack = onBack,
    ) {
        if (showAdd || editingItem != null) {
            LaunchedEffect(editingItem) { viewModel.onDialogOpened(editingItem) }
            LifeEventDialog(
                initial = editingItem,
                relatedContacts = relatedContacts,
                contactSearchQuery = relatedQuery,
                contactSearchResults = relatedResults,
                contactSearchLoading = relatedLoading,
                onSearchContacts = viewModel::searchRelated,
                onAddRelated = viewModel::addRelated,
                onRemoveRelated = viewModel::removeRelated,
                onConfirm = { form ->
                    editingItem?.let { viewModel.update(it, form) }
                        ?: viewModel.create(form)
                    viewModel.clearRelatedSearch()
                    showAdd = false
                    editingItem = null
                },
                onDismiss = {
                    viewModel.clearRelatedSearch()
                    showAdd = false
                    editingItem = null
                },
            )
        }
    }
}

// ---------------------------------------------------------------------------
// Gifts
// ---------------------------------------------------------------------------

@Composable
internal fun GiftDialog(
    initial: Gift?,
    lifeEvents: List<LifeEvent>,
    activities: List<Activity>,
    onConfirm: (GiftFormData) -> Unit,
    onDismiss: () -> Unit,
) {
    val isEditing = initial != null
    var status by remember(initial) { mutableStateOf(initial?.status ?: GiftStatuses.IDEA) }
    var description by remember(initial) { mutableStateOf(initial?.description ?: "") }
    var url by remember(initial) { mutableStateOf(initial?.url ?: "") }
    var notes by remember(initial) { mutableStateOf(initial?.notes ?: "") }
    var occasion by remember(initial) { mutableStateOf(initial?.occasion ?: "") }
    var date by remember(initial) { mutableStateOf(giftDateFromIso(initial?.date)) }
    var amount by remember(initial) { mutableStateOf(initial?.valueCents?.let(::centsToAmountText) ?: "") }
    var currency by remember(initial) { mutableStateOf(initial?.currency ?: "") }
    var lifeEventId by remember(initial) { mutableStateOf(initial?.lifeEventId) }
    var activityId by remember(initial) { mutableStateOf(initial?.activityId) }

    val parsedAmount = amount.toBigDecimalOrNull()
    val hasAmount = parsedAmount != null && parsedAmount >= BigDecimal.ZERO
    val hasCurrency = currency.isNotBlank()
    // The pair is enforced together (backend: validateGiftValueCurrency).
    val pairValid = (hasAmount == hasCurrency)
    val currencyLengthValid = currency.isBlank() || currency.length == 3
    val normalizedUrl = normalizeHttpUrl(url)
    val urlValid = url.isBlank() || normalizedUrl != null
    val dateValid = date.isBlank() || giftDateToIso(date) != null
    val canSave = description.isNotBlank() && pairValid && currencyLengthValid && urlValid && dateValid

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(if (isEditing) R.string.gifts_edit else R.string.gifts_new)) },
        text = {
            Column(
                verticalArrangement = Arrangement.spacedBy(8.dp),
                modifier = Modifier.fillMaxWidth().verticalScroll(rememberScrollState()),
            ) {
                StatusField(status = status, onStatusChange = { status = it })
                OutlinedTextField(
                    value = description, onValueChange = { description = it },
                    label = { Text(stringResource(R.string.gifts_description)) }, singleLine = true,
                )
                OutlinedTextField(
                    value = url, onValueChange = { url = it },
                    label = { Text(stringResource(R.string.gifts_url)) }, singleLine = true,
                    isError = !urlValid,
                    supportingText = if (urlValid) null else { { Text(stringResource(R.string.gifts_url_invalid)) } },
                )
                OutlinedTextField(
                    value = notes, onValueChange = { notes = it },
                    label = { Text(stringResource(R.string.gifts_notes)) }, singleLine = true,
                )
                OutlinedTextField(
                    value = occasion, onValueChange = { occasion = it },
                    label = { Text(stringResource(R.string.gifts_occasion)) }, singleLine = true,
                )
                OutlinedTextField(
                    value = date, onValueChange = { date = it },
                    label = { Text(stringResource(R.string.gifts_date)) }, singleLine = true,
                    placeholder = { Text("2026-08-10") },
                    isError = !dateValid,
                    supportingText = if (dateValid) null else { { Text(stringResource(R.string.gifts_date_invalid)) } },
                )
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedTextField(
                        value = amount, onValueChange = { amount = it },
                        label = { Text(stringResource(R.string.gifts_amount)) }, singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal),
                        isError = amount.isNotBlank() && !hasAmount,
                        modifier = Modifier.weight(1f),
                    )
                    OutlinedTextField(
                        value = currency, onValueChange = { currency = it.filter { c -> c.isLetter() }.uppercase().take(3) },
                        label = { Text(stringResource(R.string.gifts_currency)) }, singleLine = true,
                        placeholder = { Text("EUR") },
                        isError = currency.isNotBlank() && currency.length != 3,
                        modifier = Modifier.weight(1f),
                    )
                }
                if (!pairValid) {
                    Text(
                        text = stringResource(R.string.gifts_value_currency_required),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.error,
                    )
                }
                if (lifeEvents.isNotEmpty()) {
                    SelectField(
                        label = stringResource(R.string.gifts_life_event),
                        value = lifeEventId,
                        placeholder = stringResource(R.string.gifts_none),
                        options = lifeEvents.map { it.id },
                        optionLabel = { id -> lifeEvents.firstOrNull { it.id == id }?.type?.replace('_', ' ') ?: id },
                        onSelect = { lifeEventId = it },
                    )
                }
                if (activities.isNotEmpty()) {
                    SelectField(
                        label = stringResource(R.string.gifts_activity),
                        value = activityId?.toString(),
                        placeholder = stringResource(R.string.gifts_none),
                        options = activities.map { it.id.toString() },
                        optionLabel = { id -> activities.firstOrNull { a -> a.id.toString() == id }?.title ?: id },
                        onSelect = { activityId = it?.toIntOrNull() },
                    )
                }
            }
        },
        confirmButton = {
            TextButton(
                onClick = {
                    onConfirm(
                        GiftFormData(
                            status = status,
                            description = description,
                            url = normalizedUrl,
                            notes = notes,
                            occasion = occasion,
                            date = date,
                            // Only a non-negative amount is a real amount; a
                            // negative/zero-blank entry sends null (review-pass
                            // fix — web rejects negatives client-side).
                            valueCents = if (hasAmount) parsedAmount!!.movePointRight(2).toLong() else null,
                            currency = currency,
                            lifeEventId = lifeEventId,
                            activityId = activityId,
                        ),
                    )
                },
                enabled = canSave,
            ) { Text(stringResource(if (isEditing) R.string.action_save else R.string.action_create)) }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) }
        },
    )
}

@Composable
private fun StatusField(status: String, onStatusChange: (String) -> Unit) {
    var menu by remember { mutableStateOf(false) }
    Box {
        OutlinedButton(onClick = { menu = true }, modifier = Modifier.fillMaxWidth().testTag("gift-status")) {
            Text(statusLabel(status))
        }
        DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
            GiftStatuses.ALL.forEach { token ->
                DropdownMenuItem(
                    text = { Text(statusLabel(token)) },
                    onClick = {
                        menu = false
                        onStatusChange(token)
                    },
                )
            }
        }
    }
}

@Composable
private fun statusLabel(status: String): String = when (status) {
    GiftStatuses.IDEA -> stringResource(R.string.gifts_status_idea)
    GiftStatuses.PURCHASED -> stringResource(R.string.gifts_status_purchased)
    GiftStatuses.GIVEN -> stringResource(R.string.gifts_status_given)
    GiftStatuses.RECEIVED -> stringResource(R.string.gifts_status_received)
    else -> status
}

/** A labeled OutlinedButton + DropdownMenu select with a "none" choice. */
@Composable
private fun SelectField(
    label: String,
    value: String?,
    placeholder: String,
    options: List<String>,
    optionLabel: (String) -> String,
    onSelect: (String?) -> Unit,
) {
    var menu by remember { mutableStateOf(false) }
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text(
            text = label,
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Box {
            OutlinedButton(onClick = { menu = true }, modifier = Modifier.fillMaxWidth()) {
                Text(
                    text = value?.let(optionLabel) ?: placeholder,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
            DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                DropdownMenuItem(
                    text = { Text(placeholder) },
                    onClick = {
                        menu = false
                        onSelect(null)
                    },
                )
                options.forEach { option ->
                    DropdownMenuItem(
                        text = { Text(optionLabel(option), maxLines = 1, overflow = TextOverflow.Ellipsis) },
                        onClick = {
                            menu = false
                            onSelect(option)
                        },
                    )
                }
            }
        }
    }
}

/** `value_cents` -> a plain amount string ("2500" -> "25.00", "250000" -> "2500"). */
internal fun centsToAmountText(valueCents: Long): String {
    val amount = BigDecimal(valueCents).movePointLeft(2)
    return amount.stripTrailingZeros().toPlainString()
}

/**
 * Prepends https:// to a scheme-less value (mirroring web's GiftDialog), then
 * validates http(s); null when invalid. Like the web's looksLikeAbsoluteUri, a
 * value that already carries a scheme is validated as-is — so
 * `javascript:alert(1)` is rejected, not silently given a scheme.
 */
internal fun normalizeHttpUrl(value: String): String? {
    val trimmed = value.trim()
    if (trimmed.isBlank()) return null
    val looksLikeAbsoluteUri = Regex("^[a-zA-Z][a-zA-Z0-9+.-]*:").containsMatchIn(trimmed)
    val withScheme = if (looksLikeAbsoluteUri) trimmed else "https://$trimmed"
    return if (Validators.isValidHttpUrl(withScheme)) withScheme else null
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun GiftsScreen(
    onBack: () -> Unit,
    viewModel: GiftsViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val lifeEvents by viewModel.lifeEvents.collectAsStateWithLifecycle()
    val activities by viewModel.activities.collectAsStateWithLifecycle()
    val clothingSizes by viewModel.clothingSizes.collectAsStateWithLifecycle()
    val giftPreferences by viewModel.giftPreferences.collectAsStateWithLifecycle()
    var showAdd by remember { mutableStateOf(false) }
    var editingItem by remember { mutableStateOf<Gift?>(null) }
    var editingClothingItem by remember { mutableStateOf<Preference?>(null) }
    var newClothingKey by remember { mutableStateOf("") }
    var newClothingValue by remember { mutableStateOf("") }
    var showAddGiftPreference by remember { mutableStateOf(false) }
    var editingGiftPreference by remember { mutableStateOf<Preference?>(null) }

    EntityListScaffold(
        title = stringResource(R.string.gifts_title),
        addLabel = stringResource(R.string.gifts_new),
        uiState = state,
        onAdd = { showAdd = true },
        onItemClick = { id -> viewModel.findById(id)?.let { editingItem = it } },
        onDelete = viewModel::delete,
        onErrorShown = viewModel::onErrorShown,
        onBack = onBack,
        extraAction = { item ->
            val gift = viewModel.findById(item.id)
            if (gift != null && gift.status != GiftStatuses.GIVEN && gift.status != GiftStatuses.RECEIVED) {
                IconButton(onClick = { viewModel.markGiven(item.id) }) {
                    Icon(
                        Icons.Outlined.CheckCircle,
                        contentDescription = stringResource(R.string.gifts_mark_given),
                        tint = MaterialTheme.colorScheme.primary,
                    )
                }
            }
        },
        // Clothing sizes + gift preferences panels (web's ClothingSizesPanel
        // and its Gifts-tab jewelry/flowers/color/fragrance/cause/gift-avoid
        // panel): "check this right before buying" — surfaced here, not in
        // Preferences. Rendered through the scaffold's in-layout `header`
        // slot (not `dialog`) so it lays out above the gift list instead of
        // overlapping it.
        header = {
            if (!state.isLoading) {
                ClothingSizesPanel(
                    items = clothingSizes,
                    newKey = newClothingKey,
                    newValue = newClothingValue,
                    editingId = editingClothingItem?.id,
                    onNewKeyChange = { newClothingKey = it },
                    onNewValueChange = { newClothingValue = it },
                    onAdd = { key, value ->
                        viewModel.createClothingSize(key, value)
                        newClothingKey = ""
                        newClothingValue = ""
                    },
                    onStartEdit = { item -> editingClothingItem = item },
                    onEditConfirm = { item, key, value ->
                        viewModel.updateClothingSize(item, key, value)
                        editingClothingItem = null
                    },
                    onEditCancel = { editingClothingItem = null },
                    onDelete = viewModel::deleteClothingSize,
                )
                GiftPreferencesPanel(
                    items = giftPreferences,
                    onAdd = { showAddGiftPreference = true },
                    onEdit = { item -> editingGiftPreference = item },
                    onDelete = viewModel::deleteGiftPreference,
                )
            }
        },
    ) {
        if (showAdd || editingItem != null) {
            GiftDialog(
                initial = editingItem,
                lifeEvents = lifeEvents,
                activities = activities,
                onConfirm = { form ->
                    editingItem?.let { viewModel.update(it, form) }
                        ?: viewModel.create(form)
                    showAdd = false
                    editingItem = null
                },
                onDismiss = { showAdd = false; editingItem = null },
            )
        }
        if (showAddGiftPreference || editingGiftPreference != null) {
            PreferenceDialog(
                initial = editingGiftPreference,
                sections = PreferenceSection.GIFTS_TAB,
                onConfirm = { form ->
                    editingGiftPreference?.let { viewModel.updateGiftPreference(it, form) }
                        ?: viewModel.createGiftPreference(form)
                    showAddGiftPreference = false
                    editingGiftPreference = null
                },
                onDismiss = { showAddGiftPreference = false; editingGiftPreference = null },
            )
        }
    }
}

// ---------------------------------------------------------------------------
// Preferences
// ---------------------------------------------------------------------------

@Composable
internal fun PreferenceDialog(
    initial: Preference?,
    // Restricts the category dropdown (and the default selected category) to
    // these sections — e.g. the Gifts tab's dialog only offers
    // jewelry/giftPreferences/giftAvoid, so a shopping-focused "Add
    // Preference" can't accidentally create a food preference that then
    // hides in the Overview tab's list instead (web's PreferenceDialog
    // `sections` prop). Defaults to every section.
    sections: Set<String> = PreferenceSection.OVERVIEW_TAB + PreferenceSection.GIFTS_TAB,
    onConfirm: (PreferenceFormData) -> Unit,
    onDismiss: () -> Unit,
) {
    val availableCategories = remember(sections) { PreferenceCategory.CONFIG.filter { it.section in sections } }
    val defaultCategory = availableCategories.firstOrNull()?.category ?: PreferenceCategory.FOOD
    val isEditing = initial != null
    var category by remember(initial) { mutableStateOf(initial?.category ?: defaultCategory) }
    var key by remember(initial) { mutableStateOf(initial?.key ?: "") }
    var value by remember(initial) { mutableStateOf(initial?.value ?: "") }
    var notes by remember(initial) { mutableStateOf(initial?.notes ?: "") }
    var sensitivity by remember(initial) { mutableStateOf(initial?.sensitivity ?: PreferenceSensitivities.NORMAL) }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(if (isEditing) R.string.preferences_edit else R.string.preferences_new)) },
        text = {
            Column(
                verticalArrangement = Arrangement.spacedBy(8.dp),
                modifier = Modifier.verticalScroll(rememberScrollState()),
            ) {
                CategoryField(category = category, categories = availableCategories, onCategoryChange = { category = it })
                OutlinedTextField(
                    value = key, onValueChange = { key = it },
                    label = { Text(stringResource(R.string.preferences_key)) }, singleLine = true,
                )
                if (key.isBlank()) {
                    PreferenceCategory.keySuggestionsFor(category).forEach { suggestion ->
                        TextButton(onClick = { key = suggestion }, modifier = Modifier.fillMaxWidth()) {
                            Text(stringResource(preferenceKeyLabelRes(suggestion)))
                        }
                    }
                }
                OutlinedTextField(
                    value = value, onValueChange = { value = it },
                    label = { Text(stringResource(R.string.preferences_value)) }, singleLine = true,
                )
                OutlinedTextField(
                    value = notes, onValueChange = { notes = it },
                    label = { Text(stringResource(R.string.preferences_notes)) },
                )
                SensitivityField(sensitivity = sensitivity, onSensitivityChange = { sensitivity = it })
            }
        },
        confirmButton = {
            TextButton(
                onClick = {
                    onConfirm(
                        PreferenceFormData(
                            category = category,
                            key = key,
                            value = value,
                            notes = notes,
                            sensitivity = sensitivity,
                        ),
                    )
                },
                enabled = category.isNotBlank() && value.isNotBlank(),
            ) { Text(stringResource(if (isEditing) R.string.action_save else R.string.action_create)) }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) }
        },
    )
}

@Composable
private fun CategoryField(
    category: String,
    categories: List<PreferenceCategoryConfig>,
    onCategoryChange: (String) -> Unit,
) {
    var menu by remember { mutableStateOf(false) }
    Box {
        OutlinedButton(onClick = { menu = true }, modifier = Modifier.fillMaxWidth().testTag("preference-category")) {
            Text(stringResource(preferenceCategoryLabelRes(category)))
        }
        DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
            // Grouped by section (Food & Drink, Media, Jewelry & Style, ...),
            // mirroring web's ListSubheader-grouped select. `categories` is
            // already contiguous by section — PreferenceCategory.CONFIG is
            // declared in section order and filtering preserves that.
            var lastSection: String? = null
            categories.forEach { cfg ->
                if (cfg.section != lastSection) {
                    lastSection = cfg.section
                    Text(
                        text = stringResource(preferenceSectionLabelRes(cfg.section)),
                        style = MaterialTheme.typography.labelMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(horizontal = 12.dp, vertical = 6.dp),
                    )
                }
                DropdownMenuItem(
                    text = { Text(stringResource(preferenceCategoryLabelRes(cfg.category))) },
                    onClick = {
                        menu = false
                        onCategoryChange(cfg.category)
                    },
                )
            }
        }
    }
}

@Composable
private fun SensitivityField(sensitivity: String, onSensitivityChange: (String) -> Unit) {
    var menu by remember { mutableStateOf(false) }
    Box {
        OutlinedButton(onClick = { menu = true }, modifier = Modifier.fillMaxWidth().testTag("preference-sensitivity")) {
            Text(stringResource(preferenceSensitivityLabelRes(sensitivity)))
        }
        DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
            PreferenceSensitivities.ALL.forEach { token ->
                DropdownMenuItem(
                    text = { Text(stringResource(preferenceSensitivityLabelRes(token))) },
                    onClick = {
                        menu = false
                        onSensitivityChange(token)
                    },
                )
            }
        }
    }
}

@androidx.annotation.StringRes
private fun preferenceCategoryLabelRes(category: String): Int = when (category) {
    "food" -> R.string.preferences_category_food
    "drink" -> R.string.preferences_category_drink
    "media" -> R.string.preferences_category_media
    "media_movie" -> R.string.preferences_category_media_movie
    "media_tv" -> R.string.preferences_category_media_tv
    "media_game" -> R.string.preferences_category_media_game
    "media_podcast" -> R.string.preferences_category_media_podcast
    "media_music_artist" -> R.string.preferences_category_media_music_artist
    "media_music_album" -> R.string.preferences_category_media_music_album
    "media_music_genre" -> R.string.preferences_category_media_music_genre
    "media_music_song" -> R.string.preferences_category_media_music_song
    "media_book_author" -> R.string.preferences_category_media_book_author
    "media_book_series" -> R.string.preferences_category_media_book_series
    "media_book_title" -> R.string.preferences_category_media_book_title
    "jewelry_metal" -> R.string.preferences_category_jewelry_metal
    "jewelry_stone" -> R.string.preferences_category_jewelry_stone
    "jewelry_style" -> R.string.preferences_category_jewelry_style
    "jewelry_type" -> R.string.preferences_category_jewelry_type
    "flowers" -> R.string.preferences_category_flowers
    "color" -> R.string.preferences_category_color
    "hobby" -> R.string.preferences_category_hobby
    "fragrance" -> R.string.preferences_category_fragrance
    "cause" -> R.string.preferences_category_cause
    "dislike" -> R.string.preferences_category_dislike
    else -> R.string.preferences_category
}

@androidx.annotation.StringRes
private fun preferenceSensitivityLabelRes(sensitivity: String): Int = when (sensitivity) {
    PreferenceSensitivities.PRIVATE -> R.string.relationships_sensitivity_private
    PreferenceSensitivities.SECRET -> R.string.relationships_sensitivity_secret
    else -> R.string.relationships_sensitivity_normal
}

@androidx.annotation.StringRes
private fun preferenceKeyLabelRes(key: String): Int = when (key) {
    "favorite" -> R.string.preferences_key_favorite
    "like" -> R.string.preferences_key_like
    "dislike" -> R.string.preferences_key_dislike
    "allergy" -> R.string.preferences_key_allergy
    "show" -> R.string.preferences_key_show
    "movie" -> R.string.preferences_key_movie
    "music" -> R.string.preferences_key_music
    "shirt" -> R.string.preferences_key_shirt
    "pants" -> R.string.preferences_key_pants
    "dress" -> R.string.preferences_key_dress
    "skirt" -> R.string.preferences_key_skirt
    "undergarments" -> R.string.preferences_key_undergarments
    "outerwear" -> R.string.preferences_key_outerwear
    "shoe" -> R.string.preferences_key_shoe
    "hat" -> R.string.preferences_key_hat
    "glove" -> R.string.preferences_key_glove
    "belt" -> R.string.preferences_key_belt
    "ring" -> R.string.preferences_key_ring
    "socks" -> R.string.preferences_key_socks
    else -> R.string.preferences_key
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PreferencesScreen(
    onBack: () -> Unit,
    viewModel: PreferencesViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    var showAdd by remember { mutableStateOf(false) }
    var editingItem by remember { mutableStateOf<Preference?>(null) }

    EntityListScaffold(
        title = stringResource(R.string.preferences_title),
        addLabel = stringResource(R.string.preferences_new),
        uiState = state,
        onAdd = { showAdd = true },
        onItemClick = { id -> viewModel.findById(id)?.let { editingItem = it } },
        onDelete = viewModel::delete,
        onErrorShown = viewModel::onErrorShown,
        onBack = onBack,
        sectionLabel = { section -> stringResource(preferenceSectionLabelRes(section)) },
    ) {
        // Clothing sizes and the gift-shopping-relevant categories (jewelry/
        // flowers/color/fragrance/cause/gift-avoid) live in the Gifts screen
        // now (web's Gifts tab, "check this right before buying") — see
        // GiftsScreen. This dialog is scoped to Overview's own sections so it
        // can't create a category that would then only show up there instead.
        if (showAdd || editingItem != null) {
            PreferenceDialog(
                initial = editingItem,
                sections = PreferenceSection.OVERVIEW_TAB,
                onConfirm = { form ->
                    editingItem?.let { viewModel.update(it, form) }
                        ?: viewModel.create(form)
                    showAdd = false
                    editingItem = null
                },
                onDismiss = { showAdd = false; editingItem = null },
            )
        }
    }
}

@androidx.annotation.StringRes
private fun preferenceSectionLabelRes(section: String): Int = when (section) {
    PreferenceSection.FOOD_DRINK -> R.string.preferences_section_food_drink
    PreferenceSection.MEDIA -> R.string.preferences_section_media
    PreferenceSection.HOBBY -> R.string.preferences_section_hobby
    PreferenceSection.JEWELRY -> R.string.preferences_section_jewelry
    PreferenceSection.GIFT_PREFERENCES -> R.string.preferences_section_gift_preferences
    PreferenceSection.GIFT_AVOID -> R.string.preferences_section_gift_avoid
    else -> R.string.preferences_section_other
}

/** Web's ClothingSizesPanel equivalent — inline add/edit/delete of `clothing_size` preferences. */
@Composable
private fun ClothingSizesPanel(
    items: List<Preference>,
    newKey: String,
    newValue: String,
    editingId: String?,
    onNewKeyChange: (String) -> Unit,
    onNewValueChange: (String) -> Unit,
    onAdd: (String, String) -> Unit,
    onStartEdit: (Preference) -> Unit,
    onEditConfirm: (Preference, String, String) -> Unit,
    onEditCancel: () -> Unit,
    onDelete: (String) -> Unit,
) {
    Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp)) {
        Text(
            text = stringResource(R.string.preferences_clothing_sizes),
            style = MaterialTheme.typography.titleSmall,
            color = MaterialTheme.colorScheme.primary,
        )
        if (items.isEmpty()) {
            Text(
                text = stringResource(R.string.preferences_clothing_empty),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(vertical = 4.dp),
            )
        }
        items.forEach { item ->
            if (item.id == editingId) {
                var editKey by remember(item.id) { mutableStateOf(item.key ?: "") }
                var editValue by remember(item.id) { mutableStateOf(item.value) }
                Column(modifier = Modifier.fillMaxWidth().padding(vertical = 2.dp)) {
                    OutlinedTextField(
                        value = editKey, onValueChange = { editKey = it },
                        label = { Text(stringResource(R.string.gifts_clothing_type)) },
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth(),
                    )
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        OutlinedTextField(
                            value = editValue, onValueChange = { editValue = it },
                            singleLine = true,
                            modifier = Modifier.weight(1f),
                        )
                        TextButton(onClick = { onEditConfirm(item, editKey, editValue) }) { Text(stringResource(R.string.action_save)) }
                        TextButton(onClick = onEditCancel) { Text(stringResource(R.string.action_cancel)) }
                    }
                }
            } else {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    modifier = Modifier.fillMaxWidth().padding(vertical = 2.dp),
                ) {
                    Text(
                        text = item.key?.let { "${stringResource(preferenceKeyLabelRes(it))}: ${item.value}" } ?: item.value,
                        style = MaterialTheme.typography.bodyLarge,
                        modifier = Modifier.weight(1f),
                    )
                    TextButton(onClick = { onStartEdit(item) }) { Text(stringResource(R.string.action_edit)) }
                    IconButton(onClick = { onDelete(item.id) }) {
                        Icon(Icons.Outlined.Delete, contentDescription = stringResource(R.string.action_delete))
                    }
                }
            }
        }
        OutlinedTextField(
            value = newKey, onValueChange = onNewKeyChange,
            label = { Text(stringResource(R.string.gifts_clothing_type)) },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
        if (newKey.isBlank()) {
            Row(modifier = Modifier.fillMaxWidth()) {
                PreferenceCategory.CLOTHING_TYPE_SUGGESTIONS.take(4).forEach { suggestion ->
                    TextButton(onClick = { onNewKeyChange(suggestion) }) {
                        Text(stringResource(preferenceKeyLabelRes(suggestion)))
                    }
                }
            }
        }
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedTextField(
                value = newValue, onValueChange = onNewValueChange,
                label = { Text(stringResource(R.string.preferences_clothing_hint)) },
                singleLine = true,
                modifier = Modifier.weight(1f),
            )
            TextButton(onClick = { onAdd(newKey, newValue) }, enabled = newValue.isNotBlank()) {
                Text(stringResource(R.string.preferences_clothing_add))
            }
        }
    }
}

/**
 * Web's Gifts-tab jewelry/flowers/color/fragrance/cause/gift-avoid panel —
 * shopping-relevant preferences grouped by section, each row tappable to
 * edit. Adding/editing goes through the shared PreferenceDialog, scoped to
 * PreferenceSection.GIFTS_TAB so it can't create a food/media/hobby
 * preference that would then only show up in PreferencesScreen instead.
 */
@Composable
private fun GiftPreferencesPanel(
    items: List<Preference>,
    onAdd: () -> Unit,
    onEdit: (Preference) -> Unit,
    onDelete: (String) -> Unit,
) {
    Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp)) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween,
            modifier = Modifier.fillMaxWidth(),
        ) {
            Text(
                text = stringResource(R.string.gifts_preferences_heading),
                style = MaterialTheme.typography.titleSmall,
                color = MaterialTheme.colorScheme.primary,
            )
            TextButton(onClick = onAdd) { Text(stringResource(R.string.preferences_new)) }
        }
        if (items.isEmpty()) {
            Text(
                text = stringResource(R.string.entities_empty),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(vertical = 4.dp),
            )
        }
        PreferenceSection.GIFTS_TAB_ORDERED.forEach { section ->
            val sectionItems = items.filter { PreferenceCategory.sectionOf(it.category) == section }
            if (sectionItems.isNotEmpty()) {
                Text(
                    text = stringResource(preferenceSectionLabelRes(section)),
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(top = 4.dp),
                )
                sectionItems.forEach { item ->
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(8.dp),
                        modifier = Modifier.fillMaxWidth().clickable { onEdit(item) }.padding(vertical = 4.dp),
                    ) {
                        Column(modifier = Modifier.weight(1f)) {
                            Text(text = preferenceLabel(item), style = MaterialTheme.typography.bodyLarge)
                        }
                        IconButton(onClick = { onDelete(item.id) }) {
                            Icon(Icons.Outlined.Delete, contentDescription = stringResource(R.string.action_delete))
                        }
                    }
                }
            }
        }
    }
}

// ---------------------------------------------------------------------------
// Conversation agenda
// ---------------------------------------------------------------------------

@Composable
internal fun AgendaDialog(
    initial: ConversationAgenda?,
    onConfirm: (content: String, referenceUrl: String?) -> Unit,
    onDismiss: () -> Unit,
) {
    var content by remember(initial) { mutableStateOf(initial?.content ?: "") }
    var referenceUrl by remember(initial) { mutableStateOf(initial?.referenceUrl ?: "") }
    val isEditing = initial != null
    val normalizedUrl = normalizeHttpUrl(referenceUrl)
    val urlValid = referenceUrl.isBlank() || normalizedUrl != null

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(if (isEditing) R.string.agenda_edit else R.string.agenda_new)) },
        text = {
            Column(
                verticalArrangement = Arrangement.spacedBy(8.dp),
                modifier = Modifier.verticalScroll(rememberScrollState()),
            ) {
                OutlinedTextField(
                    value = content, onValueChange = { content = it },
                    label = { Text(stringResource(R.string.agenda_content)) }, singleLine = true,
                )
                OutlinedTextField(
                    value = referenceUrl, onValueChange = { referenceUrl = it },
                    label = { Text(stringResource(R.string.agenda_reference_url)) }, singleLine = true,
                    isError = !urlValid,
                    supportingText = if (urlValid) null else { { Text(stringResource(R.string.gifts_url_invalid)) } },
                )
            }
        },
        confirmButton = {
            TextButton(
                onClick = { onConfirm(content, normalizedUrl) },
                enabled = content.isNotBlank() && urlValid,
            ) { Text(stringResource(if (isEditing) R.string.action_save else R.string.action_create)) }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) }
        },
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ConversationAgendaScreen(
    onBack: () -> Unit,
    viewModel: ConversationAgendaViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val activities by viewModel.activities.collectAsStateWithLifecycle()
    val discussingId by viewModel.discussingId.collectAsStateWithLifecycle()
    var showAdd by remember { mutableStateOf(false) }
    var editingItem by remember { mutableStateOf<ConversationAgenda?>(null) }
    var pendingDiscussId by remember { mutableStateOf<String?>(null) }

    EntityListScaffold(
        title = stringResource(R.string.agenda_title),
        addLabel = stringResource(R.string.agenda_new),
        uiState = state,
        onAdd = { showAdd = true },
        onItemClick = { id -> viewModel.findById(id)?.let { editingItem = it } },
        onDelete = viewModel::delete,
        onErrorShown = viewModel::onErrorShown,
        onBack = onBack,
        sectionLabel = { section ->
            if (section == "discussed") {
                stringResource(R.string.agenda_section_discussed)
            } else {
                stringResource(R.string.agenda_section_open)
            }
        },
        extraAction = { item ->
            val agenda = viewModel.findById(item.id)
            if (agenda != null && agenda.discussedAt == null) {
                IconButton(onClick = { pendingDiscussId = item.id }) {
                    Icon(
                        Icons.Outlined.Done,
                        contentDescription = stringResource(R.string.agenda_mark_discussed),
                        tint = MaterialTheme.colorScheme.primary,
                    )
                }
            }
        },
    ) {
        if (showAdd || editingItem != null) {
            AgendaDialog(
                initial = editingItem,
                onConfirm = { content, referenceUrl ->
                    editingItem?.let { viewModel.update(it, content, referenceUrl) }
                        ?: viewModel.create(content, referenceUrl)
                    showAdd = false
                    editingItem = null
                },
                onDismiss = { showAdd = false; editingItem = null },
            )
        }
        pendingDiscussId?.let { id ->
            val item = viewModel.findById(id)
            if (item != null) {
                MarkDiscussedDialog(
                    item = item,
                    activities = activities,
                    confirming = discussingId == id,
                    onConfirm = { activityId -> viewModel.markDiscussed(id, activityId) },
                    onDismiss = {
                        viewModel.clearDiscussing()
                        pendingDiscussId = null
                    },
                )
                // Keep the dialog open (button disabled) while the PATCH is in
                // flight, and close it once it resolves — so the in-flight
                // state is real, not dead (review-pass fix).
                LaunchedEffect(discussingId) {
                    if (discussingId == null) pendingDiscussId = null
                }
            }
        }
    }
}

/** Web's MarkDiscussedDialog — marks an item discussed, optionally linked to an activity. */
@Composable
internal fun MarkDiscussedDialog(
    item: ConversationAgenda,
    activities: List<Activity>,
    confirming: Boolean,
    onConfirm: (Int?) -> Unit,
    onDismiss: () -> Unit,
) {
    var activityId by remember(item.id) { mutableStateOf<Int?>(null) }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.agenda_mark_discussed)) },
        text = {
            Column(
                verticalArrangement = Arrangement.spacedBy(8.dp),
                modifier = Modifier.verticalScroll(rememberScrollState()),
            ) {
                Text(
                    text = item.content,
                    style = MaterialTheme.typography.bodyLarge,
                )
                if (activities.isNotEmpty()) {
                    SelectField(
                        label = stringResource(R.string.agenda_discussed_link_activity),
                        value = activityId?.toString(),
                        placeholder = stringResource(R.string.gifts_none),
                        options = activities.map { it.id.toString() },
                        optionLabel = { id -> activities.firstOrNull { a -> a.id.toString() == id }?.title ?: id },
                        onSelect = { activityId = it?.toIntOrNull() },
                    )
                }
            }
        },
        confirmButton = {
            TextButton(
                onClick = { onConfirm(activityId) },
                enabled = !confirming,
            ) { Text(stringResource(R.string.agenda_discussed_button)) }
        },
        dismissButton = {
            TextButton(onClick = onDismiss, enabled = !confirming) { Text(stringResource(R.string.action_cancel)) }
        },
    )
}
