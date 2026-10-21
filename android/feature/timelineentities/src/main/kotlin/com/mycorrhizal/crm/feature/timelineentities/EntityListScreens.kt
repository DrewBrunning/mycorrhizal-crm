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
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.CheckCircle
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.Done
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material.icons.outlined.ExpandLess
import androidx.compose.material.icons.outlined.ExpandMore
import androidx.compose.material.icons.outlined.OpenInNew
import androidx.activity.compose.BackHandler
import androidx.compose.material3.AlertDialog
import com.mycorrhizal.crm.ui.components.DropdownOption
import com.mycorrhizal.crm.ui.components.LabelledDropdown
import com.mycorrhizal.crm.ui.components.FormScaffold
import androidx.compose.material3.SuggestionChip
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.material3.Card
import androidx.compose.material3.Checkbox
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
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
import androidx.compose.runtime.saveable.rememberSaveable
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
import com.mycorrhizal.crm.model.network.PreferenceLevels
import com.mycorrhizal.crm.model.network.PreferenceSensitivities
import com.mycorrhizal.crm.model.registry.LifeEventCategory
import com.mycorrhizal.crm.model.registry.LifeEventTypes
import com.mycorrhizal.crm.model.registry.PreferenceCategory
import com.mycorrhizal.crm.model.registry.PreferenceCategoryConfig
import com.mycorrhizal.crm.model.registry.PreferenceSection
import com.mycorrhizal.crm.model.util.DateFieldMode
import com.mycorrhizal.crm.model.util.DateFieldValue
import com.mycorrhizal.crm.model.util.Validators
import com.mycorrhizal.crm.model.util.parsePartialDateForEdit
import com.mycorrhizal.crm.ui.components.BrandFab
import com.mycorrhizal.crm.ui.components.DateField
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.components.RefreshableContent
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.WriteConflictDialog
import com.mycorrhizal.crm.ui.components.humanizeToken
import com.mycorrhizal.crm.ui.components.lifeEventTypeLabel
import com.mycorrhizal.crm.ui.components.lifeEventTypeLabelText
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
    emptyTitle: String,
    emptyBody: String?,
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
    // #386: an optional replacement for the default row body (label + url). The
    // trailing actions (extraAction, delete-with-confirm) are still rendered by
    // the scaffold, so a caller can supply a rich row (e.g. preferences'
    // category/key/value/notes/sensitivity) without losing the shared delete
    // flow. The composable is invoked inside the row's main content box.
    rowContent: (@Composable (EntityItem) -> Unit)? = null,
    // Optional in-layout content rendered above the list (e.g. gifts' clothing-sizes
    // panel). Deliberately separate from [dialog]: dialog content (AlertDialog) draws
    // in its own Popup/window and never affects layout, but this slot's content is a
    // normal composable and must be laid out in-flow with the scaffold, not stacked
    // as an overlapping sibling of it (that overlap was a real bug — review-pass fix).
    header: (@Composable () -> Unit)? = null,
    onRefresh: () -> Unit = {},
    // Issue #1773 review: a screen whose full-screen form replaces the list (gifts, life
    // events) hoists this so the scroll position survives the round trip to the form.
    listState: LazyListState = rememberLazyListState(),
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
            RefreshableContent(
                isRefreshing = uiState.isLoading && uiState.items.isNotEmpty(),
                onRefresh = onRefresh,
                modifier = Modifier.fillMaxSize(),
            ) {
                // #347: the header (GiftsScreen's clothing-sizes + gift-preferences
                // shopping-notes panel) lives inside the LazyColumn as a leading
                // item so it scrolls with the list — a tall expanded panel can no
                // longer push rows below the fold of a non-scrollable layout. Same
                // class of fix as ContactListScreen's SearchNotesActivitiesSection,
                // which is also a LazyColumn item rather than a fixed sibling above
                // it. Every state (loading/empty/error/populated) is an item too,
                // so the header stays reachable regardless of list contents.
                val uriHandler = LocalUriHandler.current
                LazyColumn(state = listState, modifier = Modifier.fillMaxSize().testTag("entity-list")) {
                    if (header != null) {
                        item(key = "scaffold-header") { header() }
                    }
                    when {
                        uiState.isLoading && uiState.items.isEmpty() -> item {
                            LoadingSkeleton()
                        }
                        uiState.items.isEmpty() && errorMessage == null -> item {
                            EmptyState(
                                title = emptyTitle,
                                body = emptyBody,
                                actionLabel = addLabel,
                                onAction = onAdd,
                            )
                        }
                        uiState.items.isEmpty() && errorMessage != null -> item {
                            Box(modifier = Modifier.fillMaxWidth().padding(24.dp)) {
                                Text(
                                    text = errorMessage,
                                    color = MaterialTheme.colorScheme.error,
                                    modifier = Modifier.align(Alignment.Center),
                                )
                            }
                        }
                        else -> {
                            // M18: flatten section headers into the row stream so the
                            // generic scaffold can render grouped lists (preferences,
                            // agenda) without per-entity list implementations.
                            val rows = sectionRows(uiState.items, sectionLabel != null)
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
                                            if (rowContent != null) {
                                                Box(modifier = Modifier.weight(1f)) {
                                                    rowContent(item)
                                                }
                                            } else {
                                                Column(modifier = Modifier.weight(1f)) {
                                                    Text(
                                                        text = item.displayLabel(),
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
                                                // #205: the row-action label carries
                                                // the item's label so TalkBack doesn't
                                                // read a bare "Delete" on every row.
                                                Icon(
                                                    Icons.Outlined.Delete,
                                                    contentDescription = stringResource(R.string.entities_delete_named, item.displayLabel()),
                                                )
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
        val label = uiState.items.firstOrNull { it.id == id }?.displayLabel()
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

/**
 * Issue #1773: the shared chrome of the entity create/edit forms that are full screen
 * (gift, life event) -- the same [FormScaffold] note/activity/reminder use: a back
 * arrow, a title, and a pinned Save/Create button that says what enables it while
 * disabled. System back and the top-bar arrow both leave the form, discarding what was
 * typed -- exactly what Cancel / tapping outside did in the dialogs these replaced.
 */
@Composable
internal fun EntityFormScreen(
    title: String,
    saveLabel: String,
    saveEnabled: Boolean,
    saveDisabledHint: String?,
    onSave: () -> Unit,
    onBack: () -> Unit,
    errorMessage: String? = null,
    onErrorShown: () -> Unit = {},
    content: @Composable ColumnScope.() -> Unit,
) {
    BackHandler(onBack = onBack)
    val snackbarHostState = remember { SnackbarHostState() }
    // A form that stays open on a failed save (mark discussed, #1723) replaces the list,
    // so the list's snackbar is not composed: the form shows the error itself.
    errorMessage?.let { message ->
        LaunchedEffect(message) {
            snackbarHostState.showSnackbar(message)
            onErrorShown()
        }
    }
    FormScaffold(
        title = title,
        onBack = onBack,
        saveLabel = saveLabel,
        isSaving = false,
        onSave = onSave,
        snackbarHostState = snackbarHostState,
        saveEnabled = saveEnabled,
        saveDisabledHint = saveDisabledHint,
    ) { padding ->
        Column(
            modifier = Modifier
                .padding(padding)
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
            content = content,
        )
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
 * Issue #1773: a full-screen form (it has a contact search and a dozen controls, so
 * it is past the dialog threshold in docs/android-app.md "Design system").
 * `initial == null` is add mode; a non-null [LifeEvent] pre-fills every field
 * this form models and switches the form to edit mode (title/button label,
 * and [onConfirm] is expected to route to update() instead of create() —
 * that branch lives in the caller, which is the one that knows whether it has
 * an `editingItem`).
 */
@Composable
internal fun LifeEventForm(
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
    var category by rememberSaveable(initial?.id) {
        mutableStateOf(
            initial?.category
                ?.takeIf { it in LifeEventCategory.ALL }
                ?: (if (initial != null) LIFE_EVENT_UNCATEGORIZED else ""),
        )
    }
    var type by rememberSaveable(initial?.id) { mutableStateOf(initial?.type ?: "") }
    var customType by rememberSaveable(initial?.id) {
        mutableStateOf(
            initial?.type != null &&
                category in LifeEventCategory.ALL &&
                initial.type !in LifeEventTypes.forCategory(category),
        )
    }
    var description by rememberSaveable(initial?.id) { mutableStateOf(initial?.description ?: "") }
    var year by rememberSaveable(initial?.id) { mutableStateOf(initial?.date?.year?.toString() ?: "") }
    var month by rememberSaveable(initial?.id) { mutableStateOf(initial?.date?.month?.toString() ?: "") }
    var day by rememberSaveable(initial?.id) { mutableStateOf(initial?.date?.day?.toString() ?: "") }
    var remind by rememberSaveable(initial?.id) { mutableStateOf(initial?.remind == true) }

    val canRemind = month.toIntOrNull() != null && day.toIntOrNull() != null
    val typeEnabled = category.isNotEmpty()
    val canSave = type.isNotBlank()
    // Keep year-only legacy values visible/round-trippable (partialFieldValue),
    // rather than PartialDate.formatForEdit's blank-for-year-only contract.
    val dateValue = DateFieldValue.partialFieldValue(
        PartialDate(
            year = year.toIntOrNull(),
            month = month.toIntOrNull(),
            day = day.toIntOrNull(),
        ),
    )

    val onSave: () -> Unit = {
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
    }

    EntityFormScreen(
        title = stringResource(if (isEditing) R.string.life_events_edit else R.string.life_events_new),
        saveLabel = stringResource(if (isEditing) R.string.action_save else R.string.action_create),
        saveEnabled = canSave,
        saveDisabledHint = stringResource(R.string.life_events_save_hint),
        onSave = onSave,
        onBack = onDismiss,
    ) {
        // Category select. Issue #1773: a labelled dropdown, not a bare value button.
        val categoryOptions = buildList {
            LifeEventCategory.ALL.forEach { token ->
                add(DropdownOption(token, stringResource(categoryLabelRes(token))))
            }
            if (category == LIFE_EVENT_UNCATEGORIZED) {
                add(DropdownOption(LIFE_EVENT_UNCATEGORIZED, stringResource(R.string.life_events_category_uncategorized)))
            }
        }
        LabelledDropdown(
            label = stringResource(R.string.life_events_category_label),
            valueText = categoryLabel(category),
            options = categoryOptions,
            onSelect = { token ->
                category = token
                // Picking a real category may orphan a custom type; fall back to the select.
                customType = token in LifeEventCategory.ALL &&
                    type.isNotBlank() && type !in LifeEventTypes.forCategory(token)
            },
            modifier = Modifier.testTag("life-event-category"),
        )

        // Type: disabled until a category is chosen; free text for
        // uncategorized; a category-scoped select with a custom escape
        // hatch for real categories.
        if (category == LIFE_EVENT_UNCATEGORIZED || customType) {
            OutlinedTextField(
                value = type, onValueChange = { type = it },
                label = { Text(stringResource(R.string.life_events_custom_type_label)) }, singleLine = true,
                enabled = typeEnabled,
                modifier = Modifier.fillMaxWidth(),
            )
        } else {
            val customTypeToken = "\u0000custom"
            val typeOptions = if (category in LifeEventCategory.ALL) {
                LifeEventTypes.forCategory(category).map { DropdownOption(it, lifeEventTypeLabel(it)) } +
                    DropdownOption(customTypeToken, stringResource(R.string.life_events_custom_type))
            } else {
                emptyList()
            }
            LabelledDropdown(
                label = stringResource(R.string.life_events_type_label),
                valueText = if (type.isBlank()) stringResource(R.string.life_events_select_category_first) else lifeEventTypeLabel(type),
                options = typeOptions,
                onSelect = { token ->
                    if (token == customTypeToken) {
                        customType = true
                    } else {
                        type = token
                        customType = false
                    }
                },
                enabled = typeEnabled,
                modifier = Modifier.testTag("life-event-type"),
            )
        }

        DateField(
            value = dateValue,
            onValueChange = { wire ->
                val parsed = parsePartialDateForEdit(wire)
                year = parsed.year?.toString() ?: ""
                month = parsed.month?.toString() ?: ""
                day = parsed.day?.toString() ?: ""
            },
            label = stringResource(R.string.life_events_date),
            mode = DateFieldMode.PARTIAL,
            allowClear = true,
            testTag = "life-event-date",
            modifier = Modifier.fillMaxWidth(),
        )

        OutlinedTextField(
            value = description, onValueChange = { description = it },
            label = { Text(stringResource(R.string.life_events_description)) }, singleLine = true,
            modifier = Modifier.fillMaxWidth(),
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
            modifier = Modifier.fillMaxWidth(),
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
    // Issue #1773 review: the full-screen form survives rotation and process death
    // (saved open flag + the edited item's id, not the item), and the list keeps its
    // scroll position while the form is up.
    var showAdd by rememberSaveable { mutableStateOf(false) }
    var editingId by rememberSaveable { mutableStateOf<String?>(null) }
    val editingItem = editingId?.let { id -> state.items.firstOrNull { it.id == id }?.let { viewModel.findById(id) } }
    val listState = rememberLazyListState()

    if (showAdd || editingItem != null) {
        LaunchedEffect(editingItem?.id) { viewModel.onDialogOpened(editingItem) }
        LifeEventForm(
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
                viewModel.onFormClosed()
                showAdd = false
                editingId = null
            },
            onDismiss = {
                viewModel.onFormClosed()
                showAdd = false
                editingId = null
            },
        )
    } else {
        EntityListScaffold(
            title = stringResource(R.string.life_events_title),
            addLabel = stringResource(R.string.life_events_new),
            emptyTitle = stringResource(R.string.life_events_empty),
            emptyBody = stringResource(R.string.life_events_empty_body),
            uiState = state,
            onAdd = { showAdd = true },
            onItemClick = { id -> if (viewModel.findById(id) != null) editingId = id },
            onDelete = viewModel::delete,
            onErrorShown = viewModel::onErrorShown,
            onBack = onBack,
            onRefresh = viewModel::load,
            listState = listState,
        ) {}
    }

    // The edit dialog has already closed by the time the 412 lands, so there is no form to
    // "keep editing" — the dialog only offers to reload the server's version.
    state.conflict?.let { conflict ->
        WriteConflictDialog(
            onLoadLatest = viewModel::onConflictLoadLatest,
            onKeep = viewModel::onConflictDismissed,
            messageRes = if (conflict == EntityConflict.EDIT) {
                R.string.conflict_message_edit_closed
            } else {
                R.string.conflict_message_delete
            },
            keepLabelRes = R.string.conflict_dismiss,
        )
    }
}

// ---------------------------------------------------------------------------
// Gifts
// ---------------------------------------------------------------------------

/**
 * Issue #1773: a full-screen form (ten fields plus pickers; it used to scroll inside a
 * near-full-height dialog).
 */
@Composable
internal fun GiftForm(
    initial: Gift?,
    lifeEvents: List<LifeEvent>,
    activities: List<Activity>,
    onConfirm: (GiftFormData) -> Unit,
    onDismiss: () -> Unit,
) {
    val isEditing = initial != null
    var status by rememberSaveable(initial?.id) { mutableStateOf(initial?.status ?: GiftStatuses.IDEA) }
    var description by rememberSaveable(initial?.id) { mutableStateOf(initial?.description ?: "") }
    var url by rememberSaveable(initial?.id) { mutableStateOf(initial?.url ?: "") }
    var notes by rememberSaveable(initial?.id) { mutableStateOf(initial?.notes ?: "") }
    var occasion by rememberSaveable(initial?.id) { mutableStateOf(initial?.occasion ?: "") }
    var date by rememberSaveable(initial?.id) { mutableStateOf(giftDateFromIso(initial?.date)) }
    var amount by rememberSaveable(initial?.id) { mutableStateOf(initial?.valueCents?.let(::centsToAmountText) ?: "") }
    var currency by rememberSaveable(initial?.id) { mutableStateOf(initial?.currency ?: "") }
    var lifeEventId by rememberSaveable(initial?.id) { mutableStateOf(initial?.lifeEventId) }
    var activityId by rememberSaveable(initial?.id) { mutableStateOf(initial?.activityId) }

    val parsedAmount = amount.toBigDecimalOrNull()
    val hasAmount = parsedAmount != null && parsedAmount >= BigDecimal.ZERO
    val hasCurrency = currency.isNotBlank()
    // The pair is enforced together (backend: validateGiftValueCurrency).
    val pairValid = (hasAmount == hasCurrency)
    val currencyLengthValid = currency.isBlank() || currency.length == 3
    val normalizedUrl = normalizeHttpUrl(url)
    val urlValid = url.isBlank() || normalizedUrl != null
    val canSave = description.isNotBlank() && pairValid && currencyLengthValid && urlValid

    val onSave: () -> Unit = {
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
    }
    val saveHint = stringResource(
        if (description.isBlank()) R.string.gifts_save_hint_description else R.string.form_save_hint_fix_fields,
    )

    EntityFormScreen(
        title = stringResource(if (isEditing) R.string.gifts_edit else R.string.gifts_new),
        saveLabel = stringResource(if (isEditing) R.string.action_save else R.string.action_create),
        saveEnabled = canSave,
        saveDisabledHint = saveHint,
        onSave = onSave,
        onBack = onDismiss,
    ) {
        StatusField(status = status, onStatusChange = { status = it })
        OutlinedTextField(
            value = description, onValueChange = { description = it },
            label = { Text(stringResource(R.string.gifts_description)) }, singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
        OutlinedTextField(
            value = url, onValueChange = { url = it },
            label = { Text(stringResource(R.string.gifts_url)) }, singleLine = true,
            isError = !urlValid,
            supportingText = if (urlValid) null else { { Text(stringResource(R.string.gifts_url_invalid)) } },
            modifier = Modifier.fillMaxWidth(),
        )
        OutlinedTextField(
            value = notes, onValueChange = { notes = it },
            label = { Text(stringResource(R.string.gifts_notes)) }, singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
        OutlinedTextField(
            value = occasion, onValueChange = { occasion = it },
            label = { Text(stringResource(R.string.gifts_occasion)) }, singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
        DateField(
            value = date,
            onValueChange = { date = it },
            label = stringResource(R.string.gifts_date),
            mode = DateFieldMode.DATE_ONLY,
            allowClear = true,
            testTag = "gift-date",
            modifier = Modifier.fillMaxWidth(),
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
            val lifeEventCtx = androidx.compose.ui.platform.LocalContext.current
            SelectField(
                label = stringResource(R.string.gifts_life_event),
                value = lifeEventId,
                placeholder = stringResource(R.string.gifts_none),
                options = lifeEvents.map { it.id },
                optionLabel = { id -> lifeEvents.firstOrNull { it.id == id }?.type?.let { lifeEventTypeLabelText(lifeEventCtx, it) } ?: id },
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
}

@Composable
private fun StatusField(status: String, onStatusChange: (String) -> Unit) {
    LabelledDropdown(
        label = stringResource(R.string.gifts_status_label),
        valueText = statusLabel(status),
        options = GiftStatuses.ALL.map { DropdownOption(it, statusLabel(it)) },
        onSelect = onStatusChange,
        modifier = Modifier.testTag("gift-status"),
    )
}

@Composable
private fun statusLabel(status: String): String = when (status) {
    GiftStatuses.IDEA -> stringResource(R.string.gifts_status_idea)
    GiftStatuses.PURCHASED -> stringResource(R.string.gifts_status_purchased)
    GiftStatuses.GIVEN -> stringResource(R.string.gifts_status_given)
    GiftStatuses.RECEIVED -> stringResource(R.string.gifts_status_received)
    else -> status
}

/**
 * A labelled dropdown with a leading "none" choice (issue #1773: it used to be a
 * caption plus a bare value button with no arrow).
 */
@Composable
private fun SelectField(
    label: String,
    value: String?,
    placeholder: String,
    options: List<String>,
    optionLabel: (String) -> String,
    onSelect: (String?) -> Unit,
) {
    LabelledDropdown(
        label = label,
        valueText = value?.let(optionLabel) ?: placeholder,
        options = listOf(DropdownOption<String?>(null, placeholder)) +
            options.map { DropdownOption<String?>(it, optionLabel(it)) },
        onSelect = onSelect,
    )
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
    // Issue #1773 review: as LifeEventsScreen, the full-screen form survives rotation
    // and process death, and the list keeps its scroll position under it.
    var showAdd by rememberSaveable { mutableStateOf(false) }
    var editingId by rememberSaveable { mutableStateOf<String?>(null) }
    val editingItem = editingId?.let { id -> state.items.firstOrNull { it.id == id }?.let { viewModel.findById(id) } }
    val listState = rememberLazyListState()
    var editingClothingItem by remember { mutableStateOf<Preference?>(null) }
    var newClothingKey by rememberSaveable { mutableStateOf("") }
    var newClothingValue by rememberSaveable { mutableStateOf("") }
    // Hoisted (and saved) so the panel stays open when Back returns from a preference form.
    var shoppingNotesExpanded by rememberSaveable { mutableStateOf(false) }
    var showAddGiftPreference by rememberSaveable { mutableStateOf(false) }
    var editingGiftPreferenceId by rememberSaveable { mutableStateOf<String?>(null) }
    val editingGiftPreference = editingGiftPreferenceId?.let { id -> giftPreferences.firstOrNull { it.id == id } }

    if (showAdd || editingItem != null) {
        GiftForm(
            initial = editingItem,
            lifeEvents = lifeEvents,
            activities = activities,
            onConfirm = { form ->
                editingItem?.let { viewModel.update(it, form) }
                    ?: viewModel.create(form)
                showAdd = false
                editingId = null
            },
            onDismiss = { showAdd = false; editingId = null },
        )
    } else if (showAddGiftPreference || editingGiftPreference != null) {
        PreferenceForm(
            initial = editingGiftPreference,
            sections = PreferenceSection.GIFTS_TAB,
            onConfirm = { form ->
                editingGiftPreference?.let { viewModel.updateGiftPreference(it, form) }
                    ?: viewModel.createGiftPreference(form)
                showAddGiftPreference = false
                editingGiftPreferenceId = null
            },
            onDismiss = { showAddGiftPreference = false; editingGiftPreferenceId = null },
        )
    } else {
        EntityListScaffold(
            title = stringResource(R.string.gifts_title),
            addLabel = stringResource(R.string.gifts_new),
            emptyTitle = stringResource(R.string.gifts_empty),
            emptyBody = stringResource(R.string.gifts_empty_body),
            uiState = state,
            onAdd = { showAdd = true },
            onItemClick = { id -> if (viewModel.findById(id) != null) editingId = id },
            onDelete = viewModel::delete,
            onErrorShown = viewModel::onErrorShown,
            onBack = onBack,
            onRefresh = viewModel::load,
            listState = listState,
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
            // overlapping it. Wrapped in GiftShoppingNotesSection's collapsible
            // card (Android testing feedback: a contact with several clothing
            // sizes + jewelry/flowers/etc. rows was pushing the actual gift-idea
            // list off-screen) — collapsed by default, tap to expand. Deliberately
            // kept as two separate panels rather than merged into one
            // PreferenceCategory-driven list: clothing sizes' fast inline
            // edit-in-place add flow (several sizes entered in one sitting) would
            // regress to a per-item modal if it went through the shared
            // PreferenceForm the way jewelry/flowers do.
            header = {
                if (!state.isLoading) {
                    GiftShoppingNotesSection(
                        expanded = shoppingNotesExpanded,
                        onExpandedChange = { shoppingNotesExpanded = it },
                        clothingSizes = clothingSizes,
                        giftPreferences = giftPreferences,
                        newClothingKey = newClothingKey,
                        newClothingValue = newClothingValue,
                        editingClothingItem = editingClothingItem,
                        onNewClothingKeyChange = { newClothingKey = it },
                        onNewClothingValueChange = { newClothingValue = it },
                        onAddClothingSize = { key, value ->
                            viewModel.createClothingSize(key, value)
                            newClothingKey = ""
                            newClothingValue = ""
                        },
                        onStartEditClothingSize = { item -> editingClothingItem = item },
                        onEditConfirmClothingSize = { item, key, value ->
                            viewModel.updateClothingSize(item, key, value)
                            editingClothingItem = null
                        },
                        onEditCancelClothingSize = { editingClothingItem = null },
                        onDeleteClothingSize = viewModel::deleteClothingSize,
                        onAddGiftPreference = { showAddGiftPreference = true },
                        onEditGiftPreference = { item -> editingGiftPreferenceId = item.id },
                        onDeleteGiftPreference = viewModel::deleteGiftPreference,
                    )
                }
            },
        ) {}
    }
}

// ---------------------------------------------------------------------------
// Preferences
// ---------------------------------------------------------------------------

/**
 * Issue #1773 (maintainer decision 2026-10-20): the preference create/edit form is full
 * screen -- it carries up to three fixed-list pickers (category, level, sensitivity) and
 * the "any picker means a full-screen form" rule has no dialog exception. Shared by the
 * Preferences screen and the Gifts screen's gift-preference panel. Its state is
 * `rememberSaveable`, so an open form and what was typed survive rotation and process death.
 */
@Composable
internal fun PreferenceForm(
    initial: Preference?,
    // Restricts the category dropdown (and the default selected category) to
    // these sections — e.g. the Gifts tab's form only offers
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
    var category by rememberSaveable(initial?.id) { mutableStateOf(initial?.category ?: defaultCategory) }
    var key by rememberSaveable(initial?.id) { mutableStateOf(initial?.key ?: "") }
    var value by rememberSaveable(initial?.id) { mutableStateOf(initial?.value ?: "") }
    var level by rememberSaveable(initial?.id) { mutableStateOf(initial?.level) }
    var notes by rememberSaveable(initial?.id) { mutableStateOf(initial?.notes ?: "") }
    var sensitivity by rememberSaveable(initial?.id) {
        mutableStateOf(initial?.sensitivity ?: PreferenceSensitivities.NORMAL)
    }

    EntityFormScreen(
        title = stringResource(if (isEditing) R.string.preferences_edit else R.string.preferences_new),
        saveLabel = stringResource(if (isEditing) R.string.action_save else R.string.action_create),
        saveEnabled = category.isNotBlank() && value.isNotBlank(),
        saveDisabledHint = stringResource(R.string.preferences_save_hint),
        onSave = {
            onConfirm(
                PreferenceFormData(
                    category = category,
                    key = key,
                    value = value,
                    // Only for hobby/skill categories; null otherwise.
                    level = level?.takeIf { PreferenceCategory.supportsLevelFor(category) },
                    notes = notes,
                    sensitivity = sensitivity,
                ),
            )
        },
        onBack = onDismiss,
    ) {
        CategoryField(
            category = category,
            categories = availableCategories,
            onCategoryChange = {
                category = it
                // A level only means something for hobby/skill
                // categories (issue #246) — clear it when switching
                // away rather than stranding it.
                if (!PreferenceCategory.supportsLevelFor(it)) level = null
            },
        )
        OutlinedTextField(
            value = key, onValueChange = { key = it },
            label = { Text(stringResource(R.string.preferences_key)) }, singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
        if (key.isBlank()) {
            KeySuggestions(
                suggestions = PreferenceCategory.keySuggestionsFor(category),
                onPick = { key = it },
            )
        }
        OutlinedTextField(
            value = value, onValueChange = { value = it },
            label = { Text(stringResource(R.string.preferences_value)) }, singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
        if (PreferenceCategory.supportsLevelFor(category)) {
            LevelField(level = level, onLevelChange = { level = it })
        }
        OutlinedTextField(
            value = notes, onValueChange = { notes = it },
            label = { Text(stringResource(R.string.preferences_notes)) },
            modifier = Modifier.fillMaxWidth(),
        )
        SensitivityField(sensitivity = sensitivity, onSensitivityChange = { sensitivity = it })
    }
}

@Composable
private fun CategoryField(
    category: String,
    categories: List<PreferenceCategoryConfig>,
    onCategoryChange: (String) -> Unit,
) {
    // Grouped by section (Food & Drink, Media, Jewelry & Style, ...), mirroring
    // web's ListSubheader-grouped select. `categories` is already contiguous by
    // section — PreferenceCategory.CONFIG is declared in section order and
    // filtering preserves that.
    var lastSection: String? = null
    val options = categories.map { cfg ->
        val header = if (cfg.section != lastSection) stringResource(preferenceSectionLabelRes(cfg.section)) else null
        lastSection = cfg.section
        DropdownOption(cfg.category, preferenceCategoryText(cfg.category), header)
    }
    LabelledDropdown(
        label = stringResource(R.string.preferences_category),
        valueText = preferenceCategoryText(category),
        options = options,
        onSelect = onCategoryChange,
        modifier = Modifier.testTag("preference-category"),
    )
}

/** Proficiency-level picker (issue #246), shown only for hobby/skill
 * categories. A leading "no level" entry clears a recorded level. */
@Composable
private fun LevelField(level: String?, onLevelChange: (String?) -> Unit) {
    LabelledDropdown(
        label = stringResource(R.string.preferences_level),
        valueText = stringResource(preferenceLevelLabelRes(level)),
        options = listOf(DropdownOption<String?>(null, stringResource(R.string.preferences_level_none))) +
            PreferenceLevels.ALL.map { DropdownOption<String?>(it, stringResource(preferenceLevelLabelRes(it))) },
        onSelect = onLevelChange,
        modifier = Modifier.testTag("preference-level"),
    )
}

@Composable
private fun SensitivityField(sensitivity: String, onSensitivityChange: (String) -> Unit) {
    LabelledDropdown(
        label = stringResource(R.string.relationships_sensitivity),
        valueText = stringResource(preferenceSensitivityLabelRes(sensitivity)),
        options = PreferenceSensitivities.ALL.map {
            DropdownOption(it, stringResource(preferenceSensitivityLabelRes(it)))
        },
        onSelect = onSensitivityChange,
        modifier = Modifier.testTag("preference-sensitivity"),
    )
}

/**
 * Issue #1773: the quick-fill kinds for an empty "Kind" field, as a labelled row of
 * suggestion chips ("Suggestions: Favorite, Dislike, Allergy") -- they used to be
 * full-width text buttons that read as static centred text. Tapping one fills the field.
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
internal fun KeySuggestions(suggestions: List<String>, onPick: (String) -> Unit) {
    if (suggestions.isEmpty()) return
    Column(
        verticalArrangement = Arrangement.spacedBy(4.dp),
        modifier = Modifier.testTag("preference-key-suggestions"),
    ) {
        Text(
            text = stringResource(R.string.preferences_key_suggestions),
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        FlowRow(
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            suggestions.forEach { suggestion ->
                val label = stringResource(preferenceKeyLabelRes(suggestion))
                SuggestionChip(
                    onClick = { onPick(suggestion) },
                    label = { Text(label) },
                )
            }
        }
    }
}

@androidx.annotation.StringRes
private fun preferenceCategoryLabelRes(category: String): Int? = when (category) {
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
    "dislike_activity" -> R.string.preferences_category_dislike_activity
    "fragrance" -> R.string.preferences_category_fragrance
    "cause" -> R.string.preferences_category_cause
    "dislike" -> R.string.preferences_category_dislike
    else -> null
}

@androidx.annotation.StringRes
private fun preferenceLevelLabelRes(level: String?): Int = when (level) {
    PreferenceLevels.HIGH -> R.string.preferences_level_high
    PreferenceLevels.MEDIUM -> R.string.preferences_level_medium
    PreferenceLevels.LOW -> R.string.preferences_level_low
    else -> R.string.preferences_level_none
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
    var showAdd by rememberSaveable { mutableStateOf(false) }
    var editingId by rememberSaveable { mutableStateOf<String?>(null) }
    val editingItem = editingId?.let { viewModel.findById(it) }
    // Hoisted above the form/list switch so Back returns to where the list was scrolled.
    val listState = rememberLazyListState()

    if (showAdd || editingItem != null) {
        PreferenceForm(
            initial = editingItem,
            sections = PreferenceSection.OVERVIEW_TAB,
            onConfirm = { form ->
                editingItem?.let { viewModel.update(it, form) }
                    ?: viewModel.create(form)
                showAdd = false
                editingId = null
            },
            onDismiss = { showAdd = false; editingId = null },
        )
        return
    }

    EntityListScaffold(
        title = stringResource(R.string.preferences_title),
        addLabel = stringResource(R.string.preferences_new),
        emptyTitle = stringResource(R.string.preferences_empty),
        emptyBody = stringResource(R.string.preferences_empty_body),
        uiState = state,
        onAdd = { showAdd = true },
        onItemClick = { id -> if (viewModel.findById(id) != null) editingId = id },
        onDelete = viewModel::delete,
        onErrorShown = viewModel::onErrorShown,
        onBack = onBack,
        onRefresh = viewModel::load,
        listState = listState,
        sectionLabel = { section -> stringResource(preferenceSectionLabelRes(section)) },
        // #386: rich preference rows (category + key + value + notes +
        // sensitivity) instead of the scaffold's flat label, matching web's
        // PreferenceList. Row lookup goes through findById — the same list
        // `uiState.items` was derived from, so it never misses.
        rowContent = { item ->
            viewModel.findById(item.id)?.let { PreferenceRow(it) }
        },
        // #386: explicit edit affordance next to the scaffold's delete, so the
        // edit path isn't only the row tap.
        extraAction = { item ->
            IconButton(onClick = { editingId = item.id }) {
                Icon(
                    Icons.Outlined.Edit,
                    contentDescription = stringResource(R.string.action_edit),
                )
            }
        },
    ) {}
}

@androidx.annotation.StringRes
private fun preferenceSectionLabelRes(section: String): Int = when (section) {
    PreferenceSection.FOOD_DRINK -> R.string.preferences_section_food_drink
    PreferenceSection.MEDIA -> R.string.preferences_section_media
    PreferenceSection.HOBBY -> R.string.preferences_section_hobby
    PreferenceSection.DISLIKES -> R.string.preferences_section_dislikes
    PreferenceSection.JEWELRY -> R.string.preferences_section_jewelry
    PreferenceSection.GIFT_PREFERENCES -> R.string.preferences_section_gift_preferences
    PreferenceSection.GIFT_AVOID -> R.string.preferences_section_gift_avoid
    else -> R.string.preferences_section_other
}

/**
 * #386: a single preferences-list row, mirroring web's PreferenceList item —
 * localized category + key labels on a first line, the value on the second,
 * and a secondary notes line when present. A non-normal sensitivity renders a
 * small badge (normal renders nothing, matching web's conditional chip).
 * Rendered through EntityListScaffold's [rowContent] slot so the shared
 * section headers and delete-confirm flow stay in the scaffold. `internal`
 * (not `private`) so PreferencesRowTest can drive it directly.
 */
@Composable
internal fun PreferenceRow(preference: Preference) {
    Column(modifier = Modifier.fillMaxWidth()) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Text(
                text = preferenceCategoryText(preference.category),
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.primary,
            )
            preference.key?.takeIf { it.isNotBlank() }?.let { key ->
                Text(
                    text = stringResource(preferenceKeyLabelRes(key)),
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            preference.level?.takeIf { it.isNotBlank() }?.let { level ->
                Text(
                    text = stringResource(preferenceLevelLabelRes(level)),
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            if (preference.sensitivity != PreferenceSensitivities.NORMAL) {
                SensitivityBadge(preference.sensitivity)
            }
        }
        Text(
            text = preference.value,
            style = MaterialTheme.typography.bodyLarge,
        )
        preference.notes?.takeIf { it.isNotBlank() }?.let { notes ->
            Text(
                text = notes,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

/** Small sensitivity pill (web's `height: 18` Chip) — only rendered for non-normal. */
@Composable
private fun SensitivityBadge(sensitivity: String) {
    val containerColor = when (sensitivity) {
        PreferenceSensitivities.SECRET -> MaterialTheme.colorScheme.errorContainer
        PreferenceSensitivities.PRIVATE -> MaterialTheme.colorScheme.tertiaryContainer
        else -> MaterialTheme.colorScheme.surfaceVariant
    }
    Surface(
        color = containerColor,
        shape = RoundedCornerShape(4.dp),
    ) {
        Text(
            text = stringResource(preferenceSensitivityLabelRes(sensitivity)),
            style = MaterialTheme.typography.labelSmall,
            modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.dp),
        )
    }
}

/**
 * GiftsScreen's "check this right before buying" section: clothing sizes
 * plus gift preferences (jewelry/flowers/color/fragrance/cause/gift-avoid),
 * collapsed behind one summary row by default so the two panels below don't
 * crowd the gift-idea list out. Collapse idiom matches ContactListScreen's
 * SearchNotesActivitiesSection. The header row itself never disappears (even
 * with zero items) — it's the only way to reach either panel's "add" flow.
 */
@Composable
private fun GiftShoppingNotesSection(
    expanded: Boolean,
    onExpandedChange: (Boolean) -> Unit,
    clothingSizes: List<Preference>,
    giftPreferences: List<Preference>,
    newClothingKey: String,
    newClothingValue: String,
    editingClothingItem: Preference?,
    onNewClothingKeyChange: (String) -> Unit,
    onNewClothingValueChange: (String) -> Unit,
    onAddClothingSize: (String, String) -> Unit,
    onStartEditClothingSize: (Preference) -> Unit,
    onEditConfirmClothingSize: (Preference, String, String) -> Unit,
    onEditCancelClothingSize: () -> Unit,
    onDeleteClothingSize: (String) -> Unit,
    onAddGiftPreference: () -> Unit,
    onEditGiftPreference: (Preference) -> Unit,
    onDeleteGiftPreference: (String) -> Unit,
) {
    val total = clothingSizes.size + giftPreferences.size

    Card(modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp)) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .clickable { onExpandedChange(!expanded) }
                .padding(horizontal = 16.dp, vertical = 12.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween,
        ) {
            Text(
                text = stringResource(R.string.gifts_shopping_notes_header, total),
                style = MaterialTheme.typography.titleSmall,
            )
            Icon(
                imageVector = if (expanded) Icons.Outlined.ExpandLess else Icons.Outlined.ExpandMore,
                contentDescription = stringResource(
                    if (expanded) R.string.a11y_state_expanded else R.string.a11y_state_collapsed,
                ),
            )
        }
        if (expanded) {
            Column(modifier = Modifier.padding(bottom = 8.dp)) {
                ClothingSizesPanel(
                    items = clothingSizes,
                    newKey = newClothingKey,
                    newValue = newClothingValue,
                    editingId = editingClothingItem?.id,
                    onNewKeyChange = onNewClothingKeyChange,
                    onNewValueChange = onNewClothingValueChange,
                    onAdd = onAddClothingSize,
                    onStartEdit = onStartEditClothingSize,
                    onEditConfirm = onEditConfirmClothingSize,
                    onEditCancel = onEditCancelClothingSize,
                    onDelete = onDeleteClothingSize,
                )
                GiftPreferencesPanel(
                    items = giftPreferences,
                    onAdd = onAddGiftPreference,
                    onEdit = onEditGiftPreference,
                    onDelete = onDeleteGiftPreference,
                )
            }
        }
    }
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
                        // #205: the row-action label carries the item's value so
                        // TalkBack doesn't read a bare "Delete" on every row.
                        Icon(
                            Icons.Outlined.Delete,
                            contentDescription = stringResource(R.string.entities_delete_named, item.value),
                        )
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
 * edit. Adding/editing goes through the shared PreferenceForm, scoped to
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
                text = stringResource(R.string.gifts_preferences_empty),
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
    // Issue #1723: the open mark-discussed dialog is ViewModel state (saved in
    // its SavedStateHandle), so it survives rotation and process death, and the
    // ViewModel closes it only on a successful PATCH or a dismiss.
    val pendingDiscussId by viewModel.pendingDiscussId.collectAsStateWithLifecycle()
    // Hoisted above the form/list switch so Back returns to where the list was scrolled.
    val listState = rememberLazyListState()

    // Issue #1773: marking discussed carries a picker (the activity to link), so by the
    // "any picker means a full-screen form" rule it is a full-screen form, not a dialog.
    val discussItem = pendingDiscussId?.let { viewModel.findById(it) }
    if (discussItem != null) {
        MarkDiscussedForm(
            item = discussItem,
            activities = activities,
            confirming = discussingId == discussItem.id,
            onConfirm = { activityId -> viewModel.markDiscussed(discussItem.id, activityId) },
            onDismiss = viewModel::dismissDiscuss,
            errorMessage = state.errorRes?.let { stringResource(it) } ?: state.error,
            onErrorShown = viewModel::onErrorShown,
        )
        return
    }

    EntityListScaffold(
        title = stringResource(R.string.agenda_title),
        addLabel = stringResource(R.string.agenda_new),
        emptyTitle = stringResource(R.string.agenda_empty),
        emptyBody = stringResource(R.string.agenda_empty_body),
        uiState = state,
        onAdd = { showAdd = true },
        onItemClick = { id -> viewModel.findById(id)?.let { editingItem = it } },
        onDelete = viewModel::delete,
        onErrorShown = viewModel::onErrorShown,
        onBack = onBack,
        onRefresh = viewModel::load,
        listState = listState,
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
                IconButton(onClick = { viewModel.openDiscuss(item.id) }) {
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
    }
}

/**
 * Web's MarkDiscussedDialog -- marks an item discussed, optionally linked to an activity.
 * Issue #1773: a full-screen form (it has an activity picker). The open form is ViewModel
 * state ([ConversationAgendaViewModel.pendingDiscussId]); the picked activity is
 * `rememberSaveable`.
 */
@Composable
internal fun MarkDiscussedForm(
    item: ConversationAgenda,
    activities: List<Activity>,
    confirming: Boolean,
    onConfirm: (Int?) -> Unit,
    onDismiss: () -> Unit,
    errorMessage: String? = null,
    onErrorShown: () -> Unit = {},
) {
    // rememberSaveable: the picked activity survives rotation (issue #1723 review).
    var activityId by rememberSaveable(item.id) { mutableStateOf<Int?>(null) }
    EntityFormScreen(
        title = stringResource(R.string.agenda_mark_discussed),
        saveLabel = stringResource(R.string.agenda_discussed_button),
        saveEnabled = !confirming,
        saveDisabledHint = null,
        onSave = { onConfirm(activityId) },
        onBack = { if (!confirming) onDismiss() },
        errorMessage = errorMessage,
        onErrorShown = onErrorShown,
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
}

/** Unknown/custom categories fall back to the humanized token (issue #1755), never the raw one. */
@Composable
internal fun preferenceCategoryText(category: String): String =
    preferenceCategoryLabelRes(category)?.let { stringResource(it) } ?: humanizeToken(category)

/** Issue #1755: the localized row title for an entity item (token families resolve here, not in the ViewModel). */
@Composable
internal fun EntityItem.displayLabel(): String {
    val token = labelToken ?: return label
    return when (labelFamily) {
        EntityLabelFamily.LIFE_EVENT -> {
            val type = lifeEventTypeLabel(token)
            labelDetail?.let { "$type — $it" } ?: type
        }
        EntityLabelFamily.PREFERENCE -> {
            val category = preferenceCategoryText(token)
            labelDetail?.let { "$category: $it" } ?: category
        }
        null -> label
    }
}
