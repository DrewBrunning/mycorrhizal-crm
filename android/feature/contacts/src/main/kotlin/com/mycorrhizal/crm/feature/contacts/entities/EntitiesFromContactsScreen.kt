package com.mycorrhizal.crm.feature.contacts.entities

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.selection.toggleable
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.Checkbox
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.PrimaryTabRow
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Tab
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.EntityFromContactsSkipReason
import com.mycorrhizal.crm.model.network.EntityKind
import com.mycorrhizal.crm.model.network.EntityLinkRoles
import com.mycorrhizal.crm.model.network.EntityValueGroup
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.AccessibleIconButton
import com.mycorrhizal.crm.ui.components.DropdownOption
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LabelledDropdown
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.components.localizedMessage

/**
 * #2513 (web #2301): the optional "create entities from my contacts" helper. Reached from the
 * entities screen; it reads nothing until opened and changes nothing until Create is pressed.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun EntitiesFromContactsScreen(
    onBack: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: EntitiesFromContactsViewModel = hiltViewModel()
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    Scaffold(
        modifier = modifier,
        topBar = {
            TopAppBar(
                navigationIcon = {
                    AccessibleIconButton(onClick = onBack) {
                        Icon(
                            Icons.AutoMirrored.Outlined.ArrowBack,
                            contentDescription = stringResource(R.string.cd_back)
                        )
                    }
                },
                title = {
                    Text(
                        stringResource(R.string.shared_entities_from_contacts_title),
                        style = MaterialTheme.typography.titleLarge
                    )
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary
                )
            )
        }
    ) { padding ->
        Box(Modifier.fillMaxSize().padding(padding)) {
            EntitiesFromContactsContent(
                state = state,
                actions = EntitiesFromContactsActions(
                    onSelectKind = viewModel::selectKind,
                    onSelected = viewModel::setSelected,
                    onName = viewModel::setName,
                    onRole = viewModel::setRole,
                    onPrimary = viewModel::setPrimary,
                    onContact = viewModel::setContactIncluded,
                    onCreate = viewModel::create,
                    onRetry = viewModel::refresh,
                    onLoadMore = viewModel::loadMore,
                    onDismissResult = viewModel::dismissResult
                )
            )
        }
    }
}

@Composable
internal fun EntitiesFromContactsContent(
    state: EntitiesFromContactsUiState,
    actions: EntitiesFromContactsActions
) {
    Column(Modifier.fillMaxSize()) {
        Text(
            text = stringResource(R.string.shared_entities_from_contacts_intro),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp)
        )
        PrimaryTabRow(
            selectedTabIndex = FROM_CONTACTS_KINDS.indexOf(state.kind).coerceAtLeast(0),
            modifier = Modifier.testTag("from-contacts-tabs")
        ) {
            FROM_CONTACTS_KINDS.forEach { kind ->
                Tab(
                    selected = kind == state.kind,
                    onClick = { actions.onSelectKind(kind) },
                    text = { Text(stringResource(entityKindTabRes(kind))) },
                    modifier = Modifier.testTag("from-contacts-tab-$kind")
                )
            }
        }
        LazyColumn(
            modifier = Modifier.fillMaxWidth().weight(1f).testTag("from-contacts-list"),
            contentPadding = PaddingValues(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            state.result?.let { result ->
                item(key = "result") { ResultCard(result, actions.onDismissResult) }
            }
            state.createError?.let { error ->
                item(key = "create-error") {
                    ErrorBlock(
                        title = stringResource(
                            R.string.shared_entities_from_contacts_create_failed
                        ),
                        error = error,
                        tag = "from-contacts-error"
                    )
                }
            }
            state.loadError?.let { error ->
                item(key = "load-error") {
                    Column {
                        ErrorBlock(
                            title = stringResource(
                                R.string.shared_entities_from_contacts_load_failed
                            ),
                            error = error,
                            tag = "from-contacts-load-error"
                        )
                        TextButton(onClick = actions.onRetry) {
                            Text(stringResource(R.string.action_retry))
                        }
                    }
                }
            }
            if (state.isLoading && state.groups.isEmpty()) {
                item(key = "loading") { LoadingSkeleton() }
            }
            if (state.loaded && state.groups.isEmpty() && !state.isLoading &&
                state.loadError == null
            ) {
                item(key = "empty") {
                    Box(Modifier.testTag("from-contacts-empty")) {
                        EmptyState(
                            title = stringResource(R.string.shared_entities_from_contacts_none)
                        )
                    }
                }
            }
            items(state.groups, key = { it.key }) { group ->
                GroupCard(state, group, actions)
            }
            if (state.nextCursor != null) {
                item(key = "load-more") {
                    val currentOnLoadMore by rememberUpdatedState(actions.onLoadMore)
                    LaunchedEffect(state.nextCursor) { currentOnLoadMore() }
                    Box(Modifier.testTag("from-contacts-load-more")) { LoadingSkeleton(rows = 1) }
                }
            }
        }
        Surface(tonalElevation = 3.dp, modifier = Modifier.fillMaxWidth()) {
            Button(
                onClick = actions.onCreate,
                enabled = !state.isCreating && state.chosenCount > 0,
                modifier = Modifier.fillMaxWidth().padding(16.dp).testTag("from-contacts-create")
            ) {
                Text(createLabel(state.kind, state.chosenCount))
            }
        }
    }
}

@Composable
private fun createLabel(kind: String, count: Int): String = pluralStringResource(
    if (kind == EntityKind.PLACE) {
        R.plurals.shared_entities_from_contacts_create_place
    } else {
        R.plurals.shared_entities_from_contacts_create_organisation
    },
    count,
    count
)

@Composable
private fun ErrorBlock(title: String, error: ApiError, tag: String) {
    Column(Modifier.fillMaxWidth().testTag(tag)) {
        Text(title, color = MaterialTheme.colorScheme.error)
        Text(
            error.localizedMessage(),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant
        )
    }
}

@Composable
private fun GroupCard(
    state: EntitiesFromContactsUiState,
    group: EntityValueGroup,
    actions: EntitiesFromContactsActions
) {
    val edit = state.edit(group)
    val contacts = group.contactsOrEmpty
    val withPrimary = contacts.count { it.hasPrimary }
    val includeDescription =
        stringResource(R.string.shared_entities_from_contacts_include_cd, group.name)
    Card(Modifier.fillMaxWidth().testTag("from-contacts-group-${group.key}")) {
        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(
                modifier = Modifier.fillMaxWidth()
                    .toggleable(
                        value = edit.selected,
                        role = Role.Checkbox,
                        onValueChange = { actions.onSelected(group.key, it) }
                    )
                    .semantics { contentDescription = includeDescription }
                    .testTag("from-contacts-include-${group.key}"),
                verticalAlignment = Alignment.CenterVertically
            ) {
                Checkbox(checked = edit.selected, onCheckedChange = null)
                Text(
                    pluralStringResource(
                        R.plurals.shared_entities_from_contacts_group_title,
                        contacts.size,
                        group.name,
                        contacts.size
                    ),
                    style = MaterialTheme.typography.titleMedium,
                    modifier = Modifier.padding(start = 12.dp, top = 8.dp, bottom = 8.dp)
                )
            }
            if (group.variantsOrEmpty.size > 1) {
                Text(
                    stringResource(
                        R.string.shared_entities_from_contacts_variants,
                        group.variantsOrEmpty.joinToString(", ")
                    ),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            }
            if (withPrimary > 0) {
                Text(
                    pluralStringResource(
                        if (state.kind == EntityKind.PLACE) {
                            R.plurals.shared_entities_from_contacts_has_primary_place
                        } else {
                            R.plurals.shared_entities_from_contacts_has_primary_organisation
                        },
                        withPrimary,
                        withPrimary
                    ),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            }
            if (edit.selected) GroupEditor(state.kind, group, edit, actions)
        }
    }
}

@Composable
private fun GroupEditor(
    kind: String,
    group: EntityValueGroup,
    edit: FromContactsGroupEdit,
    actions: EntitiesFromContactsActions
) {
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        OutlinedTextField(
            value = edit.name,
            onValueChange = { actions.onName(group.key, it) },
            label = {
                Text(stringResource(R.string.shared_entities_from_contacts_name, group.name))
            },
            singleLine = true,
            modifier = Modifier.fillMaxWidth().testTag("from-contacts-name-${group.key}")
        )
        LabelledDropdown(
            label = stringResource(R.string.shared_entities_from_contacts_role, group.name),
            valueText = entityRoleText(kind, edit.role, ""),
            options = EntityLinkRoles.forKind(kind).map {
                DropdownOption(it, entityRoleText(kind, it, ""))
            },
            onSelect = { actions.onRole(group.key, it) },
            modifier = Modifier.testTag("from-contacts-role-${group.key}")
        )
        CheckRow(
            checked = edit.setPrimary,
            label = stringResource(R.string.shared_entities_from_contacts_primary),
            description = stringResource(
                R.string.shared_entities_from_contacts_primary_cd,
                group.name
            ),
            tag = "from-contacts-primary-${group.key}",
            onChange = { actions.onPrimary(group.key, it) }
        )
        Text(
            stringResource(R.string.shared_entities_from_contacts_contacts_heading),
            style = MaterialTheme.typography.labelLarge
        )
        group.contactsOrEmpty.forEach { contact ->
            CheckRow(
                checked = contact.contactId !in edit.excluded,
                label = contact.displayName.ifBlank { contact.valueName },
                description = null,
                tag = "from-contacts-contact-${group.key}-${contact.contactId}",
                onChange = { actions.onContact(group.key, contact.contactId, it) }
            )
        }
    }
}

/** A labelled checkbox whose whole row toggles (one 48dp+ target, one TalkBack node). */
@Composable
private fun CheckRow(
    checked: Boolean,
    label: String,
    description: String?,
    tag: String,
    onChange: (Boolean) -> Unit
) {
    Row(
        modifier = Modifier.fillMaxWidth()
            .toggleable(value = checked, role = Role.Checkbox, onValueChange = onChange)
            .semantics { if (description != null) contentDescription = description }
            .padding(vertical = 4.dp)
            .testTag(tag),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Checkbox(checked = checked, onCheckedChange = null)
        Text(
            label,
            style = MaterialTheme.typography.bodyMedium,
            modifier = Modifier.padding(start = 12.dp)
        )
    }
}

