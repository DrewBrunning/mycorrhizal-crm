package com.mycorrhizal.crm.feature.households

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
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Menu
import androidx.compose.material.icons.automirrored.outlined.ArrowForward
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.AutoAwesome
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material.icons.outlined.Home
import androidx.compose.material.icons.outlined.LocationOn
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExposedDropdownMenuBox
import androidx.compose.material3.ExposedDropdownMenuDefaults
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedCard
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
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.AddressHouseholdSuggestion
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.Household
import com.mycorrhizal.crm.model.network.HouseholdTypes
import com.mycorrhizal.crm.model.network.formatSuggestionAddress
import com.mycorrhizal.crm.ui.components.BrandFab
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.R

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HouseholdsScreen(
    // Issue #150: null hides the hamburger — there is no drawer at Expanded.
    onMenuClick: (() -> Unit)? = {},
    onOpenHousehold: (String) -> Unit,
    viewModel: HouseholdsViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }
    var showCreateDialog by remember { mutableStateOf(false) }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    onMenuClick?.let { onMenu ->
                        IconButton(onClick = onMenu) {
                            Icon(Icons.Outlined.Menu, contentDescription = stringResource(R.string.cd_menu))
                        }
                    }
                },
                title = {
                    Text(stringResource(R.string.households_title), style = MaterialTheme.typography.titleLarge)
                },
                actions = {
                    IconButton(
                        onClick = { viewModel.scanAddressSuggestions() },
                        enabled = !state.suggestionsLoading,
                    ) {
                        if (state.suggestionsLoading) {
                            CircularProgressIndicator(
                                modifier = Modifier.padding(6.dp),
                                strokeWidth = 2.dp,
                                color = MaterialTheme.colorScheme.onPrimary,
                            )
                        } else {
                            Icon(
                                Icons.Outlined.LocationOn,
                                contentDescription = stringResource(R.string.households_suggest_addresses),
                            )
                        }
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary,
                    actionIconContentColor = MaterialTheme.colorScheme.onPrimary,
                ),
            )
        },
        floatingActionButton = {
            BrandFab(onClick = { showCreateDialog = true }) {
                Icon(Icons.Outlined.Add, contentDescription = stringResource(R.string.households_new))
            }
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            when {
                state.isLoading -> LoadingSkeleton()
                state.households.isEmpty() && !state.suggestionsLoaded && state.error == null ->
                    EmptyState(message = stringResource(R.string.households_empty))
                state.households.isEmpty() && !state.suggestionsLoaded && state.error != null -> {
                    Text(
                        text = state.error.orEmpty(),
                        color = MaterialTheme.colorScheme.error,
                        modifier = Modifier.align(Alignment.Center),
                    )
                }
                else -> {
                    LazyColumn(modifier = Modifier.fillMaxSize()) {
                        if (state.suggestionsLoaded) {
                            item(key = "suggestion-header") {
                                Text(
                                    text = stringResource(R.string.households_address_suggestions),
                                    style = MaterialTheme.typography.titleMedium,
                                    modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
                                )
                            }
                            if (state.addressSuggestions.isEmpty()) {
                                item(key = "suggestion-empty") {
                                    Text(
                                        text = stringResource(R.string.households_no_address_suggestions),
                                        style = MaterialTheme.typography.bodyMedium,
                                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                                        modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
                                    )
                                }
                            } else {
                                items(
                                    state.addressSuggestions,
                                    key = { suggestionKey(it) },
                                ) { suggestion ->
                                    val key = suggestionKey(suggestion)
                                    AddressSuggestionCard(
                                        suggestion = suggestion,
                                        contactsByUid = state.contactsByUid,
                                        pending = state.pendingSuggestionKey == "accept:$key" ||
                                            state.pendingSuggestionKey == "dismiss:$key",
                                        onAccept = { viewModel.acceptSuggestion(suggestion) },
                                        onDismiss = { viewModel.dismissSuggestion(suggestion) },
                                    )
                                }
                            }
                            item(key = "suggestion-divider") {
                                HorizontalDivider(modifier = Modifier.padding(vertical = 8.dp))
                            }
                        }
                        items(state.households, key = { it.id }) { household ->
                            HouseholdListItem(
                                household = household,
                                onClick = { onOpenHousehold(household.id) },
                                onUpdate = { name, type -> viewModel.rename(household.id, name, type) },
                                onDelete = { viewModel.delete(household.id) },
                            )
                        }
                    }
                }
            }
        }
    }

    if (showCreateDialog) {
        HouseholdFormDialog(
            title = stringResource(R.string.households_new),
            initialName = "",
            initialType = HouseholdTypes.FAMILY_UNIT,
            confirmLabel = stringResource(R.string.action_create),
            onConfirm = { name, type ->
                viewModel.create(name, type)
                showCreateDialog = false
            },
            onDismiss = { showCreateDialog = false },
        )
    }

    state.error?.let { message ->
        LaunchedEffect(message) {
            snackbarHostState.showSnackbar(message)
            viewModel.onErrorShown()
        }
    }

    val infoMessage = state.infoRes?.let { res ->
        val count = state.infoCount
        if (count != null) stringResource(res, count) else stringResource(res)
    }

    infoMessage?.let { message ->
        LaunchedEffect(message) {
            snackbarHostState.showSnackbar(message)
            viewModel.onInfoShown()
        }
    }
}

