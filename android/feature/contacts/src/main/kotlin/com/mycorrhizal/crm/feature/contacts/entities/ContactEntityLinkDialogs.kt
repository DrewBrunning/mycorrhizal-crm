package com.mycorrhizal.crm.feature.contacts.entities

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.model.network.ContactEntityLink
import com.mycorrhizal.crm.model.network.EntityKind
import com.mycorrhizal.crm.model.network.EntityLinkRoles
import com.mycorrhizal.crm.model.network.EntitySensitivity
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.DropdownOption
import com.mycorrhizal.crm.ui.components.LabelledDropdown
import com.mycorrhizal.crm.ui.components.localizedMessage
import kotlinx.coroutines.delay

private const val SEARCH_DEBOUNCE_MS = 250L
private const val ROLE_LABEL_MAX = 100

/** Create ([editing] null) or edit one contact -> entity link. */
@Composable
internal fun ContactEntityLinkDialog(
    editing: ContactEntityLink?,
    state: ContactEntityLinksUiState,
    onSearch: (kind: String, query: String) -> Unit,
    onSave: (LinkForm) -> Unit,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier
) {
    val key = editing?.id
    var kind by rememberSaveable(key) { mutableStateOf(editing?.kind ?: EntityKind.ORGANISATION) }
    var query by rememberSaveable(key) { mutableStateOf("") }
    var entityId by rememberSaveable(key) { mutableStateOf<String?>(null) }
    var newName by rememberSaveable(key) { mutableStateOf<String?>(null) }
    var role by rememberSaveable(key) { mutableStateOf(editing?.role ?: "") }
    var roleLabel by rememberSaveable(key) { mutableStateOf(editing?.roleLabel.orEmpty()) }
    var primary by rememberSaveable(key) { mutableStateOf(editing?.isPrimary ?: false) }
    var start by rememberSaveable(key) {
        mutableStateOf(editing?.period?.start?.year?.toString().orEmpty())
    }
    var end by rememberSaveable(key) {
        mutableStateOf(editing?.period?.end?.year?.toString().orEmpty())
    }
    var sensitivity by rememberSaveable(key) {
        mutableStateOf(editing?.sensitivity ?: EntitySensitivity.NORMAL)
    }
    var entityMissing by remember { mutableStateOf(false) }
    val roles = EntityLinkRoles.forKind(kind)
    // A kind change (or a role this client does not know) falls back to the kind's first role.
    val effectiveRole = if (role in roles) role else roles.firstOrNull() ?: role

    val currentOnSearch by rememberUpdatedState(onSearch)
    if (editing == null) {
        LaunchedEffect(kind, query) {
            delay(SEARCH_DEBOUNCE_MS)
            currentOnSearch(kind, query)
        }
    }

    AlertDialog(
        modifier = modifier.testTag("contact-entity-link-dialog"),
        onDismissRequest = onDismiss,
        title = {
            Text(
                if (editing == null) {
                    stringResource(R.string.contact_entity_links_add_title)
                } else {
                    stringResource(R.string.contact_entity_links_edit_title, editing.entityName)
                }
            )
        },
        text = {
            Column(
                Modifier.verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                if (editing == null) {
                    LabelledDropdown(
                        label = stringResource(R.string.contact_entity_links_kind),
                        valueText = stringResource(entityKindOneRes(kind)),
                        options = EntityKind.ALL.map {
                            DropdownOption(
                                it,
                                stringResource(entityKindOneRes(it))
                            )
                        },
                        onSelect = {
                            kind = it
                            entityId = null
                            newName = null
                            primary = primary && EntityKind.projects(it)
                        },
                        modifier = Modifier.testTag("contact-entity-link-kind")
                    )
                    EntityPicker(
                        query = query,
                        onQueryChange = {
                            query = it
                            entityMissing = false
                        },
                        results = state.searchResults.filter { it.kind == kind },
                        selectedId = entityId,
                        newName = newName,
                        onPick = {
                            entityId = it
                            newName = null
                            entityMissing = false
                        },
                        onCreate = {
                            entityId = null
                            newName = it
                            entityMissing = false
                        }
                    )
                    if (entityMissing) {
                        Text(
                            stringResource(R.string.contact_entity_links_entity_required),
                            color = MaterialTheme.colorScheme.error,
                            style = MaterialTheme.typography.bodySmall
                        )
                    }
                }
                LabelledDropdown(
                    label = stringResource(R.string.contact_entity_links_role),
                    valueText = entityRoleText(kind, effectiveRole, ""),
                    options = roles.map { DropdownOption(it, entityRoleText(kind, it, "")) },
                    onSelect = { role = it },
                    modifier = Modifier.testTag("contact-entity-link-role")
                )
                OutlinedTextField(
                    value = roleLabel,
                    onValueChange = { roleLabel = it.take(ROLE_LABEL_MAX) },
                    label = { Text(stringResource(R.string.contact_entity_links_role_label)) },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth().testTag("contact-entity-link-role-label")
                )
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    YearField(
                        start,
                        R.string.contact_period_from,
                        "contact-entity-link-start",
                        onChange = { start = it },
                        modifier = Modifier.weight(1f)
                    )
                    YearField(
                        end,
                        R.string.contact_period_to,
                        "contact-entity-link-end",
                        onChange = { end = it },
                        modifier = Modifier.weight(1f)
                    )
                }
                if (state.periodInvalid) {
                    Text(
                        stringResource(R.string.contact_entity_links_year_invalid),
                        color = MaterialTheme.colorScheme.error,
                        style = MaterialTheme.typography.bodySmall,
                        modifier = Modifier.testTag("contact-entity-link-year-error")
                    )
                }
                if (EntityKind.projects(kind)) {
                    Row(
                        modifier = Modifier.fillMaxWidth()
                            .toggleable(
                                value = primary,
                                role = Role.Switch,
                                onValueChange = { primary = it }
                            )
                            .testTag("contact-entity-link-primary"),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.SpaceBetween
                    ) {
                        Text(
                            stringResource(R.string.contact_entity_links_primary_help),
                            modifier = Modifier.weight(1f)
                        )
                        Switch(checked = primary, onCheckedChange = null)
                    }
                }
                LabelledDropdown(
                    label = stringResource(R.string.shared_entities_sensitivity_label),
                    valueText = stringResource(entitySensitivityRes(sensitivity)),
                    options = EntitySensitivity.ALL.map {
                        DropdownOption(it, stringResource(entitySensitivityRes(it)))
                    },
                    onSelect = { sensitivity = it }
                )
                state.formError?.let {
                    Text(
                        it.localizedMessage(),
                        color = MaterialTheme.colorScheme.error,
                        style = MaterialTheme.typography.bodySmall,
                        modifier = Modifier.testTag("contact-entity-link-error")
                    )
                }
            }
        },
        confirmButton = {
            TextButton(
                enabled = !state.isSaving,
                modifier = Modifier.testTag("contact-entity-link-save"),
                onClick = {
                    if (editing == null && entityId == null && newName.isNullOrBlank()) {
                        entityMissing = true
                        return@TextButton
                    }
                    onSave(
                        LinkForm(
                            entityId = entityId,
                            newEntityName = newName,
                            kind = kind,
                            role = effectiveRole,
                            roleLabel = roleLabel,
                            isPrimary = primary && EntityKind.projects(kind),
                            startYear = start,
                            endYear = end,
                            sensitivity = sensitivity,
                            editing = editing
                        )
                    )
                }
            ) { Text(stringResource(R.string.action_save)) }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) }
        }
    )
}