@Composable
private fun ResultCard(result: FromContactsResult, onDismiss: () -> Unit) {
    Card(Modifier.fillMaxWidth().testTag("from-contacts-result")) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            if (result.createdCount > 0) {
                Text(
                    pluralStringResource(
                        if (result.kind == EntityKind.PLACE) {
                            R.plurals.shared_entities_from_contacts_created_place
                        } else {
                            R.plurals.shared_entities_from_contacts_created_organisation
                        },
                        result.createdCount,
                        result.createdCount
                    ),
                    style = MaterialTheme.typography.titleMedium
                )
                Text(
                    result.createdNames.joinToString(", "),
                    style = MaterialTheme.typography.bodyMedium
                )
            } else {
                Text(
                    stringResource(R.string.shared_entities_from_contacts_result_none),
                    style = MaterialTheme.typography.titleMedium
                )
            }
            result.skipped.forEach { item ->
                Text(
                    skipText(item),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            }
            TextButton(
                onClick = onDismiss,
                modifier = Modifier.testTag("from-contacts-result-dismiss")
            ) { Text(stringResource(R.string.action_done)) }
        }
    }
}

/** #2093-style explicit mapping: every known skip token has its own text; unknown ones a generic. */
@Composable
private fun skipText(item: FromContactsSkipItem): String = when (item.reason) {
    EntityFromContactsSkipReason.GROUP_NOT_FOUND ->
        stringResource(R.string.shared_entities_from_contacts_skip_group_not_found, item.groupName)

    EntityFromContactsSkipReason.CONTACT_NOT_IN_GROUP ->
        if (item.contactName.isNullOrBlank()) {
            stringResource(
                R.string.shared_entities_from_contacts_skip_contact_not_in_group,
                item.groupName
            )
        } else {
            stringResource(
                R.string.shared_entities_from_contacts_skip_contact_named,
                item.groupName,
                item.contactName
            )
        }

    EntityFromContactsSkipReason.NAME_INVALID ->
        stringResource(R.string.shared_entities_from_contacts_skip_name_invalid, item.groupName)

    EntityFromContactsSkipReason.CORE_INVALID ->
        stringResource(R.string.shared_entities_from_contacts_skip_core_invalid, item.groupName)

    else -> stringResource(R.string.shared_entities_from_contacts_skip_unknown, item.groupName)
}