@Composable
private fun AddressSuggestionCard(
    suggestion: AddressHouseholdSuggestion,
    contactsByUid: Map<String, ContactSummary>,
    pending: Boolean,
    onAccept: () -> Unit,
    onDismiss: () -> Unit,
) {
    val membersText = suggestion.memberVCardUids.joinToString(" · ") { uid ->
        val contact = contactsByUid[uid]
        if (contact != null) displayNameFor(contact) ?: uid else uid
    }
    OutlinedCard(modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp)) {
        Column(modifier = Modifier.padding(12.dp)) {
            Text(
                text = membersText,
                style = MaterialTheme.typography.bodyLarge,
            )
            Text(
                text = formatSuggestionAddress(suggestion.address),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Row(
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                modifier = Modifier.padding(top = 8.dp),
            ) {
                TextButton(onClick = onAccept, enabled = !pending) {
                    Icon(Icons.Outlined.AutoAwesome, contentDescription = null, modifier = Modifier.padding(end = 4.dp))
                    Text(stringResource(R.string.households_accept_suggestion))
                }
                TextButton(onClick = onDismiss, enabled = !pending) {
                    Text(stringResource(R.string.households_dismiss_suggestion))
                }
            }
        }
    }
}

private fun suggestionKey(suggestion: AddressHouseholdSuggestion): String =
    "${suggestion.addressHash}:${suggestion.memberHash}"

@Composable
private fun HouseholdListItem(
    household: Household,
    onClick: () -> Unit,
    onUpdate: (String, String) -> Unit,
    onDelete: () -> Unit,
) {
    var editing by remember { mutableStateOf(false) }
    var deleting by remember { mutableStateOf(false) }

    if (editing) {
        HouseholdFormDialog(
            title = stringResource(R.string.households_edit),
            initialName = household.name,
            initialType = household.type,
            confirmLabel = stringResource(R.string.action_save),
            onConfirm = { name, type -> onUpdate(name, type); editing = false },
            onDismiss = { editing = false },
        )
    }
    if (deleting) {
        AlertDialog(
            onDismissRequest = { deleting = false },
            title = { Text(stringResource(R.string.households_delete_title)) },
            text = { Text(stringResource(R.string.households_delete_confirm, household.name)) },
            confirmButton = {
                TextButton(onClick = { onDelete(); deleting = false }) {
                    Text(stringResource(R.string.action_delete))
                }
            },
            dismissButton = {
                TextButton(onClick = { deleting = false }) {
                    Text(stringResource(R.string.action_cancel))
                }
            },
        )
    }

    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onClick)
            .padding(horizontal = 16.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        Icon(
            imageVector = Icons.Outlined.Home,
            contentDescription = null,
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = household.name,
                style = MaterialTheme.typography.bodyLarge,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            Text(
                text = HouseholdTypeLabels.label(household.type),
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        IconButton(onClick = { editing = true }) {
            Icon(
                Icons.Outlined.Edit,
                contentDescription = stringResource(R.string.action_rename),
                tint = MaterialTheme.colorScheme.primary,
            )
        }
        IconButton(onClick = { deleting = true }) {
            Icon(Icons.Outlined.Delete, contentDescription = stringResource(R.string.action_delete))
        }
        Icon(Icons.AutoMirrored.Outlined.ArrowForward, contentDescription = null)
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HouseholdFormDialog(
    title: String,
    initialName: String,
    initialType: String,
    confirmLabel: String,
    onConfirm: (String, String) -> Unit,
    onDismiss: () -> Unit,
) {
    var name by remember { mutableStateOf(initialName) }
    var type by remember { mutableStateOf(initialType) }
    var expanded by remember { mutableStateOf(false) }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(title) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedTextField(
                    value = name,
                    onValueChange = { name = it },
                    label = { Text(stringResource(R.string.households_name)) },
                    singleLine = true,
                )
                ExposedDropdownMenuBox(
                    expanded = expanded,
                    onExpandedChange = { expanded = it },
                ) {
                    OutlinedTextField(
                        value = HouseholdTypeLabels.label(type),
                        onValueChange = {},
                        readOnly = true,
                        label = { Text(stringResource(R.string.households_type)) },
                        trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded = expanded) },
                        modifier = Modifier
                            .fillMaxWidth()
                            .menuAnchor(),
                    )
                    ExposedDropdownMenu(
                        expanded = expanded,
                        onDismissRequest = { expanded = false },
                    ) {
                        HouseholdTypes.ALL.forEach { t ->
                            DropdownMenuItem(
                                text = { Text(HouseholdTypeLabels.label(t)) },
                                onClick = { type = t; expanded = false },
                            )
                        }
                    }
                }
            }
        },
        confirmButton = {
            TextButton(
                onClick = { onConfirm(name, type) },
                enabled = name.isNotBlank(),
            ) {
                Text(confirmLabel)
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) {
                Text(stringResource(R.string.action_cancel))
            }
        },
    )
}