@Composable
private fun YearField(
    value: String,
    label: Int,
    tag: String,
    onChange: (String) -> Unit,
    modifier: Modifier = Modifier
) {
    OutlinedTextField(
        value = value,
        onValueChange = { onChange(it.filter(Char::isDigit).take(4)) },
        label = { Text(stringResource(label)) },
        singleLine = true,
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
        modifier = modifier.testTag(tag)
    )
}

/** Search the user's entities of [kind], or type a new name to create one on save. */
@Composable
private fun EntityPicker(
    query: String,
    onQueryChange: (String) -> Unit,
    results: List<com.mycorrhizal.crm.model.network.Entity>,
    selectedId: String?,
    newName: String?,
    onPick: (String) -> Unit,
    onCreate: (String) -> Unit
) {
    val trimmed = query.trim()
    val exact = results.any { it.name.equals(trimmed, ignoreCase = true) }
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        OutlinedTextField(
            value = query,
            onValueChange = onQueryChange,
            label = { Text(stringResource(R.string.contact_entity_links_entity)) },
            singleLine = true,
            modifier = Modifier.fillMaxWidth().testTag("contact-entity-link-entity-search")
        )
        Column(Modifier.heightIn(max = 180.dp).verticalScroll(rememberScrollState())) {
            results.forEach { entity ->
                PickRow(
                    selected = entity.id == selectedId,
                    text = entity.name,
                    tag = "contact-entity-link-pick-${entity.id}"
                ) { onPick(entity.id) }
            }
            if (trimmed.isNotEmpty() && !exact) {
                PickRow(
                    selected = newName != null && newName == trimmed,
                    text = stringResource(R.string.contact_entity_links_create, trimmed),
                    tag = "contact-entity-link-create"
                ) { onCreate(trimmed) }
            }
        }
    }
}

@Composable
private fun PickRow(selected: Boolean, text: String, tag: String, onClick: () -> Unit) {
    Row(
        modifier = Modifier.fillMaxWidth()
            .selectable(selected = selected, role = Role.RadioButton, onClick = onClick)
            .padding(vertical = 4.dp)
            .testTag(tag),
        verticalAlignment = Alignment.CenterVertically
    ) {
        RadioButton(selected = selected, onClick = null)
        Text(text, modifier = Modifier.padding(start = 8.dp))
    }
}

/**
 * Link one of the contact's own ORG/ADR values to a shared entity (ADR 0043 decision 3). The value
 * stays on the contact. Defaults to creating a new entity from it; an existing organisation with
 * the same name is offered but never pre-selected.
 */
@Composable
internal fun LinkOwnValueDialog(
    state: ContactEntityLinksUiState,
    onFindMatch: (OwnValue) -> Unit,
    onLink: (OwnValue, existingEntityId: String?, role: String) -> Unit,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier
) {
    val values = state.ownValues
    var selectedIndex by rememberSaveable { mutableIntStateOf(0) }
    var useExisting by rememberSaveable { mutableStateOf(false) }
    val value = values.getOrNull(selectedIndex)
    val kind = if (value?.kind == "address") EntityKind.PLACE else EntityKind.ORGANISATION
    val roles = EntityLinkRoles.forKind(kind)
    var role by rememberSaveable { mutableStateOf("") }
    val effectiveRole = if (role in roles) role else roles.first()

    val currentOnFindMatch by rememberUpdatedState(onFindMatch)
    LaunchedEffect(selectedIndex, values.size) {
        useExisting = false
        value?.let(currentOnFindMatch)
    }

    AlertDialog(
        modifier = modifier.testTag("contact-entity-from-own-dialog"),
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.contact_entity_links_from_own_title)) },
        text = {
            Column(
                Modifier.verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                if (value == null) {
                    Text(stringResource(R.string.contact_entity_links_from_own_none))
                } else {
                    Text(
                        stringResource(R.string.contact_entity_links_from_own_intro),
                        style = MaterialTheme.typography.bodyMedium
                    )
                    LabelledDropdown(
                        label = stringResource(R.string.contact_entity_links_from_own_value),
                        valueText = value.label,
                        options = values.mapIndexed { i, v -> DropdownOption(i, v.label) },
                        onSelect = { selectedIndex = it },
                        modifier = Modifier.testTag("contact-entity-own-value")
                    )
                    PickRow(
                        selected = !useExisting,
                        text = stringResource(
                            if (kind == EntityKind.PLACE) {
                                R.string.contact_entity_links_from_own_create_new_place
                            } else {
                                R.string.contact_entity_links_from_own_create_new_organisation
                            }
                        ),
                        tag = "contact-entity-own-create-new"
                    ) { useExisting = false }
                    state.ownMatch?.let { match ->
                        PickRow(
                            selected = useExisting,
                            text = stringResource(
                                R.string.contact_entity_links_from_own_use_existing_organisation,
                                match.name
                            ),
                            tag = "contact-entity-own-use-existing"
                        ) { useExisting = true }
                    }
                    LabelledDropdown(
                        label = stringResource(R.string.contact_entity_links_role),
                        valueText = entityRoleText(kind, effectiveRole, ""),
                        options = roles.map { DropdownOption(it, entityRoleText(kind, it, "")) },
                        onSelect = { role = it }
                    )
                }
                state.formError?.let {
                    Text(
                        it.localizedMessage(),
                        color = MaterialTheme.colorScheme.error,
                        style = MaterialTheme.typography.bodySmall
                    )
                }
            }
        },
        confirmButton = {
            TextButton(
                enabled = value != null && !state.isSaving,
                modifier = Modifier.testTag("contact-entity-own-link"),
                onClick = {
                    value?.let {
                        onLink(
                            it,
                            if (useExisting) state.ownMatch?.id else null,
                            effectiveRole
                        )
                    }
                }
            ) { Text(stringResource(R.string.contact_entity_links_from_own_link)) }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) }
        }
    )
}
